// Package ports define los puertos de la arquitectura hexagonal.
// Los casos de uso (app) dependen solo de estas interfaces; los adaptadores las implementan.
package ports

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// ---- Seguridad ----

type Cipher interface {
	Encrypt(plain string) ([]byte, error)
	Decrypt(ct []byte) (string, error)
	BlindIndex(value string) string // HMAC determinista para búsquedas exactas
	EncryptBytes(plain []byte) ([]byte, error)
	DecryptBytes(ct []byte) ([]byte, error)
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encoded string) (bool, error)
}

type AccessClaims struct {
	UserUUID   string
	UserID     uint64
	TenantID   *uint64
	TenantSlug string
	TenantUUID string
	Roles      []string
	Perms      []string
	Locale     string
	ExpiresAt  time.Time
}

type TokenIssuer interface {
	IssueAccess(p *domain.Principal) (token string, exp time.Time, err error)
	ParseAccess(token string) (*AccessClaims, error)
}

// ---- Persistencia (procedimientos almacenados) ----

type Meta struct {
	ActorID uint64
	IP      string
}

type AuthRepository interface {
	FindUser(ctx context.Context, tenantSlug, emailBidx string) (*domain.AuthUser, error)
	RegisterAttempt(ctx context.Context, userID uint64, success bool, maxAttempts, lockMinutes int, ip string) error
	SecurityProfile(ctx context.Context, userID uint64) (*domain.AuthUser, []string, []string, error)
	CreateSession(ctx context.Context, uuid string, userID uint64, tokenHash, family string, exp time.Time, ip, ua string) error
	RotateSession(ctx context.Context, oldHash, newUUID, newHash string, exp time.Time, ip, ua string) (uint64, error)
	RevokeSession(ctx context.Context, tokenHash string) error
	BootstrapSuperAdmin(ctx context.Context, uuid string, emailEnc []byte, emailBidx string, nameEnc []byte, pwHash string) error
}

type TenantRepository interface {
	Create(ctx context.Context, m Meta, t domain.Tenant) (*domain.Tenant, error)
	List(ctx context.Context, m Meta, search, status string, page, size int) (*domain.Page[domain.Tenant], error)
	SetStatus(ctx context.Context, m Meta, tenantUUID, status string) (*domain.Tenant, error)
	PublicBySlug(ctx context.Context, slug string) (*domain.Branding, error)
	UpdateBranding(ctx context.Context, m Meta, tenantID uint64, b domain.Branding) (*domain.Branding, error)
	SetBrandingFile(ctx context.Context, m Meta, tenantID uint64, kind, fileUUID string) (*domain.Branding, error)
	Resolve(ctx context.Context, m Meta, tenantUUID string) (id uint64, name string, err error)
}

type UserRecord struct {
	domain.User
	EmailEnc, NameEnc []byte
}

type UserRepository interface {
	List(ctx context.Context, m Meta, tenantID *uint64, status string, page, size int) (int, []UserRecord, error)
	SetStatus(ctx context.Context, m Meta, tenantID *uint64, userUUID, status string) error
}

type InvitationRecord struct {
	domain.Invitation
	TenantID *uint64
	EmailEnc []byte
}

type InvitationRepository interface {
	Create(ctx context.Context, m Meta, tenantID *uint64, uuid string, emailEnc []byte, emailBidx, role, tokenHash string, exp time.Time) (*domain.Invitation, error)
	GetByToken(ctx context.Context, tokenHash string) (*InvitationRecord, error)
	Accept(ctx context.Context, tokenHash, userUUID string, nameEnc []byte, pwHash, locale, ip string) (uint64, error)
	List(ctx context.Context, m Meta, tenantID *uint64) ([]InvitationRecord, error)
	Revoke(ctx context.Context, m Meta, tenantID *uint64, uuid string) error
}

type ApiKeyRepository interface {
	Create(ctx context.Context, m Meta, tenantID uint64, uuid, name, prefix, hash string, scopes []string, exp *time.Time) (*domain.ApiKey, error)
	List(ctx context.Context, m Meta, tenantID *uint64) ([]domain.ApiKey, error)
	SetStatus(ctx context.Context, m Meta, uuid, status string) (*domain.ApiKey, error)
	Lookup(ctx context.Context, prefix string) (*domain.ApiKeyRecord, error)
	Touch(ctx context.Context, id uint64, ip string) error
}

type MasterUpsert struct {
	TenantID         *uint64
	TableCode        string
	UUID             string
	Code             string
	Label            json.RawMessage
	ParentUUID       *string
	Num1, Num2, Num3 *float64
	Str1, Str2, Str3 *string
	SortOrder        int
	IsActive         bool
}

type MasterRepository interface {
	Tables(ctx context.Context) ([]domain.MasterTable, error)
	Items(ctx context.Context, tenantID *uint64, codes string, includeInactive bool) ([]domain.MasterItem, error)
	Upsert(ctx context.Context, m Meta, in MasterUpsert) (*domain.MasterItem, error)
}

type FileRepository interface {
	Create(ctx context.Context, m Meta, f domain.FileObject, ownerID *uint64) error
	Get(ctx context.Context, tenantID *uint64, uuid string, publicOnly bool) (*domain.FileObject, error)
}

type AuditRepository interface {
	List(ctx context.Context, m Meta, tenantID *uint64, page, size int) ([]domain.AuditEntry, error)
}

type TranslationRepository interface {
	List(ctx context.Context, tenantID uint64, locale string) (map[string]string, error)
	Upsert(ctx context.Context, m Meta, tenantID uint64, locale, key, value string) error
}

// ---- Infraestructura externa ----

// FileStorage guarda binarios en el volumen persistente (nunca en el contenedor efímero).
type FileStorage interface {
	Put(ctx context.Context, key string, r io.Reader, maxBytes int64) (size int64, sha256hex string, err error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

type Mailer interface {
	SendInvitation(ctx context.Context, to, tenantName, link, locale string) error
}
