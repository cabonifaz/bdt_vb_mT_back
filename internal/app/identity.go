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
	Usage  ports.UsageRepository
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
	out, err := s.Lookup.LookupDNI(ctx, dni)
	// se cuenta cada consulta que llega al proveedor (un DNI no encontrado también consume); sin token no hay llamada
	if code := domain.AsError(err).Code; err == nil || code != "IDENTITY_NOT_CONFIGURED" {
		recordUsage(ctx, s.Usage, tid, domain.UsageDNILookup, s.Lookup.Provider(), err == nil || code == "IDENTITY_NOT_FOUND", domain.AIUsage{})
	}
	return out, err
}
