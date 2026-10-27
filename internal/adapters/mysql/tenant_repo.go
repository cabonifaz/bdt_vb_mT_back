package mysql

import (
	"context"
	"database/sql"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type TenantRepo struct{ *Store }

func scanTenant(scan func(...any) error) (domain.Tenant, error) {
	var t domain.Tenant
	var legal, tax sql.NullString
	err := scan(&t.ID, &t.UUID, &t.Slug, &t.Name, &legal, &tax, &t.Status, &t.DefaultLocale, &t.Timezone,
		&t.CreatedAt, &t.UserCount)
	if legal.Valid {
		t.LegalName = &legal.String
	}
	if tax.Valid {
		t.TaxID = &tax.String
	}
	return t, err
}

// sp_tenant_list devuelve las columnas en otro orden (created_at antes de user_count): igual que sp_tenant_get.
func (r TenantRepo) oneTenant(rs *resultSets) (*domain.Tenant, error) {
	defer rs.Close()
	var out *domain.Tenant
	for rs.Next() && out == nil {
		if err := rs.Each(func(scan func(...any) error) error {
			t, err := scanTenant(scan)
			out = &t
			return err
		}); err != nil {
			return nil, err
		}
	}
	if err := rs.Close(); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, domain.ErrNotFound
	}
	return out, nil
}

func (r TenantRepo) Create(ctx context.Context, m ports.Meta, t domain.Tenant) (*domain.Tenant, error) {
	var legal, tax any
	if t.LegalName != nil {
		legal = *t.LegalName
	}
	if t.TaxID != nil {
		tax = *t.TaxID
	}
	rs, err := r.call(ctx, "sp_tenant_create", m.ActorID, t.UUID, t.Slug, t.Name, legal, tax,
		nullStr(t.DefaultLocale), nullStr(t.Timezone), nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return r.oneTenant(rs)
}

func (r TenantRepo) List(ctx context.Context, m ports.Meta, search, status string, page, size int) (*domain.Page[domain.Tenant], error) {
	rs, err := r.call(ctx, "sp_tenant_list", m.ActorID, nullStr(search), nullStr(status), page, size)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := &domain.Page[domain.Tenant]{Items: []domain.Tenant{}}
	rs.Next()
	if err := rs.One(&out.Total); err != nil {
		return nil, err
	}
	if rs.Next() {
		if err := rs.Each(func(scan func(...any) error) error {
			t, err := scanTenant(scan)
			out.Items = append(out.Items, t)
			return err
		}); err != nil {
			return nil, err
		}
	}
	return out, rs.Close()
}

func (r TenantRepo) SetStatus(ctx context.Context, m ports.Meta, tenantUUID, status string) (*domain.Tenant, error) {
	rs, err := r.call(ctx, "sp_tenant_set_status", m.ActorID, tenantUUID, status, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return r.oneTenant(rs)
}

func scanBranding(scan func(...any) error) (domain.Branding, error) {
	var b domain.Branding
	var tenantUUID, status string
	var logoURL, logoFile, logoDark, favicon sql.NullString
	err := scan(&b.TenantID, &tenantUUID, &b.Slug, &b.Name, &status, &b.DefaultLocale, &b.DisplayName, &logoURL,
		&logoFile, &logoDark, &favicon, &b.ColorPrimary, &b.ColorPrimaryHov, &b.ColorSecondary, &b.ColorAccent,
		&b.ColorSidebar, &b.FontFamily, &b.Version)
	b.TenantUUID = tenantUUID
	b.LogoURL = logoURL.String
	if logoFile.Valid {
		b.LogoURL = "/api/v1/public/files/" + logoFile.String
	}
	if logoDark.Valid {
		b.LogoDarkURL = "/api/v1/public/files/" + logoDark.String
	}
	if favicon.Valid {
		b.FaviconURL = "/api/v1/public/files/" + favicon.String
	}
	return b, err
}

func (r TenantRepo) oneBranding(rs *resultSets) (*domain.Branding, error) {
	defer rs.Close()
	var out *domain.Branding
	for rs.Next() && out == nil {
		if err := rs.Each(func(scan func(...any) error) error {
			b, err := scanBranding(scan)
			out = &b
			return err
		}); err != nil {
			return nil, err
		}
	}
	if err := rs.Close(); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, domain.ErrNotFound
	}
	return out, nil
}

func (r TenantRepo) PublicBySlug(ctx context.Context, slug string) (*domain.Branding, error) {
	rs, err := r.call(ctx, "sp_tenant_public_get", slug)
	if err != nil {
		return nil, err
	}
	return r.oneBranding(rs)
}

func (r TenantRepo) UpdateBranding(ctx context.Context, m ports.Meta, tenantID uint64, b domain.Branding) (*domain.Branding, error) {
	rs, err := r.call(ctx, "sp_branding_update", m.ActorID, tenantID, b.DisplayName, b.ColorPrimary, b.ColorPrimaryHov,
		b.ColorSecondary, b.ColorAccent, b.ColorSidebar, b.FontFamily, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return r.oneBranding(rs)
}

func (r TenantRepo) SetBrandingFile(ctx context.Context, m ports.Meta, tenantID uint64, kind, fileUUID string) (*domain.Branding, error) {
	rs, err := r.call(ctx, "sp_branding_set_file", m.ActorID, tenantID, kind, fileUUID, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	return r.oneBranding(rs)
}

func (r TenantRepo) Resolve(ctx context.Context, m ports.Meta, tenantUUID string) (uint64, string, error) {
	rs, err := r.call(ctx, "sp_tenant_resolve", m.ActorID, tenantUUID)
	if err != nil {
		return 0, "", err
	}
	defer rs.Close()
	var id uint64
	var slug, name string
	rs.Next()
	if err := rs.One(&id, &slug, &name); err != nil {
		return 0, "", err
	}
	return id, name, nil
}
