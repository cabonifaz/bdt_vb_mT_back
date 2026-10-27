package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type AuthRepo struct{ *Store }

func scanAuthUser(scan func(...any) error, u *domain.AuthUser, withSecrets bool) error {
	var tenantID sql.NullInt64
	var tUUID, tSlug sql.NullString
	var pw sql.NullString
	var locked sql.NullTime
	var failed int
	var err error
	if withSecrets {
		err = scan(&u.ID, &u.UUID, &tenantID, &tUUID, &tSlug, &u.TenantStatus, &u.EmailEnc, &u.FullNameEnc,
			&pw, &u.Locale, &u.Status, &failed, &locked)
	} else {
		err = scan(&u.ID, &u.UUID, &tenantID, &tUUID, &tSlug, &u.TenantStatus, &u.EmailEnc, &u.FullNameEnc,
			&u.Locale, &u.Status)
	}
	if err != nil {
		return err
	}
	if tenantID.Valid {
		v := uint64(tenantID.Int64)
		u.TenantID = &v
	}
	u.TenantUUID, u.TenantSlug, u.PasswordHash = tUUID.String, tSlug.String, pw.String
	if locked.Valid {
		u.LockedUntil = &locked.Time
	}
	return nil
}

func (r AuthRepo) FindUser(ctx context.Context, tenantSlug, emailBidx string) (*domain.AuthUser, error) {
	rs, err := r.call(ctx, "sp_auth_find_user", nullStr(tenantSlug), emailBidx)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var u domain.AuthUser
	found := false
	rs.Next()
	err = rs.Each(func(scan func(...any) error) error {
		found = true
		return scanAuthUser(scan, &u, true)
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &u, nil
}

func (r AuthRepo) RegisterAttempt(ctx context.Context, userID uint64, success bool, maxAttempts, lockMinutes int, ip string) error {
	return r.exec(ctx, "sp_auth_register_attempt", userID, success, maxAttempts, lockMinutes, nullStr(ip))
}

func (r AuthRepo) SecurityProfile(ctx context.Context, userID uint64) (*domain.AuthUser, []string, []string, error) {
	rs, err := r.call(ctx, "sp_user_security_profile", userID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rs.Close()
	var u domain.AuthUser
	rs.Next()
	found := false
	if err := rs.Each(func(scan func(...any) error) error { found = true; return scanAuthUser(scan, &u, false) }); err != nil {
		return nil, nil, nil, err
	}
	if !found {
		return nil, nil, nil, domain.ErrUnauthorized
	}
	var roles, perms []string
	if rs.Next() {
		_ = rs.Each(func(scan func(...any) error) error {
			var s string
			err := scan(&s)
			roles = append(roles, s)
			return err
		})
	}
	if rs.Next() {
		_ = rs.Each(func(scan func(...any) error) error {
			var s string
			err := scan(&s)
			perms = append(perms, s)
			return err
		})
	}
	return &u, roles, perms, nil
}

func (r AuthRepo) CreateSession(ctx context.Context, uuid string, userID uint64, tokenHash, family string, exp time.Time, ip, ua string) error {
	return r.exec(ctx, "sp_session_create", uuid, userID, tokenHash, family, exp.UTC(), nullStr(ip), nullStr(ua))
}

func (r AuthRepo) RotateSession(ctx context.Context, oldHash, newUUID, newHash string, exp time.Time, ip, ua string) (uint64, error) {
	rs, err := r.call(ctx, "sp_session_rotate", oldHash, newUUID, newHash, exp.UTC(), nullStr(ip), nullStr(ua))
	if err != nil {
		return 0, err
	}
	defer rs.Close()
	var uid uint64
	// sp_session_rotate llama a sp_session_create (sin result set) y luego devuelve user_id
	for rs.Next() {
		if err := rs.Each(func(scan func(...any) error) error { return scan(&uid) }); err != nil {
			return 0, err
		}
		if uid != 0 {
			break
		}
	}
	if err := rs.Close(); err != nil {
		return 0, err
	}
	if uid == 0 {
		return 0, domain.NewError("SESSION_INVALID")
	}
	return uid, nil
}

func (r AuthRepo) RevokeSession(ctx context.Context, tokenHash string) error {
	return r.exec(ctx, "sp_session_revoke", tokenHash)
}

func (r AuthRepo) BootstrapSuperAdmin(ctx context.Context, uuid string, emailEnc []byte, emailBidx string, nameEnc []byte, pwHash string) error {
	return r.exec(ctx, "sp_bootstrap_superadmin", uuid, emailEnc, emailBidx, nameEnc, pwHash)
}
