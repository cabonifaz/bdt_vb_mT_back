package ports

import (
	"context"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// UsageRepository acumula el consumo de servicios externos por tenant y lo reporta al Super Admin.
type UsageRepository interface {
	Record(ctx context.Context, tenantID uint64, kind, provider string, ok bool, promptTokens, completionTokens int64) error
	Report(ctx context.Context, m Meta, from, to time.Time) ([]domain.UsageRow, error)
}
