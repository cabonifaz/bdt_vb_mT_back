package mysql

import (
	"context"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

type IdentityRepo struct{ *Store }

func (r IdentityRepo) LogLookup(ctx context.Context, m ports.Meta, tenantID uint64, provider, result string) error {
	return r.exec(ctx, "sp_identity_lookup_log", m.ActorID, tenantID, provider, result, nullStr(m.IP))
}
