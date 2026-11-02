package mysql

import (
	"context"
	"encoding/json"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type FeatureRepo struct{ *Store }

func (r FeatureRepo) TenantFeatures(ctx context.Context, tenantID uint64) ([]string, error) {
	rs, err := r.call(ctx, "sp_tenant_features", tenantID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []string
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var c string
			if err := scan(&c); err != nil {
				return err
			}
			out = append(out, c)
			return nil
		})
	}
	return out, err
}

func (r FeatureRepo) List(ctx context.Context, m ports.Meta) ([]json.RawMessage, error) {
	return RequirementRepo(r).rawDocs(ctx, "sp_features_list", m.ActorID)
}

func (r FeatureRepo) SetStatus(ctx context.Context, m ports.Meta, code, status string) error {
	return r.exec(ctx, "sp_feature_set_status", m.ActorID, code, status, nullStr(m.IP))
}

func (r FeatureRepo) Tenants(ctx context.Context, m ports.Meta, code string) ([]json.RawMessage, error) {
	return RequirementRepo(r).rawDocs(ctx, "sp_feature_tenants", m.ActorID, code)
}

func (r FeatureRepo) SetTenant(ctx context.Context, m ports.Meta, tenantID, code string, enabled bool) (*ports.FeatureChange, error) {
	rs, err := r.call(ctx, "sp_tenant_feature_set", m.ActorID, tenantID, code, enabled, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var c ports.FeatureChange
	var name, desc, roles []byte
	found := false
	for rs.Next() && !found {
		err := rs.Each(func(scan func(...any) error) error {
			found = true
			return scan(&c.NewlyEnabled, &c.TenantID, &c.TenantSlug, &name, &desc, &c.Module, &roles, &c.Status)
		})
		if err != nil {
			return nil, err
		}
	}
	if !found {
		return nil, domain.ErrNotFound
	}
	_ = json.Unmarshal(name, &c.Name)
	_ = json.Unmarshal(desc, &c.Description)
	_ = json.Unmarshal(roles, &c.Roles)
	return &c, nil
}

func (r FeatureRepo) Unnoticed(ctx context.Context) ([]ports.FeatureNew, error) {
	rs, err := r.call(ctx, "sp_features_unnoticed")
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []ports.FeatureNew
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var f ports.FeatureNew
			var name, desc []byte
			if err := scan(&f.Code, &name, &desc, &f.Module); err != nil {
				return err
			}
			_ = json.Unmarshal(name, &f.Name)
			_ = json.Unmarshal(desc, &f.Description)
			out = append(out, f)
			return nil
		})
	}
	return out, err
}

func (r FeatureRepo) Noticed(ctx context.Context, code string) (bool, error) {
	rs, err := r.call(ctx, "sp_feature_noticed", code)
	if err != nil {
		return false, err
	}
	defer rs.Close()
	var n int
	if rs.Next() {
		err = rs.One(&n)
	}
	return n > 0, err
}
