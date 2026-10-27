package app

import (
	"context"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type HomeService struct {
	Repo     ports.HomeRepository
	Timezone *time.Location
	Now      func() time.Time
}

// Summary: "hoy" es la fecha local del tenant, no la del servidor.
func (s *HomeService) Summary(ctx context.Context, p *domain.Principal) (*domain.HomeSummary, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	loc := s.Timezone
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	local := now().In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	out, err := s.Repo.Summary(ctx, meta(p, ""), tid, today, now().UTC())
	if err != nil {
		return nil, err
	}
	out.Today = today.Format("2006-01-02")
	return out, nil
}
