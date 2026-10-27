package httpapi

import (
	"context"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/tokens"
)

type ctxKey int

const (
	keyRequestID ctxKey = iota
	keyPrincipal
	keyApiClient
)

func requestID(r *http.Request) string {
	v, _ := r.Context().Value(keyRequestID).(string)
	return v
}

func principal(r *http.Request) *domain.Principal {
	p, _ := r.Context().Value(keyPrincipal).(*domain.Principal)
	return p
}

func apiClient(r *http.Request) *domain.ApiClient {
	c, _ := r.Context().Value(keyApiClient).(*domain.ApiClient)
	return c
}

func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := tokens.Random(9)
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), keyRequestID, id)))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.log.Error("panic", "value", v, "path", r.URL.Path, "rid", requestID(r))
				s.writeErr(w, r, domain.ErrInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// securityHeaders: la API solo sirve JSON; la página de docs define su propia CSP.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// cors: solo orígenes de la lista blanca (CORS_ORIGINS). Nunca "*" con credenciales.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && slices.Contains(s.cfg.CORSOrigins, origin) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Vary", "Origin")
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Api-Key, X-Client-Op-Id, Accept-Language")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		} else if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP: detrás del proxy de Railway se usa X-Real-IP (lo fija el edge); localmente RemoteAddr.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(ip) != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ---- Rate limiting por IP (token bucket en memoria) ----

type limiter struct {
	mu      sync.Mutex
	clients map[string]*visitor
	rate    rate.Limit
	burst   int
}

type visitor struct {
	l    *rate.Limiter
	seen time.Time
}

func newLimiter(perMinute, burst int) *limiter {
	lm := &limiter{clients: map[string]*visitor{}, rate: rate.Limit(float64(perMinute) / 60), burst: burst}
	go func() {
		for range time.Tick(5 * time.Minute) {
			lm.mu.Lock()
			for k, v := range lm.clients {
				if time.Since(v.seen) > 10*time.Minute {
					delete(lm.clients, k)
				}
			}
			lm.mu.Unlock()
		}
	}()
	return lm
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.clients[key]
	if !ok {
		v = &visitor{l: rate.NewLimiter(l.rate, l.burst)}
		l.clients[key] = v
	}
	v.seen = time.Now()
	return v.l.Allow()
}

func (s *Server) rateLimit(l *limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(s.clientIP(r)) {
				w.Header().Set("Retry-After", "60")
				s.writeErr(w, r, domain.ErrRateLimited)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---- Autenticación ----

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		tok, ok := strings.CutPrefix(h, "Bearer ")
		if !ok || tok == "" {
			s.writeErr(w, r, domain.ErrUnauthorized)
			return
		}
		c, err := s.tokens.ParseAccess(tok)
		if err != nil {
			s.writeErr(w, r, domain.ErrUnauthorized)
			return
		}
		p := &domain.Principal{UserID: c.UserID, UserUUID: c.UserUUID, TenantID: c.TenantID, TenantSlug: c.TenantSlug, TenantUUID: c.TenantUUID,
			Roles: c.Roles, Perms: c.Perms, Locale: c.Locale}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), keyPrincipal, p)))
	})
}

func (s *Server) requirePerm(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p := principal(r); p == nil || !p.Has(perm) {
				s.writeErr(w, r, domain.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// requireTenantUser: endpoints /tenant/* solo para usuarios que pertenecen a un tenant.
func (s *Server) requireTenantUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := principal(r); p == nil || p.TenantID == nil {
			s.writeErr(w, r, domain.ErrForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireApiKey: API pública de integración (header X-Api-Key).
func (s *Server) requireApiKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Api-Key")
		if key == "" || len(key) > 200 {
			s.writeErr(w, r, domain.NewError("APIKEY_INVALID"))
			return
		}
		c, err := s.apiKeys.Authenticate(r.Context(), key, s.clientIP(r))
		if err != nil {
			s.writeErr(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), keyApiClient, c)))
	})
}

func (s *Server) requireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c := apiClient(r); c == nil || !c.Can(scope) {
				s.writeErr(w, r, domain.NewError("APIKEY_SCOPE"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
