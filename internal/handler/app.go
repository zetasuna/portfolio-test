package handler

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/example/portfolio-crud/internal/auth"
	"github.com/example/portfolio-crud/internal/config"
	"github.com/example/portfolio-crud/internal/middleware"
	"github.com/example/portfolio-crud/internal/model"
	"github.com/example/portfolio-crud/internal/observability"
	"github.com/example/portfolio-crud/internal/repository"
	"github.com/example/portfolio-crud/internal/service"
	"github.com/example/portfolio-crud/internal/storage"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type App struct {
	Cfg          config.Config
	DB           *pgxpool.Pool
	Projects     *service.ProjectService
	ProjectRepo  *repository.ProjectRepository
	Users        *repository.UserRepository
	Sessions     *auth.SessionStore
	Storage      *storage.Local
	Metrics      *observability.Metrics
	Logger       *slog.Logger
	Templates    map[string]*template.Template
	LoginLimiter *middleware.Limiter
	AdminLimiter *middleware.Limiter
}
type pageData struct {
	Title       string
	Description string
	Projects    []model.Project
	Project     *model.Project
	Stats       model.ProjectStats
	Page        int
	Pages       int
	Error       string
	CSRFToken   string
	Form        model.ProjectInput
	CurrentPath string
	User        string
}

func New(cfg config.Config, db *pgxpool.Pool, metrics *observability.Metrics, logger *slog.Logger) (*App, error) {
	pr := &repository.ProjectRepository{DB: db}
	app := &App{Cfg: cfg, DB: db, Projects: &service.ProjectService{Repo: pr}, ProjectRepo: pr, Users: &repository.UserRepository{DB: db}, Sessions: &auth.SessionStore{DB: db, TTL: cfg.SessionTTL}, Storage: &storage.Local{Dir: cfg.UploadDir, MaxBytes: cfg.MaxUploadBytes}, Metrics: metrics, Logger: logger, Templates: map[string]*template.Template{}, LoginLimiter: middleware.NewLimiter(10, 5*time.Minute, cfg.TrustProxy), AdminLimiter: middleware.NewLimiter(60, time.Minute, cfg.TrustProxy)}
	if err := app.Storage.Init(); err != nil {
		return nil, err
	}
	base := filepath.Join(cfg.TemplateDir, "layouts", "base.html")
	pages := map[string]string{"home": "public/home.html", "projects": "public/projects.html", "project": "public/project.html", "login": "admin/login.html", "dashboard": "admin/dashboard.html", "admin-projects": "admin/projects.html", "project-form": "admin/project-form.html", "not-found": "errors/404.html", "error": "errors/500.html"}
	for key, rel := range pages {
		t, err := template.New("base.html").Funcs(template.FuncMap{"join": strings.Join, "initial": initial, "inc": func(n int) int { return n + 1 }, "dec": func(n int) int {
			if n > 1 {
				return n - 1
			}
			return 1
		}, "splitLines": splitLines}).ParseFiles(base, filepath.Join(cfg.TemplateDir, rel))
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", key, err)
		}
		app.Templates[key] = t
	}
	return app, nil
}

