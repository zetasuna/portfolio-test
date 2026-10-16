package middleware

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/example/portfolio-crud/internal/auth"
	"github.com/go-chi/chi/v5/middleware"
)

const SessionCookie = "portfolio_session"

type sessionKey struct{}

func SessionFromContext(ctx context.Context) *auth.Session {
	v, _ := ctx.Value(sessionKey{}).(*auth.Session)
	return v
}
func WithSession(ctx context.Context, s *auth.Session) context.Context {
	return context.WithValue(ctx, sessionKey{}, s)
}

func LoadSession(store *auth.SessionStore, secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
				if s, err := store.Get(r.Context(), c.Value); err == nil {
					r = r.WithContext(WithSession(r.Context(), s))
				} else if err != auth.ErrSessionNotFound {
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
func SetSessionCookie(w http.ResponseWriter, s *auth.Session, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: s.Token, Path: "/", Expires: s.ExpiresAt, MaxAge: int(time.Until(s.ExpiresAt).Seconds()), HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}
func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}
func CSRFValid(r *http.Request, s *auth.Session) bool {
	if s == nil || s.CSRFToken == "" {
		return false
	}
	token := r.FormValue("csrf_token")
	if token == "" {
		token = r.Header.Get("X-CSRF-Token")
	}
	return len(token) == len(s.CSRFToken) && subtle.ConstantTimeCompare([]byte(token), []byte(s.CSRFToken)) == 1
}
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; img-src 'self'; style-src 'self'; script-src 'self'; object-src 'none'")
		next.ServeHTTP(w, r)
	})
}
func BodyLimit(max int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, max)
			next.ServeHTTP(w, r)
		})
	}
}

type bucket struct {
	start time.Time
	count int
}
type Limiter struct {
	mu         sync.Mutex
	buckets    map[string]bucket
	Limit      int
	Window     time.Duration
	TrustProxy bool
}

func NewLimiter(limit int, window time.Duration, trustProxy bool) *Limiter {
	return &Limiter{buckets: map[string]bucket{}, Limit: limit, Window: window, TrustProxy: trustProxy}
}
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := clientIP(r, l.TrustProxy)
		now := time.Now()
		l.mu.Lock()
		b, ok := l.buckets[key]
		if !ok || now.Sub(b.start) >= l.Window {
			b = bucket{start: now, count: 0}
		}
		b.count++
		l.buckets[key] = b
		// Opportunistically prune expired keys to keep memory bounded.
		if len(l.buckets) > 10000 {
			for k, v := range l.buckets {
				if now.Sub(v.start) >= l.Window {
					delete(l.buckets, k)
				}
			}
		}
		allowed := b.count <= l.Limit
		l.mu.Unlock()
		if !allowed {
			w.Header().Set("Retry-After", stringInt(max(1, int(l.Window.Seconds()))))
			http.Error(w, "Too many requests. Please try again later.", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func stringInt(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := SessionFromContext(r.Context())
		if s == nil || s.UserID == nil || *s.UserID == "" {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func RequestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := middleware.GetReqID(r.Context()); id != "" {
			w.Header().Set("X-Request-ID", id)
		}
		next.ServeHTTP(w, r)
	})
}

// HostAllowlist blocks unrecognized Host headers to reduce host-header poisoning risk.
func HostAllowlist(allowed []string) func(http.Handler) http.Handler {
	set := map[string]struct{}{}
	for _, host := range allowed {
		set[strings.ToLower(strings.TrimSpace(host))] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := strings.ToLower(r.Host)
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			if _, ok := set[host]; !ok {
				http.Error(w, "Invalid host", http.StatusMisdirectedRequest)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
