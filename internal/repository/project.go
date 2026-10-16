package repository

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/example/portfolio-crud/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var ErrNotFound = errors.New("record not found")
var ErrSlugConflict = errors.New("slug already exists")
var ErrInvalid = errors.New("invalid input")
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type ProjectRepository struct{ DB *pgxpool.Pool }

var projectTracer = otel.Tracer("portfolio/repository")

const projectColumns = `id::text,title,slug,summary,description,thumbnail_path,technologies,github_url,demo_url,status,created_at,updated_at`

func scanProject(row pgx.Row) (*model.Project, error) {
	p := &model.Project{}
	err := row.Scan(&p.ID, &p.Title, &p.Slug, &p.Summary, &p.Description, &p.ThumbnailPath, &p.Technologies, &p.GitHubURL, &p.DemoURL, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}
func mapDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrSlugConflict
	}
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		return ErrInvalid
	}
	return err
}

func (r *ProjectRepository) ListPublished(ctx context.Context, limit, offset int) ([]model.Project, int, error) {
	ctx, span := projectTracer.Start(ctx, "db.projects.list_published")
	defer span.End()
	span.SetAttributes(attribute.String("db.system", "postgresql"), attribute.String("db.operation", "SELECT"))
	var count int
	if err := r.DB.QueryRow(ctx, `SELECT count(*) FROM projects WHERE status='published'`).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := r.DB.Query(ctx, `SELECT `+projectColumns+` FROM projects WHERE status='published' ORDER BY updated_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []model.Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *p)
	}
	return items, count, rows.Err()
}
func (r *ProjectRepository) GetPublishedBySlug(ctx context.Context, slug string) (*model.Project, error) {
	ctx, span := projectTracer.Start(ctx, "db.projects.get_published")
	defer span.End()
	span.SetAttributes(attribute.String("db.system", "postgresql"), attribute.String("db.operation", "SELECT"))
	return scanProject(r.DB.QueryRow(ctx, `SELECT `+projectColumns+` FROM projects WHERE slug=$1 AND status='published'`, slug))
}
func (r *ProjectRepository) ListAdmin(ctx context.Context, limit, offset int) ([]model.Project, int, error) {
	ctx, span := projectTracer.Start(ctx, "db.projects.list_admin")
	defer span.End()
	span.SetAttributes(attribute.String("db.system", "postgresql"), attribute.String("db.operation", "SELECT"))
	var count int
	if err := r.DB.QueryRow(ctx, `SELECT count(*) FROM projects`).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := r.DB.Query(ctx, `SELECT `+projectColumns+` FROM projects ORDER BY updated_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []model.Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *p)
	}
	return items, count, rows.Err()
}
func (r *ProjectRepository) GetByID(ctx context.Context, id string) (*model.Project, error) {
	if !uuidPattern.MatchString(id) {
		return nil, ErrNotFound
	}
	ctx, span := projectTracer.Start(ctx, "db.projects.get")
	defer span.End()
	span.SetAttributes(attribute.String("db.system", "postgresql"), attribute.String("db.operation", "SELECT"))
	return scanProject(r.DB.QueryRow(ctx, `SELECT `+projectColumns+` FROM projects WHERE id=$1`, id))
}
func (r *ProjectRepository) Create(ctx context.Context, in model.ProjectInput) (*model.Project, error) {
	ctx, span := projectTracer.Start(ctx, "db.projects.create")
	defer span.End()
	span.SetAttributes(attribute.String("db.system", "postgresql"), attribute.String("db.operation", "INSERT"))
	p, err := scanProject(r.DB.QueryRow(ctx, `INSERT INTO projects(title,slug,summary,description,thumbnail_path,technologies,github_url,demo_url,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+projectColumns,
		in.Title, in.Slug, in.Summary, in.Description, in.ThumbnailPath, in.Technologies, in.GitHubURL, in.DemoURL, string(in.Status)))
	if err != nil {
		return nil, mapDBError(err)
	}
	return p, nil
}
func (r *ProjectRepository) Update(ctx context.Context, id string, in model.ProjectInput) (*model.Project, error) {
	if !uuidPattern.MatchString(id) {
		return nil, ErrNotFound
	}
	ctx, span := projectTracer.Start(ctx, "db.projects.update")
	defer span.End()
	span.SetAttributes(attribute.String("db.system", "postgresql"), attribute.String("db.operation", "UPDATE"))
	p, err := scanProject(r.DB.QueryRow(ctx, `UPDATE projects SET title=$2,slug=$3,summary=$4,description=$5,thumbnail_path=$6,technologies=$7,github_url=$8,demo_url=$9,status=$10,updated_at=now() WHERE id=$1 RETURNING `+projectColumns,
		id, in.Title, in.Slug, in.Summary, in.Description, in.ThumbnailPath, in.Technologies, in.GitHubURL, in.DemoURL, string(in.Status)))
	if err != nil {
		return nil, mapDBError(err)
	}
	return p, nil
}
func (r *ProjectRepository) Delete(ctx context.Context, id string) (string, error) {
	if !uuidPattern.MatchString(id) {
		return "", ErrNotFound
	}
	ctx, span := projectTracer.Start(ctx, "db.projects.delete")
	defer span.End()
	span.SetAttributes(attribute.String("db.system", "postgresql"), attribute.String("db.operation", "DELETE"))
	var path string
	err := r.DB.QueryRow(ctx, `DELETE FROM projects WHERE id=$1 RETURNING thumbnail_path`, id).Scan(&path)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return path, err
}
func (r *ProjectRepository) Stats(ctx context.Context) (model.ProjectStats, error) {
	ctx, span := projectTracer.Start(ctx, "db.projects.stats")
	defer span.End()
	span.SetAttributes(attribute.String("db.system", "postgresql"), attribute.String("db.operation", "SELECT"))
	var s model.ProjectStats
	err := r.DB.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE status='published'),count(*) FILTER (WHERE status='draft') FROM projects`).Scan(&s.Total, &s.Published, &s.Draft)
	return s, err
}
func CleanTechnologies(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if !seen[key] {
			seen[key] = true
			out = append(out, v)
		}
	}
	return out
}
