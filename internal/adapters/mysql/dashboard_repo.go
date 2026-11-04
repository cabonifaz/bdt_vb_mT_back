package mysql

import (
	"context"
	"encoding/json"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

type DashboardRepo struct{ *Store }

func (r DashboardRepo) Selection(ctx context.Context, m ports.Meta, tenantID uint64, q ports.DashboardQuery) (json.RawMessage, error) {
	rs, err := r.call(ctx, "sp_dash_selection", m.ActorID, tenantID, q.From, q.To, q.Grain, q.ClientID, q.Today)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var doc []byte
	if rs.Next() {
		if err := rs.One(&doc); err != nil {
			return nil, err
		}
	}
	return doc, nil
}
