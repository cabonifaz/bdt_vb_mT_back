package mysql

import (
	"context"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type UsageRepo struct{ *Store }

func (r UsageRepo) Record(ctx context.Context, tenantID uint64, kind, provider string, ok bool, prompt, completion int64) error {
	return r.exec(ctx, "sp_usage_record", tenantID, kind, provider, ok, prompt, completion)
}

func (r UsageRepo) Report(ctx context.Context, m ports.Meta, from, to time.Time) ([]domain.UsageRow, error) {
	rs, err := r.call(ctx, "sp_usage_report", m.ActorID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []domain.UsageRow{}
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var u domain.UsageRow
			if err := scan(&u.TenantID, &u.Slug, &u.Name, &u.Month, &u.Kind, &u.Provider, &u.Calls, &u.Errors, &u.PromptTokens, &u.CompletionTokens); err != nil {
				return err
			}
			out = append(out, u)
			return nil
		})
	}
	return out, err
}
