package app

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/tokens"
)

func meta(p *domain.Principal, ip string) ports.Meta { return ports.Meta{ActorID: p.UserID, IP: ip} }

// ---------------------------------------------------------------------
// Tenants y branding
// ---------------------------------------------------------------------

type TenantService struct {
	Repo    ports.TenantRepository
	Users   *UserService
	Files   ports.FileRepository
	Storage ports.FileStorage
}

type NewTenant struct {
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	LegalName  *string `json:"legalName"`
	TaxID      *string `json:"taxId"`
	Locale     string  `json:"defaultLocale"`
	Timezone   string  `json:"timezone"`
	AdminEmail string  `json:"adminEmail"`
}

// Create da de alta el tenant y, si se indica, invita a su primer administrador.
func (s *TenantService) Create(ctx context.Context, p *domain.Principal, ip string, in NewTenant) (*domain.Tenant, *InviteResult, error) {
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 150 {
		return nil, nil, domain.ErrValidation
	}
	t, err := s.Repo.Create(ctx, meta(p, ip), domain.Tenant{
		UUID: uuid.NewString(), Slug: in.Slug, Name: strings.TrimSpace(in.Name), LegalName: in.LegalName, TaxID: in.TaxID,
		DefaultLocale: in.Locale, Timezone: in.Timezone,
	})
	if err != nil {
		return nil, nil, err
	}
	var inv *InviteResult
	if in.AdminEmail != "" {
		inv, err = s.Users.Invite(ctx, p, ip, &t.ID, in.AdminEmail, domain.RoleTenantAdmin, t.Name, in.Locale)
		if err != nil {
			return t, nil, err
		}
	}
	return t, inv, nil
}

func (s *TenantService) List(ctx context.Context, p *domain.Principal, search, status string, page, size int) (*domain.Page[domain.Tenant], error) {
	return s.Repo.List(ctx, meta(p, ""), search, status, page, size)
}

func (s *TenantService) SetStatus(ctx context.Context, p *domain.Principal, ip, tenantUUID, status string) (*domain.Tenant, error) {
	return s.Repo.SetStatus(ctx, meta(p, ip), tenantUUID, status)
}

func (s *TenantService) PublicBranding(ctx context.Context, slug string) (*domain.Branding, error) {
	return s.Repo.PublicBySlug(ctx, strings.ToLower(slug))
}

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func (s *TenantService) UpdateBranding(ctx context.Context, p *domain.Principal, ip string, tenantID uint64, b domain.Branding) (*domain.Branding, error) {
	for _, c := range []string{b.ColorPrimary, b.ColorPrimaryHov, b.ColorSecondary, b.ColorAccent, b.ColorSidebar} {
		if !hexColor.MatchString(c) {
			return nil, domain.NewError("INVALID_COLOR")
		}
	}
	if strings.TrimSpace(b.DisplayName) == "" || len(b.DisplayName) > 150 {
		return nil, domain.ErrValidation
	}
	return s.Repo.UpdateBranding(ctx, meta(p, ip), tenantID, b)
}

// ---------------------------------------------------------------------
// Usuarios e invitaciones
// ---------------------------------------------------------------------

type UserService struct {
	Users       ports.UserRepository
	Invitations ports.InvitationRepository
	Cipher      ports.Cipher
	Hasher      ports.PasswordHasher
	Mailer      ports.Mailer
	AppURL      string
	InviteTTL   time.Duration
	// ExposeLink devuelve el enlace en la respuesta (solo local/staging mientras no haya proveedor de correo).
	ExposeLink bool
}

type InviteResult struct {
	Invitation *domain.Invitation `json:"invitation"`
	Link       string             `json:"link,omitempty"`
}

