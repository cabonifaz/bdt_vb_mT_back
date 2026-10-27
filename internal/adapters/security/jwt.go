package security

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/tokens"
)

type JWTIssuer struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

func NewJWTIssuer(secret string, ttl time.Duration) (*JWTIssuer, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT_SECRET debe tener al menos 32 caracteres")
	}
	return &JWTIssuer{secret: []byte(secret), ttl: ttl, issuer: "bdt-mt"}, nil
}

type claims struct {
	UID    uint64   `json:"uid"`
	TID    *uint64  `json:"tid,omitempty"`
	Tenant string   `json:"ten,omitempty"`
	TUID   string   `json:"tuid,omitempty"`
	Roles  []string `json:"roles"`
	Perms  []string `json:"perms"`
	Locale string   `json:"loc"`
	jwt.RegisteredClaims
}

func (j *JWTIssuer) IssueAccess(p *domain.Principal) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(j.ttl)
	c := claims{
		UID: p.UserID, TID: p.TenantID, Tenant: p.TenantSlug, TUID: p.TenantUUID, Roles: p.Roles, Perms: p.Perms, Locale: p.Locale,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: p.UserUUID, Issuer: j.issuer, Audience: jwt.ClaimStrings{"bdt-app"},
			IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp),
			ID: tokens.Random(12),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(j.secret)
	return s, exp, err
}

func (j *JWTIssuer) ParseAccess(token string) (*ports.AccessClaims, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) { return j.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(j.issuer), jwt.WithAudience("bdt-app"),
		jwt.WithExpirationRequired(), jwt.WithLeeway(5*time.Second))
	if err != nil {
		return nil, domain.ErrUnauthorized
	}
	return &ports.AccessClaims{
		UserUUID: c.Subject, UserID: c.UID, TenantID: c.TID, TenantSlug: c.Tenant, TenantUUID: c.TUID,
		Roles: c.Roles, Perms: c.Perms, Locale: c.Locale, ExpiresAt: c.ExpiresAt.Time,
	}, nil
}
