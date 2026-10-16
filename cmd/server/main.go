package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/example/portfolio-crud/internal/auth"
	"github.com/example/portfolio-crud/internal/config"
	"github.com/example/portfolio-crud/internal/handler"
	"github.com/example/portfolio-crud/internal/observability"
	"github.com/example/portfolio-crud/internal/repository"
	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if os.Args[1] == "healthcheck" {
		client := http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://127.0.0.1:" + envOr("PORT", "8080") + "/readyz")
		if err != nil || resp.StatusCode != http.StatusOK {
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
			} else {
				_ = resp.Body.Close()
				fmt.Fprintln(os.Stderr, "readiness endpoint is unhealthy")
			}
			os.Exit(1)
		}
		_ = resp.Body.Close()
		return
	}
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration invalid", "error", err)
		os.Exit(2)
	}
	ctx := context.Background()
	switch os.Args[1] {
	case "start":
		if err := runMigrations(cfg.DatabaseURL); err != nil {
			logger.Error("startup migration failed", "error", err)
			os.Exit(1)
		}
		if err := serve(cfg, logger); err != nil {
			logger.Error("server stopped with error", "error", err)
			os.Exit(1)
		}
	case "migrate":
		if err := runMigrations(cfg.DatabaseURL); err != nil {
			logger.Error("migration failed", "error", err)
			os.Exit(1)
		}
		logger.Info("database migrations are up to date")
	case "create-admin":
		if err := createAdmin(ctx, cfg, logger); err != nil {
			logger.Error("admin bootstrap failed", "error", err)
			os.Exit(1)
		}
	case "serve":
		if err := serve(cfg, logger); err != nil {
			logger.Error("server stopped with error", "error", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(2)
	}
}
func envOr(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func usage() {
	fmt.Fprintln(os.Stderr, "usage: portfolio {start|migrate|create-admin|serve|healthcheck}")
}
func runMigrations(databaseURL string) error {
	dir := os.Getenv("MIGRATIONS_DIR")
	if dir == "" {
		dir = "./migrations"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	source := "file://" + filepath.ToSlash(abs)
	m, err := migrate.New(source, databaseURL)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()
	err = m.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}
func createAdmin(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	username := strings.TrimSpace(os.Getenv("ADMIN_USERNAME"))
	password := os.Getenv("ADMIN_PASSWORD")
	if len(username) < 3 || len(username) > 80 {
		return fmt.Errorf("ADMIN_USERNAME must be 3–80 characters")
	}
	if strings.ContainsAny(username, " \t\r\n") {
		return fmt.Errorf("ADMIN_USERNAME must not contain whitespace")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	pool, err := connectDB(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	repo := repository.UserRepository{DB: pool}
	if err = repo.CreateOrUpdate(ctx, username, hash); err != nil {
		return err
	}
	logger.Info("admin account created or password rotated", "username", username)
	return nil
}
func connectDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pc, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	pc.MaxConns = 8
	pc.MinConns = 1
	if pc.ConnConfig.RuntimeParams == nil {
		pc.ConnConfig.RuntimeParams = map[string]string{}
	}
	pc.ConnConfig.RuntimeParams["statement_timeout"] = "5000"
	pc.ConnConfig.RuntimeParams["lock_timeout"] = "2000"
	pc.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "30000"
	pc.MaxConnLifetime = 30 * time.Minute
	pc.MaxConnIdleTime = 5 * time.Minute
	pc.HealthCheckPeriod = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return pool, nil
		}
		if time.Now().After(deadline) {
			pool.Close()
			return nil, fmt.Errorf("postgres remained unavailable: %w", err)
		}
		time.Sleep(time.Second)
	}
}
func cleanupExpiredSessions(ctx context.Context, store *auth.SessionStore, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := store.CleanupExpired(cleanupCtx); err != nil {
				logger.Warn("session cleanup failed", "error", err)
			}
			cancel()
		}
	}
}

func serve(cfg config.Config, logger *slog.Logger) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pool, err := connectDB(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	shutdownTracing, err := observability.InitTracing(ctx, cfg.OTLPEndpoint, cfg.ServiceName, cfg.AppInstance, cfg.OTLPSamplingRatio)
	if err != nil {
		logger.Error("tracing initialization failed; service will not start", "error", err)
		return err
	}
	metrics := observability.NewMetrics()
	app, err := handler.New(cfg, pool, metrics, logger)
	if err != nil {
		return err
	}
	go cleanupExpiredSessions(ctx, app.Sessions, logger)
	srv := &http.Server{Addr: cfg.Addr, Handler: app.Router(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.Addr, "instance", cfg.AppInstance)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		cancel()
		return err
	case <-ctx.Done():
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
		return err
	}
	traceCtx, traceCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer traceCancel()
	if err := shutdownTracing(traceCtx); err != nil {
		logger.Warn("trace exporter shutdown failed", "error", err)
	}
	logger.Info("server stopped gracefully")
	return nil
}

var _ = http.ErrServerClosed
