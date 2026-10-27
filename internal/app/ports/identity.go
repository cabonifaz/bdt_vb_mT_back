package ports

import (
	"context"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// IdentityLookup consulta un registro público de identidad (p. ej. RENIEC para DNI peruanos).
type IdentityLookup interface {
	LookupDNI(ctx context.Context, dni string) (*domain.PersonIdentity, error)
	Provider() string
}

// IdentityAuditRepository valida el permiso y audita cada consulta (sin guardar el número consultado).
type IdentityAuditRepository interface {
	LogLookup(ctx context.Context, m Meta, tenantID uint64, provider, result string) error
}
