package ports

import (
	"context"
	"encoding/json"
)

// IGDTIdentity: a qué tenant de IGDT pertenece el token y con qué alcances.
type IGDTIdentity struct {
	Tenant string   `json:"tenant"`
	KeyID  string   `json:"keyId"`
	Scopes []string `json:"scopes"`
}

// IGDTChanges: una página de cambios de los catálogos que gobierna IGDT.
type IGDTChanges struct {
	Changes json.RawMessage `json:"changes"`
	Next    uint64          `json:"next"`
	More    bool            `json:"more"`
}

// IGDTClient: API de integración de IGDT.
type IGDTClient interface {
	Enabled() bool
	WhoAmI(ctx context.Context, key string) (*IGDTIdentity, error)
	Changes(ctx context.Context, key string, since uint64, limit int) (*IGDTChanges, error)
	Import(ctx context.Context, key string, export json.RawMessage) (json.RawMessage, error)
}

// IGDTTarget: tenant con la integración activa (token cifrado) y su cursor de sincronización.
type IGDTTarget struct {
	TenantID uint64
	Slug     string
	KeyEnc   []byte
	Cursor   uint64
	Imported bool
}

type IGDTRepository interface {
	Status(ctx context.Context, m Meta, tenantID uint64) (json.RawMessage, error)
	Save(ctx context.Context, m Meta, tenantID uint64, keyEnc []byte, remoteTenant string) error
	Disable(ctx context.Context, m Meta, tenantID uint64) error
	Active(ctx context.Context, tenantID uint64) (bool, error)
	Targets(ctx context.Context, tenantID *uint64) ([]IGDTTarget, error)
	Export(ctx context.Context, tenantID uint64) (json.RawMessage, error)
	ApplyMap(ctx context.Context, tenantID uint64, m json.RawMessage) error
	ApplyChanges(ctx context.Context, tenantID uint64, changes json.RawMessage, cursor uint64) (int, error)
	SyncError(ctx context.Context, tenantID uint64, msg string) error
	TariffLookup(ctx context.Context, m Meta, tenantID uint64, profileID string, clientID *string, date string) (json.RawMessage, error)
}
