package mysql

import (
	"context"
	"encoding/json"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

// IGDTRepo: integración con IGDT (token cifrado, carga inicial, copia de solo lectura de los catálogos y tarifario).
type IGDTRepo struct{ *Store }

func (r IGDTRepo) one(ctx context.Context, proc string, args ...any) (json.RawMessage, error) {
	docs, err := RequirementRepo(r).rawDocs(ctx, proc, args...)
	if err != nil || len(docs) == 0 {
		return nil, err
	}
	return docs[0], nil
}

func (r IGDTRepo) Status(ctx context.Context, m ports.Meta, tenantID uint64) (json.RawMessage, error) {
	return r.one(ctx, "sp_igdt_status", m.ActorID, tenantID)
}

func (r IGDTRepo) Save(ctx context.Context, m ports.Meta, tenantID uint64, keyEnc []byte, remoteTenant string) error {
	return r.exec(ctx, "sp_igdt_save", m.ActorID, tenantID, keyEnc, remoteTenant, nullStr(m.IP))
}

func (r IGDTRepo) Disable(ctx context.Context, m ports.Meta, tenantID uint64) error {
	return r.exec(ctx, "sp_igdt_disable", m.ActorID, tenantID, nullStr(m.IP))
}

func (r IGDTRepo) Active(ctx context.Context, tenantID uint64) (bool, error) {
	rs, err := r.call(ctx, "sp_igdt_active", tenantID)
	if err != nil {
		return false, err
	}
	defer rs.Close()
	var active bool
	if rs.Next() {
		if err := rs.One(&active); err != nil {
			return false, err
		}
	}
	return active, nil
}

func (r IGDTRepo) Targets(ctx context.Context, tenantID *uint64) ([]ports.IGDTTarget, error) {
	rs, err := r.call(ctx, "sp_igdt_targets", tenantID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []ports.IGDTTarget{}
	if !rs.Next() {
		return out, nil
	}
	err = rs.Each(func(scan func(...any) error) error {
		var t ports.IGDTTarget
		if err := scan(&t.TenantID, &t.Slug, &t.KeyEnc, &t.Cursor, &t.Imported, &t.HiresCursor); err != nil {
			return err
		}
		out = append(out, t)
		return nil
	})
	return out, err
}

func (r IGDTRepo) Export(ctx context.Context, tenantID uint64) (json.RawMessage, error) {
	return r.one(ctx, "sp_igdt_export", tenantID)
}

func (r IGDTRepo) ApplyMap(ctx context.Context, tenantID uint64, m json.RawMessage) error {
	return r.exec(ctx, "sp_igdt_apply_map", tenantID, optJSON(m))
}

func (r IGDTRepo) ApplyChanges(ctx context.Context, tenantID uint64, changes json.RawMessage, cursor uint64) (int, error) {
	rs, err := r.call(ctx, "sp_igdt_apply_changes", tenantID, optJSON(changes), cursor)
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
	return n, nil
}

func (r IGDTRepo) SyncError(ctx context.Context, tenantID uint64, msg string) error {
	return r.exec(ctx, "sp_igdt_sync_error", tenantID, msg)
}

func (r IGDTRepo) TariffLookup(ctx context.Context, m ports.Meta, tenantID uint64, profileID string, clientID *string, date string) (json.RawMessage, error) {
	return r.one(ctx, "sp_tariff_lookup", m.ActorID, tenantID, profileID, clientID, nullStr(date))
}
