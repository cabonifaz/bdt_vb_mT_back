package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

const refreshCookie = "bdt_rt"

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.Health(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) client(r *http.Request) app.Client {
	return app.Client{IP: s.clientIP(r), UserAgent: r.UserAgent()}
}

// ------------------------------------------------------------------ Auth

// setRefreshCookie: con "Recordarme" la cookie es persistente (sobrevive al cierre del navegador);
// sin él es una cookie de sesión del navegador.
func (s *Server) setRefreshCookie(w http.ResponseWriter, token string, exp time.Time, persistent bool) {
	c := &http.Cookie{Name: refreshCookie, Value: token, Path: "/api/v1/auth", HttpOnly: true, Secure: s.cfg.CookieSecure}
	if persistent {
		c.Expires = exp
	}
	if s.cfg.CookieSecure {
		c.SameSite = http.SameSiteNoneMode // front y API en dominios distintos (Railway)
	} else {
		c.SameSite = http.SameSiteLaxMode
	}
	if token == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

// sameOriginGuard: las operaciones con cookie exigen un Origin de la lista blanca (anti-CSRF).
func (s *Server) originAllowed(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return false
	}
	for _, a := range s.cfg.CORSOrigins {
		if a == o {
			return true
		}
	}
	return false
}

type sessionOut struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
	User        meOut     `json:"user"`
}

type meOut struct {
	ID          string   `json:"id"`
	FullName    string   `json:"fullName"`
	Email       string   `json:"email"`
	Locale      string   `json:"locale"`
	TenantSlug  string   `json:"tenantSlug,omitempty"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	IsPlatform  bool     `json:"isPlatform"`
}

func toMe(p *domain.Principal) meOut {
	return meOut{ID: p.UserUUID, FullName: p.FullName, Email: p.Email, Locale: p.Locale, TenantSlug: p.TenantSlug,
		Roles: p.Roles, Permissions: p.Perms, IsPlatform: p.IsPlatform()}
}

func (s *Server) writeSession(w http.ResponseWriter, sess *app.Session) {
	s.setRefreshCookie(w, sess.RefreshToken, sess.RefreshExp, sess.Remember)
	writeJSON(w, http.StatusOK, sessionOut{AccessToken: sess.AccessToken, ExpiresAt: sess.ExpiresAt, User: toMe(sess.User)})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Tenant   string `json:"tenant"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	sess, err := s.Auth.Login(r.Context(), in.Tenant, in.Email, in.Password, in.Remember, s.client(r))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.writeSession(w, sess)
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	if !s.originAllowed(r) {
		s.writeErr(w, r, domain.ErrForbidden)
		return
	}
	c, err := r.Cookie(refreshCookie)
	if err != nil {
		s.writeErr(w, r, domain.ErrUnauthorized)
		return
	}
	sess, err := s.Auth.Refresh(r.Context(), c.Value, s.client(r))
	if err != nil {
		s.setRefreshCookie(w, "", time.Time{}, false)
		s.writeErr(w, r, err)
		return
	}
	s.writeSession(w, sess)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookie); err == nil && s.originAllowed(r) {
		_ = s.Auth.Logout(r.Context(), c.Value)
	}
	s.setRefreshCookie(w, "", time.Time{}, false)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p, err := s.Auth.Principal(r.Context(), principal(r).UserID)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toMe(p))
}

// ------------------------------------------------------------------ Público

