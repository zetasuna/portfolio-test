package integration

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/example/portfolio-crud/internal/model"
	"github.com/example/portfolio-crud/internal/repository"
	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	abs, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	source := (&url.URL{Scheme: "file", Path: abs}).String()
	m, err := migrate.New(source, dsn)
	if err != nil {
		t.Fatal(err)
	}
	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	_, _ = m.Close()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	return pool
}
func TestProjectCRUDAndDuplicateSlug(t *testing.T) {
	db := openDB(t)
	repo := repository.ProjectRepository{DB: db}
	slug := "integration-" + fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	in := model.ProjectInput{Title: "Integration test", Slug: slug, Summary: "A summary for integration test", Description: "A description long enough to be valid", Technologies: []string{"Go", "PostgreSQL"}, Status: model.StatusPublished}
	p, err := repo.Create(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repo.Delete(context.Background(), p.ID) })
	if _, err = repo.Create(context.Background(), in); !errors.Is(err, repository.ErrSlugConflict) {
		t.Fatalf("expected duplicate slug error, got %v", err)
	}
	got, err := repo.GetByID(context.Background(), p.ID)
	if err != nil || got.Slug != slug {
		t.Fatalf("get project failed: got=%+v err=%v", got, err)
	}
	in.Title = "Updated integration project"
	in.Status = model.StatusDraft
	if _, err = repo.Update(context.Background(), p.ID, in); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.GetPublishedBySlug(context.Background(), slug); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("draft must not be public, got error %v", err)
	}
	rows, total, err := repo.ListAdmin(context.Background(), 20, 0)
	if err != nil || total < 1 || len(rows) == 0 {
		t.Fatalf("admin list failed: total=%d len=%d err=%v", total, len(rows), err)
	}
	if _, err = repo.Delete(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.GetByID(context.Background(), p.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("deleted project still exists: %v", err)
	}
}
func TestDatabaseErrorsAreReturned(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	repo := repository.ProjectRepository{DB: db}
	db.Close()
	if _, _, err := repo.ListAdmin(context.Background(), 10, 0); err == nil {
		t.Fatal("expected closed database pool to return an error")
	}
}
