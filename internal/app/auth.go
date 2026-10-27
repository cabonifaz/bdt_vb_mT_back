// Package app contiene los casos de uso. Solo depende de domain y ports.
package app

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/tokens"
)

type AuthConfig struct {
	RefreshTTL  time.Duration
	MaxAttempts int
	LockMinutes int
}

type AuthService struct {
	Repo   ports.AuthRepository
	Cipher ports.Cipher
	Hasher ports.PasswordHasher
	Tokens ports.TokenIssuer
	Cfg    AuthConfig
}

type Session struct {
	AccessToken  string            `json:"accessToken"`
	ExpiresAt    time.Time         `json:"expiresAt"`
	RefreshToken string            `json:"-"`
	RefreshExp   time.Time         `json:"-"`
	User         *domain.Principal `json:"-"`
}

type Client struct{ IP, UserAgent string }

// Login: tenantSlug vacío = acceso de plataforma (Super Admin).
func (s *AuthService) Login(ctx context.Context, tenantSlug, email, password string, c Client) (*Session, error) {
	email = strings.TrimSpace(email)
	if email == "" || password == "" || len(password) > 256 {
		return nil, domain.ErrInvalidCredentials
	}
	u, err := s.Repo.FindUser(ctx, strings.ToLower(strings.TrimSpace(tenantSlug)), s.Cipher.BlindIndex(email))
	if err != nil {
		return nil, err
	}
	if u == nil || u.PasswordHash == "" {
		_, _ = s.Hasher.Hash(password) // iguala tiempos (evita enumerar correos)
		return nil, domain.ErrInvalidCredentials
	}
	if u.LockedUntil != nil && u.LockedUntil.After(time.Now()) {
		return nil, domain.ErrAccountLocked
	}
	ok, err := s.Hasher.Verify(password, u.PasswordHash)
	if err != nil {
		return nil, err
	}
	if err := s.Repo.RegisterAttempt(ctx, u.ID, ok, s.Cfg.MaxAttempts, s.Cfg.LockMinutes, c.IP); err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrInvalidCredentials
	}
	if u.Status != "ACTIVE" {
		return nil, domain.ErrAccountDisabled
	}
	if u.TenantStatus != "ACTIVE" {
		return nil, domain.ErrTenantInactive
	}
	return s.newSession(ctx, u.ID, uuid.NewString(), c)
}

func (s *AuthService) newSession(ctx context.Context, userID uint64, family string, c Client) (*Session, error) {
	p, err := s.Principal(ctx, userID)
	if err != nil {
		return nil, err
	}
	refresh := tokens.Random(32)
	exp := time.Now().Add(s.Cfg.RefreshTTL)
	if err := s.Repo.CreateSession(ctx, uuid.NewString(), userID, tokens.Hash(refresh), family, exp, c.IP, c.UserAgent); err != nil {
		return nil, err
	}
	return s.issue(p, refresh, exp)
}

func (s *AuthService) issue(p *domain.Principal, refresh string, refreshExp time.Time) (*Session, error) {
	at, atExp, err := s.Tokens.IssueAccess(p)
	if err != nil {
		return nil, err
	}
	return &Session{AccessToken: at, ExpiresAt: atExp, RefreshToken: refresh, RefreshExp: refreshExp, User: p}, nil
}

// Refresh rota el refresh token (uso único). Reutilizar uno viejo revoca toda la familia.
func (s *AuthService) Refresh(ctx context.Context, refresh string, c Client) (*Session, error) {
	if refresh == "" {
		return nil, domain.ErrUnauthorized
	}
	next := tokens.Random(32)
	exp := time.Now().Add(s.Cfg.RefreshTTL)
	uid, err := s.Repo.RotateSession(ctx, tokens.Hash(refresh), uuid.NewString(), tokens.Hash(next), exp, c.IP, c.UserAgent)
	if err != nil {
		return nil, err
	}
	p, err := s.Principal(ctx, uid)
	if err != nil {
		return nil, err
	}
	return s.issue(p, next, exp)
}

func (s *AuthService) Logout(ctx context.Context, refresh string) error {
	if refresh == "" {
		return nil
	}
	return s.Repo.RevokeSession(ctx, tokens.Hash(refresh))
}

// Principal carga roles y permisos vigentes desde la BD (no confía en datos viejos).
func (s *AuthService) Principal(ctx context.Context, userID uint64) (*domain.Principal, error) {
	u, roles, perms, err := s.Repo.SecurityProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.Status != "ACTIVE" {
		return nil, domain.ErrAccountDisabled
	}
	if u.TenantStatus != "ACTIVE" {
		return nil, domain.ErrTenantInactive
	}
	email, err := s.Cipher.Decrypt(u.EmailEnc)
	if err != nil {
		return nil, err
	}
	name, err := s.Cipher.Decrypt(u.FullNameEnc)
	if err != nil {
		return nil, err
	}
	return &domain.Principal{
		UserID: u.ID, UserUUID: u.UUID, TenantID: u.TenantID, TenantUUID: u.TenantUUID, TenantSlug: u.TenantSlug,
		FullName: name, Email: email, Locale: u.Locale, Roles: roles, Perms: perms,
	}, nil
}

// BootstrapSuperAdmin crea el primer Super Admin (solo funciona si no existe ninguno).
func (s *AuthService) BootstrapSuperAdmin(ctx context.Context, email, fullName, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	if !ValidEmail(email) || strings.TrimSpace(fullName) == "" {
		return domain.ErrValidation
	}
	ee, err := s.Cipher.Encrypt(strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return err
	}
	ne, err := s.Cipher.Encrypt(strings.TrimSpace(fullName))
	if err != nil {
		return err
	}
	h, err := s.Hasher.Hash(password)
	if err != nil {
		return err
	}
	return s.Repo.BootstrapSuperAdmin(ctx, uuid.NewString(), ee, s.Cipher.BlindIndex(email), ne, h)
}

// ValidatePassword: mínimo 12 caracteres y al menos 3 de 4 tipos (mayúscula, minúscula, dígito, símbolo).
func ValidatePassword(p string) error {
	if len(p) < 12 || len(p) > 128 {
		return domain.ErrWeakPassword
	}
	var up, lo, di, sy bool
	for _, r := range p {
		switch {
		case unicode.IsUpper(r):
			up = true
		case unicode.IsLower(r):
			lo = true
		case unicode.IsDigit(r):
			di = true
		default:
			sy = true
		}
	}
	n := 0
	for _, b := range []bool{up, lo, di, sy} {
		if b {
			n++
		}
	}
	if n < 3 {
		return domain.ErrWeakPassword
	}
	return nil
}

func ValidEmail(e string) bool {
	e = strings.TrimSpace(e)
	at := strings.LastIndex(e, "@")
	return len(e) <= 254 && at > 0 && at < len(e)-3 && strings.Contains(e[at:], ".") && !strings.ContainsAny(e, " <>\"'")
}
