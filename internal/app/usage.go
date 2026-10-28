package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// UsageService: reporte de consumo de DNI y tokens de IA por tenant (Super Admin).
type UsageService struct {
	Repo ports.UsageRepository
}

// Report devuelve el consumo entre dos fechas (YYYY-MM-DD, máx. 366 días; el SP valida el permiso).
func (s *UsageService) Report(ctx context.Context, p *domain.Principal, from, to string) ([]domain.UsageRow, error) {
	f, err1 := time.Parse("2006-01-02", from)
	t, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil || t.Before(f) || t.Sub(f) > 366*24*time.Hour {
		return nil, domain.ErrValidation
	}
	return s.Repo.Report(ctx, meta(p, ""), f, t)
}

// recordUsage acumula una llamada a un proveedor externo. Si falla el registro no se interrumpe la operación del usuario.
func recordUsage(ctx context.Context, repo ports.UsageRepository, tenantID uint64, kind, provider string, ok bool, u domain.AIUsage) {
	if repo == nil {
		return
	}
	if err := repo.Record(context.WithoutCancel(ctx), tenantID, kind, provider, ok, u.PromptTokens, u.CompletionTokens); err != nil {
		slog.Warn("no se pudo registrar el consumo", "kind", kind, "tenant", tenantID, "err", err)
	}
}
