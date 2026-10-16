package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr              string
	DatabaseURL       string
	SessionTTL        time.Duration
	CookieSecure      bool
	AllowedHosts      []string
	UploadDir         string
	MaxUploadBytes    int64
	TemplateDir       string
	StaticDir         string
	OTLPEndpoint      string
	OTLPSamplingRatio float64
	ServiceName       string
	AppInstance       string
	PublicBaseURL     string
	TrustProxy        bool
}

func Load() (Config, error) {
	port := getenv("PORT", "8080")
	if _, err := strconv.Atoi(port); err != nil {
		return Config{}, fmt.Errorf("PORT must be numeric: %w", err)
	}
	c := Config{
		Addr:              ":" + port,
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		SessionTTL:        12 * time.Hour,
		CookieSecure:      getenv("COOKIE_SECURE", "false") == "true",
		AllowedHosts:      splitCSV(getenv("ALLOWED_HOSTS", "localhost,127.0.0.1,nginx")),
		UploadDir:         getenv("UPLOAD_DIR", "./uploads"),
		MaxUploadBytes:    5 << 20,
		TemplateDir:       getenv("TEMPLATE_DIR", "web/templates"),
		StaticDir:         getenv("STATIC_DIR", "web/static"),
		OTLPEndpoint:      getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317"),
		OTLPSamplingRatio: 1.0,
		ServiceName:       getenv("OTEL_SERVICE_NAME", "portfolio-app"),
		AppInstance:       getenv("APP_INSTANCE", "local"),
		PublicBaseURL:     strings.TrimRight(getenv("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		TrustProxy:        getenv("TRUST_PROXY_HEADERS", "false") == "true",
	}
	if v := os.Getenv("SESSION_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Minute {
			return Config{}, fmt.Errorf("SESSION_TTL must be a duration >= 1m")
		}
		c.SessionTTL = d
	}
	if v := os.Getenv("MAX_UPLOAD_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1024 || n > 20<<20 {
			return Config{}, fmt.Errorf("MAX_UPLOAD_BYTES must be between 1 KiB and 20 MiB")
		}
		c.MaxUploadBytes = n
	}
	if v := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); v != "" {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil || n < 0 || n > 1 {
			return Config{}, fmt.Errorf("OTEL_TRACES_SAMPLER_ARG must be between 0 and 1")
		}
		c.OTLPSamplingRatio = n
	}
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return Config{}, fmt.Errorf("DATABASE_URL must be a valid postgres:// URL")
	}
	return c, nil
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func splitCSV(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(strings.ToLower(v))
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
