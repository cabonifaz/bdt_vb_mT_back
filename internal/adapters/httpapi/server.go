package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

type Config struct {
	CORSOrigins  []string
	TrustProxy   bool
	CookieSecure bool
	MaxUploadMB  int64
	RateGeneral  int
	RateAuth     int
}

type Deps struct {
	Auth         *app.AuthService
	Tenants      *app.TenantService
	Users        *app.UserService
	ApiKeys      *app.ApiKeyService
	Masters      *app.MasterService
	Files        *app.FileService
	Audit        ports.AuditRepository
	Translations ports.TranslationRepository
	Tokens       ports.TokenIssuer
	Health       func() error
}

type Server struct {
	cfg    Config
	log    *slog.Logger
	tokens ports.TokenIssuer
	Deps
	apiKeys *app.ApiKeyService
}

func NewServer(cfg Config, log *slog.Logger, d Deps) *Server {
	return &Server{cfg: cfg, log: log, tokens: d.Tokens, Deps: d, apiKeys: d.ApiKeys}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(s.withRequestID, s.recoverer, securityHeaders, s.cors)

	general := newLimiter(s.cfg.RateGeneral, s.cfg.RateGeneral/5+1) // por IP
	authLim := newLimiter(s.cfg.RateAuth, s.cfg.RateAuth/2+1)       // login, refresh, invitaciones
	integ := newLimiter(300, 60)

	r.Get("/health", s.health)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.rateLimit(general))

		// ---- Público ----
		r.Get("/public/tenants/{slug}", s.publicTenant)
		r.Get("/public/tenants/{slug}/translations/{locale}", s.publicTranslations)
		r.Get("/public/files/{id}", s.publicFile)

		r.Group(func(r chi.Router) {
			r.Use(s.rateLimit(authLim))
			r.Post("/auth/login", s.login)
			r.Post("/auth/refresh", s.refresh)
			r.Post("/auth/logout", s.logout)
			r.Get("/invitations/{token}", s.invitationInfo)
			r.Post("/invitations/{token}/accept", s.acceptInvitation)
		})

		// ---- Autenticado ----
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/auth/me", s.me)
			r.Get("/masters/tables", s.masterTables)
			r.Get("/masters/items", s.masterItems)

			// Administración del tenant (usuario de un tenant)
			r.Route("/tenant", func(r chi.Router) {
				r.Use(s.requireTenantUser)
				r.With(s.requirePerm("tenant.branding.manage")).Put("/branding", s.updateBranding)
				r.With(s.requirePerm("tenant.branding.manage")).Post("/branding/files/{kind}", s.uploadBrandingFile)
				r.With(s.requirePerm("tenant.masters.manage")).Put("/masters/items", s.upsertMasterItem)
				r.With(s.requirePerm("tenant.users.manage")).Get("/users", s.listUsers)
				r.With(s.requirePerm("tenant.users.manage")).Patch("/users/{id}/status", s.setUserStatus)
				r.With(s.requirePerm("tenant.users.invite")).Get("/invitations", s.listInvitations)
				r.With(s.requirePerm("tenant.users.invite")).Post("/invitations", s.createInvitation)
				r.With(s.requirePerm("tenant.users.invite")).Delete("/invitations/{id}", s.revokeInvitation)
				r.With(s.requirePerm("tenant.apikeys.request")).Get("/api-keys", s.listApiKeys)
				r.With(s.requirePerm("tenant.apikeys.request")).Post("/api-keys", s.createApiKey)
				r.With(s.requirePerm("tenant.apikeys.request")).Post("/api-keys/{id}/revoke", s.revokeApiKey)
				r.With(s.requirePerm("tenant.translations.manage")).Put("/translations", s.upsertTranslation)
				r.With(s.requirePerm("tenant.audit.read")).Get("/audit", s.listAudit)
			})

			// Plataforma (Super Admin)
			r.Route("/platform", func(r chi.Router) {
				r.Use(s.requirePerm("platform.tenants.manage"))
				r.Get("/tenants", s.listTenants)
				r.Post("/tenants", s.createTenant)
				r.Patch("/tenants/{id}/status", s.setTenantStatus)
				r.Get("/users", s.listUsers)
				r.Post("/invitations", s.createInvitation)
				r.Get("/invitations", s.listInvitations)
				r.With(s.requirePerm("platform.apikeys.approve")).Get("/api-keys", s.listApiKeys)
				r.With(s.requirePerm("platform.apikeys.approve")).Patch("/api-keys/{id}/status", s.setApiKeyStatus)
				r.With(s.requirePerm("platform.masters.manage")).Put("/masters/items", s.upsertMasterItem)
				r.With(s.requirePerm("platform.audit.read")).Get("/audit", s.listAudit)
			})
		})
	})

	// ---- API de integración (solo con API key habilitada por el Super Admin) ----
	r.Get("/integration/docs", s.integrationDocs)
	r.Get("/integration/docs.js", s.integrationDocsJS)
	r.Route("/integration/v1", func(r chi.Router) {
		r.Use(s.rateLimit(integ), s.requireApiKey)
		r.Get("/openapi.json", s.openAPISpec)
		r.Get("/whoami", s.integrationWhoAmI)
		r.With(s.requireScope("masters.read")).Get("/masters", s.integrationMasters)
	})
	return r
}

func NewHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr: addr, Handler: h,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second,
		WriteTimeout: 120 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 1 << 16,
	}
}
