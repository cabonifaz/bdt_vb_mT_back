package mysql

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// SupportRepo: solicitudes al Super Admin, novedades y guías de primer uso.
type SupportRepo struct{ *Store }

func (r SupportRepo) Create(ctx context.Context, m ports.Meta, tenantID uint64, in ports.SupportNew) (int, error) {
	rs, err := r.call(ctx, "sp_support_create", m.ActorID, tenantID, in.ID, in.Kind, in.Impact, in.Title, in.Body,
		string(in.Origin), optJSON(in.Refs), in.MessageID, optJSON(in.Files), nullStr(m.IP))
	if err != nil {
		return 0, err
	}
	defer rs.Close()
	var id string
	var number int
	for rs.Next() {
		found := false
		if err := rs.Each(func(scan func(...any) error) error { found = true; return scan(&id, &number) }); err != nil {
			return 0, err
		}
		if found {
			break
		}
	}
	return number, nil
}

func (r SupportRepo) Reply(ctx context.Context, m ports.Meta, id, messageID, body, status string, files json.RawMessage) (*ports.SupportReplied, error) {
	rs, err := r.call(ctx, "sp_support_reply", m.ActorID, id, messageID, body, nullStr(status), optJSON(files), nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out ports.SupportReplied
	found := false
	for rs.Next() && !found {
		err := rs.Each(func(scan func(...any) error) error {
			found = true
			return scan(&out.ID, &out.Number, &out.Title, &out.Status, &out.Staff, &out.TenantID, &out.TenantSlug, &out.ReporterID)
		})
		if err != nil {
			return nil, err
		}
	}
	if !found {
		return nil, domain.ErrNotFound
	}
	return &out, nil
}

func (r SupportRepo) List(ctx context.Context, m ports.Meta, status, search string, limit int) ([]json.RawMessage, error) {
	return RequirementRepo(r).rawDocs(ctx, "sp_support_list", m.ActorID, nullStr(status), nullStr(search), limit)
}

func (r SupportRepo) Get(ctx context.Context, m ports.Meta, id string) (json.RawMessage, error) {
	docs, err := RequirementRepo(r).rawDocs(ctx, "sp_support_get", m.ActorID, id)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, domain.ErrNotFound
	}
	return docs[0], nil
}

func (r SupportRepo) File(ctx context.Context, m ports.Meta, fileID string) (*domain.FileObject, error) {
	rs, err := r.call(ctx, "sp_support_file_get", m.ActorID, fileID)
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

func (r SupportRepo) CreateFile(ctx context.Context, m ports.Meta, tenantID *uint64, f domain.FileObject) error {
	return r.exec(ctx, "sp_file_create", m.ActorID, nullU64(tenantID), f.UUID, "SUPPORT", nil, "ATTACHMENT",
		f.StorageKey, f.OriginalName, f.MimeType, f.Size, f.SHA256, f.Encrypted)
}

func (r SupportRepo) PublishRelease(ctx context.Context, m ports.Meta, id string, title, body json.RawMessage, module, tour string) error {
	return r.exec(ctx, "sp_release_publish", m.ActorID, id, string(title), string(body), nullStr(module), nullStr(tour), nullStr(m.IP))
}

func (r SupportRepo) Releases(ctx context.Context, m ports.Meta, limit int) ([]json.RawMessage, error) {
	return RequirementRepo(r).rawDocs(ctx, "sp_release_list", m.ActorID, limit)
}

func (r SupportRepo) Tours(ctx context.Context, m ports.Meta) (map[string]int, error) {
	rs, err := r.call(ctx, "sp_tours_get", m.ActorID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := map[string]int{}
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var k string
			var v int
			if err := scan(&k, &v); err != nil {
				return err
			}
			out[k] = v
			return nil
		})
	}
	return out, err
}

func (r SupportRepo) TourSeen(ctx context.Context, m ports.Meta, key string, version int) error {
	return r.exec(ctx, "sp_tour_seen", m.ActorID, key, version)
}

// NotifyUsers: notificación sin agrupar a una audiencia (soporte y novedades); tenantID 0 = de plataforma.
func (r NotificationRepo) NotifyUsers(ctx context.Context, m ports.Meta, tenantID uint64, in ports.NotifyInput, audience string, userID uint64) (*ports.NotifyResult, error) {
	var uid any
	if userID != 0 {
		uid = userID
	}
	rs, err := r.call(ctx, "sp_notify_users", nullU64ptr(m.ActorID), tenantID, in.ID, in.Type, in.EntityType, in.EntityID,
		string(in.Payload), audience, uid)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := &ports.NotifyResult{}
	if rs.Next() {
		if err := rs.One(&out.ID, &out.New, &out.ActorNameEnc); err != nil {
			return nil, err
		}
	}
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var t ports.PushTarget
			if err := scan(&t.Hash, &t.Endpoint, &t.P256dh, &t.Auth, &t.Locale); err != nil {
				return err
			}
			out.Targets = append(out.Targets, t)
			return nil
		})
	}
	return out, err
}

func nullU64ptr(v uint64) any {
	if v == 0 {
		return nil
	}
	return v
}
