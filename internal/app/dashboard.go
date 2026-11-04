package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// DashboardService: indicadores de selección para el jefe de selección y el administrador (ingresos, tiempos,
// vencidos, analistas, embudo, clientes y perfiles). El SP arma el documento; aquí se valida el período y se
// descifran los nombres de los analistas.
type DashboardService struct {
	Repo   ports.DashboardRepository
	Cipher ports.Cipher
}

// DashboardFilter: From/To en AAAA-MM-DD (hasta 2 años), Grain WEEK o MONTH, ClientID opcional, Today la fecha
// local del navegador.
type DashboardFilter struct {
	From, To, Grain, ClientID, Today string
}

func (s *DashboardService) Selection(ctx context.Context, p *domain.Principal, f DashboardFilter) (json.RawMessage, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	from, err1 := time.Parse("2006-01-02", f.From)
	to, err2 := time.Parse("2006-01-02", f.To)
	if err1 != nil || err2 != nil || to.Before(from) || to.Sub(from) > 731*24*time.Hour || (f.Grain != "WEEK" && f.Grain != "MONTH") {
		return nil, domain.ErrValidation
	}
	q := ports.DashboardQuery{From: f.From, To: f.To, Grain: f.Grain, Today: boardToday(f.Today, time.Now())}
	if f.ClientID != "" {
		if !validUUID(f.ClientID) {
			return nil, domain.ErrValidation
		}
		q.ClientID = &f.ClientID
	}
	doc, err := s.Repo.Selection(ctx, meta(p, ""), tid, q)
	if err != nil {
		return nil, err
	}
	return decryptNames(s.Cipher, doc), nil
}