func initial(v string) string {
	if v == "" {
		return "P"
	}
	return string([]rune(v)[0])
}
func splitLines(v string) []string {
	lines := strings.Split(strings.ReplaceAll(v, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func (a *App) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-App-Instance", a.Cfg.AppInstance)
			next.ServeHTTP(w, r)
		})
	})
	r.Use(chimw.RequestID)
	r.Use(middleware.HostAllowlist(a.Cfg.AllowedHosts))
	r.Use(chimw.Recoverer)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.BodyLimit(10 << 20))
	r.Use(middleware.LoadSession(a.Sessions, a.Cfg.CookieSecure))
	r.Use(otelhttp.NewMiddleware("portfolio.http"))
	r.Use(observability.HTTPMiddleware(a.Metrics, a.Logger, a.Cfg.AppInstance))
	r.Use(middleware.RequestIDHeader)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.DB.Ping(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	r.Get("/metrics", a.Metrics.Handler().ServeHTTP)
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir(a.Cfg.StaticDir))))
	r.Get("/uploads/{name}", a.serveUpload)
	r.Get("/", a.home)
	r.Get("/projects", a.projects)
	r.Get("/projects/{slug}", a.project)
	r.Get("/admin/login", a.loginPage)
	r.With(a.LoginLimiter.Middleware).Post("/admin/login", a.login)
	r.Group(func(admin chi.Router) {
		admin.Use(middleware.RequireAdmin)
		admin.Get("/admin", a.dashboard)
		admin.Get("/admin/", a.dashboard)
		admin.With(a.AdminLimiter.Middleware).Post("/admin/logout", a.logout)
		admin.Get("/admin/projects", a.adminProjects)
		admin.Get("/admin/projects/new", a.newProject)
		admin.With(a.AdminLimiter.Middleware).Post("/admin/projects", a.createProject)
		admin.Get("/admin/projects/{id}/edit", a.editProject)
		admin.With(a.AdminLimiter.Middleware).Post("/admin/projects/{id}", a.updateProject)
		admin.With(a.AdminLimiter.Middleware).Post("/admin/projects/{id}/delete", a.deleteProject)
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		a.render(w, http.StatusNotFound, "not-found", pageData{Title: "Không tìm thấy trang", Description: "Trang bạn tìm không tồn tại."})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})
	return r
}
func (a *App) render(w http.ResponseWriter, status int, key string, data pageData) {
	data.CurrentPath = ""
	if data.Title == "" {
		data.Title = "Portfolio"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := a.Templates[key].ExecuteTemplate(w, "base", data); err != nil {
		a.Logger.Error("template render failed", "template", key, "error", err)
	}
}
func (a *App) home(w http.ResponseWriter, r *http.Request) {
	items, _, err := a.ProjectRepo.ListPublished(r.Context(), 6, 0)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, http.StatusOK, "home", pageData{Title: "Portfolio · Engineering with intent", Description: "Selected projects in backend engineering, infrastructure and automation.", Projects: items})
}
func (a *App) projects(w http.ResponseWriter, r *http.Request) {
	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			page = 1
		} else {
			page = n
		}
	}
	const limit = 9
	items, total, err := a.ProjectRepo.ListPublished(r.Context(), limit, (page-1)*limit)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	pages := (total + limit - 1) / limit
	if pages == 0 {
		pages = 1
	}
	if page > pages {
		a.render(w, http.StatusNotFound, "not-found", pageData{Title: "Không tìm thấy trang", Description: "Trang dự án không tồn tại."})
		return
	}
	a.render(w, http.StatusOK, "projects", pageData{Title: "Projects", Description: "Selected work and experiments.", Projects: items, Page: page, Pages: pages})
}
func (a *App) project(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, err := a.ProjectRepo.GetPublishedBySlug(r.Context(), slug)
	if errors.Is(err, repository.ErrNotFound) {
		a.render(w, http.StatusNotFound, "not-found", pageData{Title: "Không tìm thấy dự án", Description: "Dự án này không tồn tại hoặc chưa được công khai."})
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, http.StatusOK, "project", pageData{Title: p.Title, Description: p.Summary, Project: p})
}
func (a *App) serveUpload(w http.ResponseWriter, r *http.Request) {
	f, typ, err := a.Storage.Open(chi.URLParam(r, "name"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.Base(f.Name()), fileModTime(f), f)
}
func fileModTime(f *os.File) time.Time {
	if st, err := f.Stat(); err == nil {
		return st.ModTime()
	}
	return time.Now()
}
func (a *App) loginPage(w http.ResponseWriter, r *http.Request) {
	s := middleware.SessionFromContext(r.Context())
	if s != nil && s.UserID != nil {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if s == nil {
		created, err := a.Sessions.Create(r.Context(), nil)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		middleware.SetSessionCookie(w, created, a.Cfg.CookieSecure)
		s = created
	}
	a.render(w, http.StatusOK, "login", pageData{Title: "Admin sign in", CSRFToken: s.CSRFToken, Error: r.URL.Query().Get("error")})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	s := middleware.SessionFromContext(r.Context())
	if s == nil || !middleware.CSRFValid(r, s) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if len(username) > 80 || len(password) > 1024 {
		a.loginFailed(w, r)
		return
	}
	u, err := a.Users.GetByUsername(r.Context(), username)
	if err != nil || !auth.VerifyPassword(u.PasswordHash, password) {
		a.loginFailed(w, r)
		return
	}
	_ = a.Sessions.Delete(r.Context(), s.Token)
	fresh, err := a.Sessions.Create(r.Context(), &u.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	middleware.SetSessionCookie(w, fresh, a.Cfg.CookieSecure)
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
func (a *App) loginFailed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	s := middleware.SessionFromContext(r.Context())
	csrf := ""
	if s != nil {
		csrf = s.CSRFToken
	}
	a.render(w, http.StatusUnauthorized, "login", pageData{Title: "Admin sign in", CSRFToken: csrf, Error: "Tên đăng nhập hoặc mật khẩu không đúng."})
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	s := middleware.SessionFromContext(r.Context())
	if !middleware.CSRFValid(r, s) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return
	}
	if s != nil {
		if err := a.Sessions.Delete(r.Context(), s.Token); err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	middleware.ClearSessionCookie(w, a.Cfg.CookieSecure)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}
func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := a.ProjectRepo.Stats(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, http.StatusOK, "dashboard", pageData{Title: "Dashboard", Stats: stats, CSRFToken: csrfToken(r)})
}
func csrfToken(r *http.Request) string {
	if s := middleware.SessionFromContext(r.Context()); s != nil {
		return s.CSRFToken
	}
	return ""
}
func (a *App) adminProjects(w http.ResponseWriter, r *http.Request) {
	page := 1
	if n, e := strconv.Atoi(r.URL.Query().Get("page")); e == nil && n > 0 {
		page = n
	}
	const limit = 20
	items, total, err := a.ProjectRepo.ListAdmin(r.Context(), limit, (page-1)*limit)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	pages := (total + limit - 1) / limit
	if pages == 0 {
		pages = 1
	}
	a.render(w, http.StatusOK, "admin-projects", pageData{Title: "Manage projects", Projects: items, Page: page, Pages: pages, CSRFToken: csrfToken(r)})
}
func (a *App) newProject(w http.ResponseWriter, r *http.Request) {
	a.render(w, http.StatusOK, "project-form", pageData{Title: "New project", CSRFToken: csrfToken(r), Form: model.ProjectInput{Status: model.StatusDraft}, Error: r.URL.Query().Get("error")})
}
func (a *App) editProject(w http.ResponseWriter, r *http.Request) {
	p, err := a.ProjectRepo.GetByID(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, repository.ErrNotFound) {
		a.render(w, http.StatusNotFound, "not-found", pageData{Title: "Không tìm thấy dự án"})
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, http.StatusOK, "project-form", pageData{Title: "Edit project", CSRFToken: csrfToken(r), Project: p, Form: model.ProjectInput{Title: p.Title, Slug: p.Slug, Summary: p.Summary, Description: p.Description, Technologies: p.Technologies, GitHubURL: p.GitHubURL, DemoURL: p.DemoURL, Status: p.Status, ThumbnailPath: p.ThumbnailPath}, Error: r.URL.Query().Get("error")})
}
func formInput(r *http.Request, thumbnail string) (model.ProjectInput, error) {
	if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return model.ProjectInput{}, err
	}
	status := model.ProjectStatus(r.FormValue("status"))
	techs := []string{}
	for _, v := range strings.Split(r.FormValue("technologies"), ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			techs = append(techs, v)
		}
	}
	return model.ProjectInput{Title: r.FormValue("title"), Slug: strings.TrimSpace(strings.ToLower(r.FormValue("slug"))), Summary: r.FormValue("summary"), Description: r.FormValue("description"), Technologies: techs, GitHubURL: strings.TrimSpace(r.FormValue("github_url")), DemoURL: strings.TrimSpace(r.FormValue("demo_url")), Status: status, ThumbnailPath: thumbnail}, nil
}
func (a *App) validateCSRF(w http.ResponseWriter, r *http.Request) bool {
	s := middleware.SessionFromContext(r.Context())
	if !middleware.CSRFValid(r, s) {
		http.Error(w, "Invalid CSRF token", http.StatusForbidden)
		return false
	}
	return true
}
func (a *App) uploadedImage(r *http.Request) (string, error) {
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	f, _, err := r.FormFile("thumbnail")
	if errors.Is(err, http.ErrMissingFile) || err == http.ErrNotMultipart {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	return a.Storage.Save(f)
}
func (a *App) createProject(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(w, r) {
		return
	}
	in, err := formInput(r, "")
	if err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	name, err := a.uploadedImage(r)
	if err != nil {
		a.render(w, http.StatusBadRequest, "project-form", pageData{Title: "New project", CSRFToken: csrfToken(r), Form: in, Error: "Ảnh không hợp lệ hoặc vượt quá giới hạn cho phép."})
		return
	}
	in.ThumbnailPath = name
	p, err := a.Projects.Create(r.Context(), in)
	if err != nil {
		_ = a.Storage.Remove(name)
		status, msg := safeProjectError(err)
		if status == http.StatusInternalServerError {
			a.Logger.Error("project create failed", "request_id", chimw.GetReqID(r.Context()), "error", err)
		}
		a.render(w, status, "project-form", pageData{Title: "New project", CSRFToken: csrfToken(r), Form: in, Error: msg})
		return
	}
	http.Redirect(w, r, "/admin/projects/"+p.ID+"/edit", http.StatusSeeOther)
}
func (a *App) updateProject(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(w, r) {
		return
	}
	id := chi.URLParam(r, "id")
	old, err := a.ProjectRepo.GetByID(r.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	in, err := formInput(r, old.ThumbnailPath)
	if err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	removeThumbnail := r.FormValue("remove_thumbnail") == "true"
	name, upErr := a.uploadedImage(r)
	if upErr != nil {
		a.render(w, http.StatusBadRequest, "project-form", pageData{Title: "Edit project", Project: old, CSRFToken: csrfToken(r), Form: in, Error: "Ảnh không hợp lệ hoặc vượt quá giới hạn cho phép."})
		return
	}
	if name != "" {
		in.ThumbnailPath = name
	} else if removeThumbnail {
		in.ThumbnailPath = ""
	}
	_, err = a.Projects.Update(r.Context(), id, in)
	if err != nil {
		if name != "" {
			_ = a.Storage.Remove(name)
		}
		status, msg := safeProjectError(err)
		if status == http.StatusInternalServerError {
			a.Logger.Error("project update failed", "request_id", chimw.GetReqID(r.Context()), "error", err)
		}
		a.render(w, status, "project-form", pageData{Title: "Edit project", Project: old, CSRFToken: csrfToken(r), Form: in, Error: msg})
		return
	}
	if (name != "" || removeThumbnail) && old.ThumbnailPath != "" {
		_ = a.Storage.Remove(old.ThumbnailPath)
	}
	http.Redirect(w, r, "/admin/projects/"+id+"/edit", http.StatusSeeOther)
}
func (a *App) deleteProject(w http.ResponseWriter, r *http.Request) {
	if !a.validateCSRF(w, r) {
		return
	}
	name, err := a.ProjectRepo.Delete(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, repository.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if name != "" {
		if err := a.Storage.Remove(name); err != nil {
			a.Logger.Warn("could not remove orphaned thumbnail", "error", err)
		}
	}
	http.Redirect(w, r, "/admin/projects", http.StatusSeeOther)
}
func safeProjectError(err error) (int, string) {
	if errors.Is(err, repository.ErrSlugConflict) {
		return http.StatusConflict, "Slug đã tồn tại. Hãy chọn một slug khác."
	}
	if errors.Is(err, repository.ErrInvalid) {
		return http.StatusBadRequest, "Dữ liệu dự án không hợp lệ. Vui lòng kiểm tra lại các trường."
	}
	var validation *service.ValidationError
	if errors.As(err, &validation) {
		return http.StatusBadRequest, validation.Field + ": " + validation.Message
	}
	return http.StatusInternalServerError, "Không thể lưu dự án lúc này. Vui lòng thử lại sau."
}

func (a *App) serverError(w http.ResponseWriter, r *http.Request, err error) {
	a.Logger.Error("request failed", "request_id", chimw.GetReqID(r.Context()), "error", err)
	a.render(w, http.StatusInternalServerError, "error", pageData{Title: "Có lỗi xảy ra", Description: "Máy chủ chưa thể xử lý yêu cầu. Vui lòng thử lại sau."})
}
func CheckDatabase(ctx context.Context, db *pgxpool.Pool) error { return db.Ping(ctx) }
func ParsePage(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 1
	}
	return n
}
