package mysql

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type UserRepo struct{ *Store }

func (r UserRepo) List(ctx context.Context, m ports.Meta, tenantID *uint64, status string, page, size int) (int, []ports.UserRecord, error) {
	rs, err := r.call(ctx, "sp_user_list", m.ActorID, nullU64(tenantID), nullStr(status), page, size)
	if err != nil {
		return 0, nil, err
	}
	defer rs.Close()
	var total int
	rs.Next()
	if err := rs.One(&total); err != nil {
		return 0, nil, err
	}
	out := []ports.UserRecord{}
	if rs.Next() {
		err = rs.Each(func(scan func(...any) error) error {
			var u ports.UserRecord
			var last sql.NullTime
			var roles sql.NullString
			if err := scan(&u.ID, &u.UUID, &u.EmailEnc, &u.NameEnc, &u.Locale, &u.Status, &last, &u.CreatedAt, &roles); err != nil {
				return err
			}
			if last.Valid {
				u.LastLoginAt = &last.Time
			}
			u.Roles = splitCSV(roles.String)
			out = append(out, u)
			return nil
		})
	}
	return total, out, err
}

func (r UserRepo) SetStatus(ctx context.Context, m ports.Meta, tenantID *uint64, userUUID, status string) error {
	return r.exec(ctx, "sp_user_set_status", m.ActorID, nullU64(tenantID), userUUID, status, nullStr(m.IP))
}

func splitCSV(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}

type InvitationRepo struct{ *Store }

func (r InvitationRepo) Create(ctx context.Context, m ports.Meta, tenantID *uint64, uuid string, emailEnc []byte, emailBidx, role, tokenHash string, exp time.Time) (*domain.Invitation, error) {
	rs, err := r.call(ctx, "sp_invitation_create", m.ActorID, nullU64(tenantID), uuid, emailEnc, emailBidx, role, tokenHash, exp.UTC(), nullStr(m.IP))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var inv domain.Invitation
	var found bool
	for rs.Next() && !found {
		if err := rs.Each(func(scan func(...any) error) error {
			found = true
			return scan(&inv.UUID, &inv.Status, &inv.ExpiresAt, &inv.RoleCode)
		}); err != nil {
			return nil, err
		}
	}
	if err := rs.Close(); err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r InvitationRepo) GetByToken(ctx context.Context, tokenHash string) (*ports.InvitationRecord, error) {
	rs, err := r.call(ctx, "sp_invitation_get_by_token", tokenHash)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var rec ports.InvitationRecord
	var tid sql.NullInt64
	var slug, name sql.NullString
	found := false
	for rs.Next() && !found {
		if err := rs.Each(func(scan func(...any) error) error {
			found = true
			return scan(&rec.UUID, &tid, &slug, &name, &rec.EmailEnc, &rec.RoleCode, &rec.Status, &rec.ExpiresAt)
		}); err != nil {
			return nil, err
		}
	}
	if !found {
		return nil, domain.NewError("INVITATION_INVALID")
	}
	if tid.Valid {
		v := uint64(tid.Int64)
		rec.TenantID = &v
	}
	rec.TenantSlug, rec.TenantName = slug.String, name.String
	return &rec, nil
}

func (r InvitationRepo) Accept(ctx context.Context, tokenHash, userUUID string, nameEnc []byte, pwHash, locale, ip string) (uint64, error) {
	rs, err := r.call(ctx, "sp_invitation_accept", tokenHash, userUUID, nameEnc, pwHash, nullStr(locale), nullStr(ip))
	if err != nil {
		return 0, err
	}
	defer rs.Close()
	var id uint64
	for rs.Next() && id == 0 {
		if err := rs.Each(func(scan func(...any) error) error { return scan(&id) }); err != nil {
			return 0, err
		}
	}
	return id, rs.Close()
}

func (r InvitationRepo) List(ctx context.Context, m ports.Meta, tenantID *uint64) ([]ports.InvitationRecord, error) {
	rs, err := r.call(ctx, "sp_invitation_list", m.ActorID, nullU64(tenantID))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := []ports.InvitationRecord{}
	rs.Next()
	err = rs.Each(func(scan func(...any) error) error {
		var rec ports.InvitationRecord
		var acc sql.NullTime
		if err := scan(&rec.UUID, &rec.EmailEnc, &rec.RoleCode, &rec.Status, &rec.ExpiresAt, &acc, &rec.CreatedAt); err != nil {
			return err
		}
		if acc.Valid {
			rec.AcceptedAt = &acc.Time
		}
		out = append(out, rec)
		return nil
	})
	return out, err
}

func (r InvitationRepo) Revoke(ctx context.Context, m ports.Meta, tenantID *uint64, uuid string) error {
	return r.exec(ctx, "sp_invitation_revoke", m.ActorID, nullU64(tenantID), uuid, nullStr(m.IP))
}
