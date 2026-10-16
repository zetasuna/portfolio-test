package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/example/portfolio-crud/internal/auth"
)

func TestCSRFValid(t *testing.T) {
	s := &auth.Session{CSRFToken: "abc123"}
	form := url.Values{"csrf_token": []string{"abc123"}}
	r := httptest.NewRequest(http.MethodPost, "/admin/projects", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if !CSRFValid(r, s) {
		t.Fatal("expected valid CSRF token")
	}
}
func TestCSRFRejectsMissingOrWrongToken(t *testing.T) {
	s := &auth.Session{CSRFToken: "expected"}
	for _, body := range []string{"", "csrf_token=wrong"} {
		r := httptest.NewRequest(http.MethodPost, "/admin/projects", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if CSRFValid(r, s) {
			t.Fatalf("accepted invalid token for body %q", body)
		}
	}
}
func TestLoginLimiterReturns429(t *testing.T) {
	l := NewLimiter(2, time.Minute, false)
	h := l.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for i := 0; i < 3; i++ {
		r := httptest.NewRequest(http.MethodPost, "/admin/login", nil)
		r.RemoteAddr = "192.0.2.4:12345"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if i < 2 && w.Code != http.StatusNoContent {
			t.Fatalf("request %d: got %d", i, w.Code)
		}
		if i == 2 && w.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d", w.Code)
		}
	}
}
func TestHostAllowlist(t *testing.T) {
	h := HostAllowlist([]string{"localhost", "app1"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, tc := range []struct {
		host string
		want int
	}{{"localhost:8080", 204}, {"app1:8080", 204}, {"evil.example", 421}} {
		r := httptest.NewRequest(http.MethodGet, "http://"+tc.host+"/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("Host %q: got %d want %d", tc.host, w.Code, tc.want)
		}
	}
}
