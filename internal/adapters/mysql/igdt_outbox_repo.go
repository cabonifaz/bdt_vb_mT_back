package mysql

import (
	"context"
	"encoding/json"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

// Cola de envíos a IGDT (bandeja de selección) y snapshots de RQ y candidatos.

func (r IGDTRepo) OutboxRQ(ctx context.Context, tenantID uint64, rqID string) error {
	return r.exec(ctx, "sp_igdt_outbox_rq", tenantID, rqID)
}

func (r IGDTRepo) OutboxCandidate(ctx context.Context, tenantID uint64, candidateID string) error {
	return r.exec(ctx, "sp_igdt_outbox_candidate", tenantID, candidateID)
}

func (r IGDTRepo) OutboxReconcile(ctx context.Context, tenantID uint64, full bool) (int, error) {
	rs, err := r.call(ctx, "sp_igdt_outbox_reconcile", tenantID, full)
	if err != nil {
		return 0, err
	}
	defer rs.Close()
	var n int
	if rs.Next() {
		if err := rs.One(&n); err != nil {
			return 0, err
		}
	}
	return n, rs.Close()
}

func (r IGDTRepo) OutboxNext(ctx context.Context, tenantID uint64, limit int) ([]ports.IGDTOutboxItem, error) {
	rs, err := r.call(ctx, "sp_igdt_outbox_next", tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []ports.IGDTOutboxItem{}
	if !rs.Next() {
		return out, nil
	}
	err = rs.Each(func(scan func(...any) error) error {
		var it ports.IGDTOutboxItem
		if err := scan(&it.ID, &it.Kind, &it.EntityID); err != nil {
			return err
		}
		out = append(out, it)
		return nil
	})
	return out, err
}

func (r IGDTRepo) OutboxResults(ctx context.Context, results []ports.IGDTOutboxResult) error {
	if len(results) == 0 {
		return nil
	}
	b, err := json.Marshal(results)
	if err != nil {
		return err
	}
	return r.exec(ctx, "sp_igdt_outbox_results", string(b))
}

func (r IGDTRepo) OutboxPurge(ctx context.Context) error {
	return r.exec(ctx, "sp_igdt_outbox_purge")
}

func (r IGDTRepo) OutboxStatus(ctx context.Context, tenantID uint64) (json.RawMessage, error) {
	return r.one(ctx, "sp_igdt_outbox_status", tenantID)
}

func (r IGDTRepo) RQSnapshots(ctx context.Context, tenantID uint64, ids []string) ([]json.RawMessage, error) {
	b, _ := json.Marshal(ids)
	return RequirementRepo(r).rawDocs(ctx, "sp_igdt_rq_snapshots", tenantID, string(b))
}

func (r IGDTRepo) HireSnapshots(ctx context.Context, tenantID uint64, ids []string) ([]json.RawMessage, error) {
	b, _ := json.Marshal(ids)
	return RequirementRepo(r).rawDocs(ctx, "sp_igdt_hire_snapshots", tenantID, string(b))
}

func (r IGDTRepo) ApplyHires(ctx context.Context, tenantID uint64, events json.RawMessage, cursor uint64) ([]string, error) {
	rs, err := r.call(ctx, "sp_igdt_apply_hires", tenantID, optJSON(events), cursor)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []string{}
	if !rs.Next() {
		return out, nil
	}
	err = rs.Each(func(scan func(...any) error) error {
		var id, code string
		if err := scan(&id, &code); err != nil {
			return err
		}
		out = append(out, code)
		return nil
	})
	return out, err
}
