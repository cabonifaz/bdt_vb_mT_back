package domain

import "slices"

const (
	RoleSuperAdmin  = "SUPER_ADMIN"
	RoleTenantAdmin = "TENANT_ADMIN"
)

// Principal es el usuario autenticado en el contexto de una petición.
type Principal struct {
	UserID     uint64
	UserUUID   string
	TenantID   *uint64 // nil = usuario de plataforma (Super Admin)
	TenantUUID string
	TenantSlug string
	FullName   string
	Email      string
	Locale     string
	Roles      []string
	Perms      []string
}

func (p *Principal) Has(perm string) bool { return slices.Contains(p.Perms, perm) }
func (p *Principal) IsPlatform() bool     { return p.TenantID == nil }

// ApiClient es el sistema externo autenticado con una API key de su tenant.
type ApiClient struct {
	KeyID      uint64
	KeyUUID    string
	TenantID   uint64
	TenantSlug string
	Scopes     []string
}

func (c *ApiClient) Can(scope string) bool { return slices.Contains(c.Scopes, scope) }
