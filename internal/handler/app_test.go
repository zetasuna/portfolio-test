package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/example/portfolio-crud/internal/config"
	"github.com/example/portfolio-crud/internal/observability"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestApp(t *testing.T) (*App, *pgxpool.Pool) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate handler test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../.."))
	db, err := pgxpool.New(context.Background(), "postgres://test:test@127.0.0.1:1/test?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	cfg := config.Config{
		Addr:              ":8080",
		DatabaseURL:       "postgres://test:test@127.0.0.1:1/test?sslmode=disable&connect_timeout=1",
		SessionTTL:        time.Hour,
		AllowedHosts:      []string{"portfolio.test"},
		UploadDir:         t.TempDir(),
		MaxUploadBytes:    5 << 20,
		TemplateDir:       filepath.Join(root, "web", "templates"),
		StaticDir:         filepath.Join(root, "web", "static"),
		OTLPEndpoint:      "127.0.0.1:4317",
		OTLPSamplingRatio: 1,
		ServiceName:       "portfolio-test",
		AppInstance:       "test",
	}
	app, err := New(cfg, db, observability.NewMetrics(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return app, db
}

func TestHealthEndpointsAndMetrics(t *testing.T) {
	app, _ := newTestApp(t)
	h := app.Router()

	t.Run("liveness", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		r.Host = "portfolio.test"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "ok" {
			t.Fatalf("unexpected liveness response: status=%d body=%q", w.Code, w.Body.String())
		}
		if got := w.Header().Get("X-App-Instance"); got != "test" {
			t.Fatalf("expected app instance response header, got %q", got)
		}
	})

	t.Run("readiness reports database failure", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		r.Host = "portfolio.test"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 because test database is unavailable, got %d", w.Code)
		}
	})

	t.Run("metrics endpoint is instrumented", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		r.Host = "portfolio.test"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("expected metrics endpoint 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "go_goroutines") {
			t.Fatal("expected Go runtime metrics")
		}
	})

	t.Run("admin is protected without session", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/admin/projects", nil)
		r.Host = "portfolio.test"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/admin/login" {
			t.Fatalf("expected redirect to login, got status=%d location=%q", w.Code, w.Header().Get("Location"))
		}
	})

}