func (s *Server) publicTenant(w http.ResponseWriter, r *http.Request) {
	b, err := s.Tenants.PublicBranding(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache") // el branding cambia al guardarse: siempre revalidar
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) publicTranslations(w http.ResponseWriter, r *http.Request) {
	b, err := s.Tenants.PublicBranding(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	locale := chi.URLParam(r, "locale")
	if locale != "es" && locale != "en" {
		s.writeErr(w, r, domain.ErrNotFound)
		return
	}
	m, err := s.Translations.List(r.Context(), b.TenantID, locale)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache") // el branding cambia al guardarse: siempre revalidar
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) publicFile(w http.ResponseWriter, r *http.Request) {
	f, rc, err := s.Files.OpenPublic(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	defer rc.Close()
	h := w.Header()
	h.Set("Content-Type", f.MimeType)
	h.Set("Content-Length", strconv.FormatInt(f.Size, 10))
	h.Set("Cache-Control", "public, max-age=86400, immutable")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("ETag", `"`+f.SHA256+`"`)
	_, _ = io.Copy(w, rc)
}

// ------------------------------------------------------------------ Invitaciones (públicas)

func (s *Server) invitationInfo(w http.ResponseWriter, r *http.Request) {
	inv, err := s.Users.InvitationInfo(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (s *Server) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		FullName string `json:"fullName"`
		Password string `json:"password"`
		Locale   string `json:"locale"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if _, err := s.Users.AcceptInvitation(r.Context(), chi.URLParam(r, "token"), in.FullName, in.Password, in.Locale, s.clientIP(r)); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ------------------------------------------------------------------ Maestros

func (s *Server) masterTables(w http.ResponseWriter, r *http.Request) {
	t, err := s.Masters.Tables(r.Context())
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) masterItems(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	inactive := r.URL.Query().Get("includeInactive") == "true" && (p.Has("tenant.masters.manage") || p.Has("platform.masters.manage"))
	items, err := s.Masters.Items(r.Context(), p.TenantID, r.URL.Query().Get("tables"), inactive)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type masterIn struct {
	Table     string          `json:"table"`
	ID        string          `json:"id"`
	Code      string          `json:"code"`
	Label     json.RawMessage `json:"label"`
	ParentID  *string         `json:"parentId"`
	Num1      *float64        `json:"num1"`
	Num2      *float64        `json:"num2"`
	Num3      *float64        `json:"num3"`
	Str1      *string         `json:"str1"`
	Str2      *string         `json:"str2"`
	Str3      *string         `json:"str3"`
	SortOrder int             `json:"sortOrder"`
	IsActive  *bool           `json:"isActive"`
}

func (s *Server) upsertMasterItem(w http.ResponseWriter, r *http.Request) {
	var in masterIn
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	p := principal(r)
	active := in.IsActive == nil || *in.IsActive
	// /platform => maestro global (tenant nil); /tenant => propio del tenant
	var tenantID *uint64
	if strings.Contains(r.URL.Path, "/tenant/") {
		tenantID = p.TenantID
	}
	it, err := s.Masters.Upsert(r.Context(), p, s.clientIP(r), ports.MasterUpsert{
		TenantID: tenantID, TableCode: in.Table, UUID: in.ID, Code: in.Code, Label: in.Label, ParentUUID: in.ParentID,
		Num1: in.Num1, Num2: in.Num2, Num3: in.Num3, Str1: in.Str1, Str2: in.Str2, Str3: in.Str3,
		SortOrder: in.SortOrder, IsActive: active,
	})
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

// ------------------------------------------------------------------ Branding

func (s *Server) updateBranding(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DisplayName       string `json:"displayName"`
		ColorPrimary      string `json:"colorPrimary"`
		ColorPrimaryHover string `json:"colorPrimaryHover"`
		ColorSecondary    string `json:"colorSecondary"`
		ColorAccent       string `json:"colorAccent"`
		ColorSidebar      string `json:"colorSidebar"`
		FontFamily        string `json:"fontFamily"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	p := principal(r)
	b, err := s.Tenants.UpdateBranding(r.Context(), p, s.clientIP(r), *p.TenantID, domain.Branding{
		DisplayName: in.DisplayName, ColorPrimary: in.ColorPrimary, ColorPrimaryHov: in.ColorPrimaryHover,
		ColorSecondary: in.ColorSecondary, ColorAccent: in.ColorAccent, ColorSidebar: in.ColorSidebar, FontFamily: in.FontFamily,
	})
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) updateCVTemplate(w http.ResponseWriter, r *http.Request) {
	var in ports.CVTemplateUpdate
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	b, err := s.Tenants.UpdateCVTemplate(r.Context(), principal(r), s.clientIP(r), in)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) uploadBrandingFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 3<<20)
	if err := r.ParseMultipartForm(3 << 20); err != nil {
		s.writeErr(w, r, domain.NewError("FILE_TOO_LARGE"))
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		s.writeErr(w, r, domain.ErrValidation)
		return
	}
	defer f.Close()
	b, err := s.Files.UploadBranding(r.Context(), principal(r), s.clientIP(r), strings.ToUpper(chi.URLParam(r, "kind")), hdr.Filename, f)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// ------------------------------------------------------------------ Usuarios e invitaciones

// scope: en /tenant/* siempre el tenant del usuario; en /platform/* el Super Admin puede
// actuar sobre un tenant con ?tenantId=<uuid>, o sin él sobre la plataforma.
func (s *Server) scope(w http.ResponseWriter, r *http.Request) (*uint64, string, bool) {
	p := principal(r)
	if strings.Contains(r.URL.Path, "/tenant/") {
		return p.TenantID, p.TenantSlug, true
	}
	if t := r.URL.Query().Get("tenantId"); t != "" {
		id, name, err := s.Tenants.Resolve(r.Context(), p, t)
		if err != nil {
			s.writeErr(w, r, err)
			return nil, "", false
		}
		return &id, name, true
	}
	return nil, "Banco de Talentos", true
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := s.scope(w, r)
	if !ok {
		return
	}
	page, err := s.Users.List(r.Context(), principal(r), tenant, r.URL.Query().Get("status"), qInt(r, "page", 1), qInt(r, "pageSize", 20))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) setUserStatus(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := s.scope(w, r)
	if !ok {
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if err := s.Users.SetStatus(r.Context(), principal(r), s.clientIP(r), tenant, chi.URLParam(r, "id"), in.Status); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createInvitation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email  string `json:"email"`
		Role   string `json:"role"`
		Locale string `json:"locale"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	p := principal(r)
	tenant, name, ok := s.scope(w, r)
	if !ok {
		return
	}
	res, err := s.Users.Invite(r.Context(), p, s.clientIP(r), tenant, in.Email, strings.ToUpper(in.Role), name, in.Locale)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (s *Server) listInvitations(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := s.scope(w, r)
	if !ok {
		return
	}
	list, err := s.Users.ListInvitations(r.Context(), principal(r), tenant)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := s.scope(w, r)
	if !ok {
		return
	}
	if err := s.Users.RevokeInvitation(r.Context(), principal(r), s.clientIP(r), tenant, chi.URLParam(r, "id")); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ------------------------------------------------------------------ Tenants (SA)

func (s *Server) listTenants(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := s.Tenants.List(r.Context(), principal(r), q.Get("search"), q.Get("status"), qInt(r, "page", 1), qInt(r, "pageSize", 20))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	var in app.NewTenant
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	t, inv, err := s.Tenants.Create(r.Context(), principal(r), s.clientIP(r), in)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"tenant": t, "adminInvitation": inv})
}

func (s *Server) setTenantStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	t, err := s.Tenants.SetStatus(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), in.Status)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// ------------------------------------------------------------------ API keys

func (s *Server) listApiKeys(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := s.scope(w, r)
	if !ok {
		return
	}
	list, err := s.ApiKeys.List(r.Context(), principal(r), tenant)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createApiKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	p := principal(r)
	k, err := s.ApiKeys.Create(r.Context(), p, s.clientIP(r), *p.TenantID, in.Name, in.Scopes, in.ExpiresAt)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, k)
}

func (s *Server) revokeApiKey(w http.ResponseWriter, r *http.Request) {
	k, err := s.ApiKeys.SetStatus(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), "REVOKED")
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

func (s *Server) setApiKeyStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	k, err := s.ApiKeys.SetStatus(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), strings.ToUpper(in.Status))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

// ------------------------------------------------------------------ Traducciones y auditoría

func (s *Server) upsertTranslation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Locale string `json:"locale"`
		Key    string `json:"key"`
		Value  string `json:"value"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if (in.Locale != "es" && in.Locale != "en") || in.Key == "" || len(in.Key) > 150 || len(in.Value) > 1000 {
		s.writeErr(w, r, domain.ErrValidation)
		return
	}
	p := principal(r)
	if err := s.Translations.Upsert(r.Context(), ports.Meta{ActorID: p.UserID, IP: s.clientIP(r)}, *p.TenantID, in.Locale, in.Key, in.Value); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	tenant, _, ok := s.scope(w, r)
	if !ok {
		return
	}
	p := principal(r)
	list, err := s.Audit.List(r.Context(), ports.Meta{ActorID: p.UserID}, tenant, qInt(r, "page", 1), qInt(r, "pageSize", 50))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
