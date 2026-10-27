package app

import (
	"context"
	"regexp"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

var dniRe = regexp.MustCompile(`^\d{8}$`)

// IdentityService autocompleta los datos del postulante a partir de su DNI.
type IdentityService struct {
	Lookup ports.IdentityLookup
	Repo   ports.IdentityAuditRepository
}

func (s *IdentityService) LookupDNI(ctx context.Context, p *domain.Principal, ip, dni string) (*domain.PersonIdentity, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !dniRe.MatchString(dni) {
		return nil, domain.NewError("INVALID_DNI")
	}
	// el permiso se verifica en el SP antes de consultar al proveedor
	if err := s.Repo.LogLookup(ctx, meta(p, ip), tid, s.Lookup.Provider(), "REQUESTED"); err != nil {
		return nil, err
	}
	return s.Lookup.LookupDNI(ctx, dni)
}