func (s *UserService) Invite(ctx context.Context, p *domain.Principal, ip string, tenantID *uint64, email, role, tenantName, locale string) (*InviteResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !ValidEmail(email) {
		return nil, domain.NewError("INVALID_EMAIL")
	}
	enc, err := s.Cipher.Encrypt(email)
	if err != nil {
		return nil, err
	}
	token := tokens.Random(32)
	inv, err := s.Invitations.Create(ctx, meta(p, ip), tenantID, uuid.NewString(), enc, s.Cipher.BlindIndex(email), role,
		tokens.Hash(token), time.Now().Add(s.InviteTTL))
	if err != nil {
		return nil, err
	}
	inv.Email = email
	link := strings.TrimRight(s.AppURL, "/") + "/invitacion/" + token
	if err := s.Mailer.SendInvitation(ctx, email, tenantName, link, locale); err != nil {
		return nil, err
	}
	res := &InviteResult{Invitation: inv}
	if s.ExposeLink {
		res.Link = link
	}
	return res, nil
}

func (s *UserService) InvitationInfo(ctx context.Context, token string) (*domain.Invitation, error) {
	rec, err := s.Invitations.GetByToken(ctx, tokens.Hash(token))
	if err != nil {
		return nil, err
	}
	switch rec.Status {
	case "PENDING":
	case "EXPIRED":
		return nil, domain.NewError("INVITATION_EXPIRED")
	default:
		return nil, domain.NewError("INVITATION_INVALID")
	}
	email, err := s.Cipher.Decrypt(rec.EmailEnc)
	if err != nil {
		return nil, err
	}
	inv := rec.Invitation
	inv.Email = maskEmail(email)
	return &inv, nil
}

func (s *UserService) AcceptInvitation(ctx context.Context, token, fullName, password, locale, ip string) (uint64, error) {
	if err := ValidatePassword(password); err != nil {
		return 0, err
	}
	fullName = strings.TrimSpace(fullName)
	if fullName == "" || len(fullName) > 150 {
		return 0, domain.ErrValidation
	}
	if locale != "es" && locale != "en" {
		locale = "es"
	}
	ne, err := s.Cipher.Encrypt(fullName)
	if err != nil {
		return 0, err
	}
	h, err := s.Hasher.Hash(password)
	if err != nil {
		return 0, err
	}
	return s.Invitations.Accept(ctx, tokens.Hash(token), uuid.NewString(), ne, h, locale, ip)
}

func (s *UserService) List(ctx context.Context, p *domain.Principal, tenantID *uint64, status string, page, size int) (*domain.Page[domain.User], error) {
	total, recs, err := s.Users.List(ctx, meta(p, ""), tenantID, status, page, size)
	if err != nil {
		return nil, err
	}
	out := &domain.Page[domain.User]{Total: total, Items: make([]domain.User, 0, len(recs))}
	for _, r := range recs {
		u := r.User
		if u.Email, err = s.Cipher.Decrypt(r.EmailEnc); err != nil {
			return nil, err
		}
		if u.FullName, err = s.Cipher.Decrypt(r.NameEnc); err != nil {
			return nil, err
		}
		out.Items = append(out.Items, u)
	}
	return out, nil
}

func (s *UserService) SetStatus(ctx context.Context, p *domain.Principal, ip string, tenantID *uint64, userUUID, status string) error {
	return s.Users.SetStatus(ctx, meta(p, ip), tenantID, userUUID, status)
}

