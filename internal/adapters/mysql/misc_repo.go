package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// ---------------- API keys ----------------

type ApiKeyRepo struct{ *Store }

func scanApiKey(scan func(...any) error) (domain.ApiKey, error) {
	var k domain.ApiKey
	var tenantID uint64
	var scopes []byte
	var enabled, expires, used sql.NullTime
	if err := scan(&k.UUID, &tenantID, &k.TenantSlug, &k.TenantName, &k.Name, &k.Prefix, &scopes, &k.Status,
		&enabled, &expires, &used, &k.CreatedAt); err != nil {
		return k, err
	}
	_ = json.Unmarshal(scopes, &k.Scopes)
	if enabled.Valid {
		k.EnabledAt = &enabled.Time
	}
	if expires.Valid {
		k.ExpiresAt = &expires.Time
	}
	if used.Valid {
		k.LastUsedAt = &used.Time
	}
	return k, nil
}

func (r ApiKeyRepo) collect(rs *resultSets) ([]domain.ApiKey, error) {
	defer rs.Close()
	out := []domain.ApiKey{}
	for rs.Next() {
		if err := rs.Each(func(scan func(...any) error) error {
			k, err := scanApiKey(scan)
			out = append(out, k)
			return err
		}); err != nil {
			return nil, err
		}
	}
	return out, rs.Close()
}

