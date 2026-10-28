package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type NotificationRepo struct{ *Store }

func (r NotificationRepo) Notify(ctx context.Context, m ports.Meta, tenantID uint64, in ports.NotifyInput) (*ports.NotifyResult, error) {
	var group any
	if in.GroupWith != "" {
		group = in.GroupWith
	}
	rs, err := r.call(ctx, "sp_notify_entity", m.ActorID, tenantID, in.ID, in.Type, group, in.EntityType, in.EntityID,
		string(in.Payload), in.RecipientPerm, in.GroupMinutes)
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
	if out.New && rs.Next() {
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

func (r NotificationRepo) List(ctx context.Context, m ports.Meta, tenantID uint64, unreadOnly bool, before *time.Time, limit int) (int, []ports.NotificationRecord, error) {
	var b any
	if before != nil {
		b = before.UTC().Format("2006-01-02 15:04:05.000")
	}
	rs, err := r.call(ctx, "sp_notification_list", m.ActorID, tenantID, unreadOnly, b, limit)
	if err != nil {
		return 0, nil, err
	}
	defer rs.Close()
	var unread int
	if rs.Next() {
		if err := rs.One(&unread); err != nil {
			return 0, nil, err
		}
	}
	items := []ports.NotificationRecord{}
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var x ports.NotificationRecord
			var payload []byte
			var read sql.NullTime
			var actor sql.NullString
			if err := scan(&x.ID, &x.Type, &x.EntityType, &x.EntityID, &payload, &x.Count, &x.CreatedAt, &x.At, &read, &actor, &x.ActorNameEnc); err != nil {
				return err
			}
			x.Payload = json.RawMessage(payload)
			if read.Valid {
				x.ReadAt = &read.Time
			}
			x.ActorID = actor.String
			items = append(items, x)
			return nil
		})
	}
	return unread, items, err
}

func (r NotificationRepo) MarkRead(ctx context.Context, m ports.Meta, tenantID uint64, id *string) error {
	return r.exec(ctx, "sp_notification_mark_read", m.ActorID, tenantID, id)
}

func (r NotificationRepo) Subscribe(ctx context.Context, m ports.Meta, tenantID uint64, hash, endpoint, p256dh, auth, ua string) (int, error) {
	rs, err := r.call(ctx, "sp_push_subscribe", m.ActorID, tenantID, hash, endpoint, p256dh, auth, nullStr(ua))
	if err != nil {
		return 0, err
	}
	defer rs.Close()
	var n int
	if rs.Next() {
		err = rs.One(&n)
	}
	return n, err
}

func (r NotificationRepo) Unsubscribe(ctx context.Context, m ports.Meta, tenantID uint64, hash string) error {
	return r.exec(ctx, "sp_push_unsubscribe", m.ActorID, tenantID, hash)
}

func (r NotificationRepo) PushResult(ctx context.Context, hash string, ok, gone bool) error {
	return r.exec(ctx, "sp_push_result", hash, ok, gone)
}

func (r NotificationRepo) Prefs(ctx context.Context, m ports.Meta, tenantID uint64) ([]domain.NotificationPref, int, error) {
	rs, err := r.call(ctx, "sp_notification_prefs_get", m.ActorID, tenantID)
	if err != nil {
		return nil, 0, err
	}
	defer rs.Close()
	prefs := []domain.NotificationPref{}
	if rs.Next() {
		if err := rs.Each(func(scan func(...any) error) error {
			var p domain.NotificationPref
			if err := scan(&p.Type, &p.InApp, &p.Push); err != nil {
				return err
			}
			prefs = append(prefs, p)
			return nil
		}); err != nil {
			return nil, 0, err
		}
	}
	var devices int
	if rs.Next() {
		err = rs.One(&devices)
	}
	return prefs, devices, err
}

func (r NotificationRepo) SetPref(ctx context.Context, m ports.Meta, tenantID uint64, p domain.NotificationPref) error {
	return r.exec(ctx, "sp_notification_pref_set", m.ActorID, tenantID, p.Type, p.InApp, p.Push)
}