func (s *UserService) ListInvitations(ctx context.Context, p *domain.Principal, tenantID *uint64) ([]domain.Invitation, error) {
	recs, err := s.Invitations.List(ctx, meta(p, ""), tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Invitation, 0, len(recs))
	for _, r := range recs {
		inv := r.Invitation
		if inv.Email, err = s.Cipher.Decrypt(r.EmailEnc); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, nil
}

func (s *UserService) RevokeInvitation(ctx context.Context, p *domain.Principal, ip string, tenantID *uint64, uuid string) error {
	return s.Invitations.Revoke(ctx, meta(p, ip), tenantID, uuid)
}

func maskEmail(e string) string {
	at := strings.LastIndex(e, "@")
	if at <= 1 {
		return e
	}
	return e[:1] + strings.Repeat("*", at-1) + e[at:]
}

// ---------------------------------------------------------------------
// API keys
// ---------------------------------------------------------------------

var AllowedScopes = []string{"talent.read", "talent.write", "masters.read"}

type ApiKeyService struct {
	Repo   ports.ApiKeyRepository
	Pepper []byte
}

// Formato de la key: bdt_<prefijo 12>_<secreto>. Solo se guarda HMAC(pepper, key).
func (s *ApiKeyService) Create(ctx context.Context, p *domain.Principal, ip string, tenantID uint64, name string, scopes []string, exp *time.Time) (*domain.ApiKey, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 || len(scopes) == 0 {
		return nil, domain.ErrValidation
	}
	for _, sc := range scopes {
		if !contains(AllowedScopes, sc) {
			return nil, domain.NewError("APIKEY_SCOPE")
		}
	}
	prefix := strings.ToLower(tokens.Random(9))[:12]
	prefix = strings.NewReplacer("-", "x", "_", "y").Replace(prefix)
	secret := "bdt_" + prefix + "_" + tokens.Random(32)
	k, err := s.Repo.Create(ctx, meta(p, ip), tenantID, uuid.NewString(), name, prefix, tokens.Keyed(s.Pepper, secret), scopes, exp)
	if err != nil {
		return nil, err
	}
	k.Secret = secret
	return k, nil
}

func (s *ApiKeyService) List(ctx context.Context, p *domain.Principal, tenantID *uint64) ([]domain.ApiKey, error) {
	return s.Repo.List(ctx, meta(p, ""), tenantID)
}

func (s *ApiKeyService) SetStatus(ctx context.Context, p *domain.Principal, ip, uuid, status string) (*domain.ApiKey, error) {
	return s.Repo.SetStatus(ctx, meta(p, ip), uuid, status)
}

// Authenticate valida una key presentada por un sistema externo.
func (s *ApiKeyService) Authenticate(ctx context.Context, presented, ip string) (*domain.ApiClient, error) {
	parts := strings.SplitN(presented, "_", 3)
	if len(parts) != 3 || parts[0] != "bdt" || len(parts[1]) != 12 {
		return nil, domain.NewError("APIKEY_INVALID")
	}
	rec, err := s.Repo.Lookup(ctx, parts[1])
	if err != nil {
		return nil, domain.NewError("APIKEY_INVALID")
	}
	if !tokens.Equal(rec.Hash, tokens.Keyed(s.Pepper, presented)) {
		return nil, domain.NewError("APIKEY_INVALID")
	}
	if rec.Status != "ENABLED" || rec.TenantStatus != "ACTIVE" || (rec.ExpiresAt != nil && rec.ExpiresAt.Before(time.Now())) {
		return nil, domain.NewError("APIKEY_NOT_ENABLED")
	}
	_ = s.Repo.Touch(ctx, rec.ID, ip)
	return &domain.ApiClient{KeyID: rec.ID, KeyUUID: rec.UUID, TenantID: rec.TenantID, TenantSlug: rec.TenantSlug, Scopes: rec.Scopes}, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------
// Maestro de maestros
// ---------------------------------------------------------------------

type MasterService struct{ Repo ports.MasterRepository }

func (s *MasterService) Tables(ctx context.Context) ([]domain.MasterTable, error) {
	return s.Repo.Tables(ctx)
}

func (s *MasterService) Items(ctx context.Context, tenantID *uint64, codes string, includeInactive bool) ([]domain.MasterItem, error) {
	return s.Repo.Items(ctx, tenantID, codes, includeInactive)
}

func (s *MasterService) Upsert(ctx context.Context, p *domain.Principal, ip string, in ports.MasterUpsert) (*domain.MasterItem, error) {
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if in.Code == "" || len(in.Code) > 60 || !json.Valid(in.Label) {
		return nil, domain.ErrValidation
	}
	if in.UUID == "" {
		in.UUID = uuid.NewString()
	} else if _, err := uuid.Parse(in.UUID); err != nil {
		return nil, domain.ErrValidation
	}
	return s.Repo.Upsert(ctx, meta(p, ip), in)
}

// Resolve convierte el uuid público de un tenant en su id interno (solo Super Admin).
func (s *TenantService) Resolve(ctx context.Context, p *domain.Principal, tenantUUID string) (uint64, string, error) {
	if _, err := uuid.Parse(tenantUUID); err != nil {
		return 0, "", domain.ErrNotFound
	}
	return s.Repo.Resolve(ctx, meta(p, ""), tenantUUID)
}