func (r ApiKeyRepo) Create(ctx context.Context, m ports.Meta, tenantID uint64, uuid, name, prefix, hash string, scopes []string, exp *time.Time) (*domain.ApiKey, error) {
	sc, _ := json.Marshal(scopes)
	var e any
	if exp != nil {
		e = exp.UTC()
	}
	rs, err := r.call(ctx, "sp_apikey_create", m.ActorID, tenantID, uuid, name, prefix, hash, string(sc), e, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	keys, err := r.collect(rs)
	if err != nil || len(keys) == 0 {
		return nil, firstErr(err, domain.ErrInternal)
	}
	return &keys[0], nil
}

func (r ApiKeyRepo) List(ctx context.Context, m ports.Meta, tenantID *uint64) ([]domain.ApiKey, error) {
	rs, err := r.call(ctx, "sp_apikey_list", m.ActorID, nullU64(tenantID), nil)
	if err != nil {
		return nil, err
	}
	return r.collect(rs)
}

func (r ApiKeyRepo) SetStatus(ctx context.Context, m ports.Meta, uuid, status string) (*domain.ApiKey, error) {
	rs, err := r.call(ctx, "sp_apikey_set_status", m.ActorID, uuid, status, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	keys, err := r.collect(rs)
	if err != nil || len(keys) == 0 {
		return nil, firstErr(err, domain.ErrNotFound)
	}
	return &keys[0], nil
}

func (r ApiKeyRepo) Lookup(ctx context.Context, prefix string) (*domain.ApiKeyRecord, error) {
	rs, err := r.call(ctx, "sp_apikey_lookup", prefix)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var k domain.ApiKeyRecord
	var scopes []byte
	var exp sql.NullTime
	rs.Next()
	if err := rs.One(&k.ID, &k.UUID, &k.TenantID, &k.TenantSlug, &k.Hash, &scopes, &k.Status, &exp, &k.TenantStatus); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(scopes, &k.Scopes)
	if exp.Valid {
		k.ExpiresAt = &exp.Time
	}
	return &k, nil
}

func (r ApiKeyRepo) Touch(ctx context.Context, id uint64, ip string) error {
	return r.exec(ctx, "sp_apikey_touch", id, nullStr(ip))
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// ---------------- Maestros ----------------

type MasterRepo struct{ *Store }

func (r MasterRepo) Tables(ctx context.Context) ([]domain.MasterTable, error) {
	rs, err := r.call(ctx, "sp_master_table_list")
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []domain.MasterTable{}
	rs.Next()
	err = rs.Each(func(scan func(...any) error) error {
		var t domain.MasterTable
		var id uint64
		var name []byte
		var desc, parent sql.NullString
		if err := scan(&id, &t.Code, &name, &desc, &t.Scope, &t.IsSystem, &parent); err != nil {
			return err
		}
		t.Name = json.RawMessage(name)
		if desc.Valid {
			t.Description = &desc.String
		}
		if parent.Valid {
			t.ParentCode = &parent.String
		}
		out = append(out, t)
		return nil
	})
	return out, err
}

func scanMasterItem(scan func(...any) error) (domain.MasterItem, error) {
	var it domain.MasterItem
	var label []byte
	var parent, s1, s2, s3 sql.NullString
	var n1, n2, n3 sql.NullFloat64
	var d1, d2, d3 sql.NullTime
	var legacy sql.NullInt64
	var version time.Time
	err := scan(&it.UUID, &it.TableCode, &it.Code, &label, &parent, &n1, &n2, &n3, &s1, &s2, &s3, &d1, &d2, &d3,
		&it.SortOrder, &it.IsActive, &it.IsTenantOwned, &legacy, &version)
	it.Label = json.RawMessage(label)
	it.ParentUUID = strPtr(parent)
	it.Str1, it.Str2, it.Str3 = strPtr(s1), strPtr(s2), strPtr(s3)
	it.Num1, it.Num2, it.Num3 = fPtr(n1), fPtr(n2), fPtr(n3)
	if legacy.Valid {
		v := int(legacy.Int64)
		it.LegacyID = &v
	}
	return it, err
}

func strPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}

func fPtr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	return &n.Float64
}

func (r MasterRepo) Items(ctx context.Context, tenantID *uint64, codes string, includeInactive bool) ([]domain.MasterItem, error) {
	rs, err := r.call(ctx, "sp_master_item_list", nullU64(tenantID), nullStr(codes), includeInactive)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []domain.MasterItem{}
	rs.Next()
	err = rs.Each(func(scan func(...any) error) error {
		it, err := scanMasterItem(scan)
		out = append(out, it)
		return err
	})
	return out, err
}

func (r MasterRepo) Upsert(ctx context.Context, m ports.Meta, in ports.MasterUpsert) (*domain.MasterItem, error) {
	ptr := func(p *string) any {
		if p == nil {
			return nil
		}
		return *p
	}
	fp := func(p *float64) any {
		if p == nil {
			return nil
		}
		return *p
	}
	rs, err := r.call(ctx, "sp_master_item_upsert", m.ActorID, nullU64(in.TenantID), in.TableCode, in.UUID, in.Code,
		string(in.Label), ptr(in.ParentUUID), fp(in.Num1), fp(in.Num2), fp(in.Num3), ptr(in.Str1), ptr(in.Str2), ptr(in.Str3),
		in.SortOrder, in.IsActive, nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var it domain.MasterItem
	var label []byte
	found := false
	for rs.Next() && !found {
		if err := rs.Each(func(scan func(...any) error) error {
			found = true
			return scan(&it.UUID, &it.TableCode, &it.Code, &label, &it.SortOrder, &it.IsActive)
		}); err != nil {
			return nil, err
		}
	}
	if err := rs.Close(); err != nil {
		return nil, err
	}
	it.Label = json.RawMessage(label)
	return &it, nil
}

// ---------------- Archivos ----------------

type FileRepo struct{ *Store }

func (r FileRepo) Create(ctx context.Context, m ports.Meta, f domain.FileObject, ownerID *uint64) error {
	return r.exec(ctx, "sp_file_create", m.ActorID, nullU64(f.TenantID), f.UUID, f.OwnerType, nullU64(ownerID), f.Category,
		f.StorageKey, f.OriginalName, f.MimeType, f.Size, f.SHA256, f.Encrypted)
}

func (r FileRepo) Get(ctx context.Context, tenantID *uint64, uuid string, publicOnly bool) (*domain.FileObject, error) {
	rs, err := r.call(ctx, "sp_file_get", nullU64(tenantID), uuid, publicOnly)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var f domain.FileObject
	var tid sql.NullInt64
	rs.Next()
	if err := rs.One(&f.ID, &f.UUID, &tid, &f.OwnerType, &f.Category, &f.StorageKey, &f.OriginalName, &f.MimeType,
		&f.Size, &f.SHA256, &f.Encrypted); err != nil {
		return nil, err
	}
	if tid.Valid {
		v := uint64(tid.Int64)
		f.TenantID = &v
	}
	return &f, nil
}

// ---------------- Auditoría y traducciones ----------------

type AuditRepo struct{ *Store }

func (r AuditRepo) List(ctx context.Context, m ports.Meta, tenantID *uint64, page, size int) ([]domain.AuditEntry, error) {
	rs, err := r.call(ctx, "sp_audit_list", m.ActorID, nullU64(tenantID), page, size)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []domain.AuditEntry{}
	rs.Next()
	err = rs.Each(func(scan func(...any) error) error {
		var a domain.AuditEntry
		var tid sql.NullInt64
		var user, eid, ip sql.NullString
		var detail []byte
		if err := scan(&a.ID, &tid, &user, &a.Action, &a.Entity, &eid, &detail, &ip, &a.CreatedAt); err != nil {
			return err
		}
		a.UserUUID, a.EntityID, a.IP = strPtr(user), strPtr(eid), strPtr(ip)
		if len(detail) > 0 {
			a.Detail = json.RawMessage(detail)
		}
		out = append(out, a)
		return nil
	})
	return out, err
}

type TranslationRepo struct{ *Store }

func (r TranslationRepo) List(ctx context.Context, tenantID uint64, locale string) (map[string]string, error) {
	rs, err := r.call(ctx, "sp_translation_list", tenantID, locale)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := map[string]string{}
	rs.Next()
	err = rs.Each(func(scan func(...any) error) error {
		var k, v string
		err := scan(&k, &v)
		out[k] = v
		return err
	})
	return out, err
}

func (r TranslationRepo) Upsert(ctx context.Context, m ports.Meta, tenantID uint64, locale, key, value string) error {
	return r.exec(ctx, "sp_translation_upsert", m.ActorID, tenantID, locale, key, value, nullStr(m.IP))
}
