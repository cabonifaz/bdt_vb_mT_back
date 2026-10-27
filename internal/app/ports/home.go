package ports

import (
	"context"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type HomeRepository interface {
	Summary(ctx context.Context, m Meta, tenantID uint64, today, now time.Time) (*domain.HomeSummary, error)
}
