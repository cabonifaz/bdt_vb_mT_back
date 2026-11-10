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
	// LinkBDT entrega a IGDT la llave BDT del tenant (PUT /integration/v1/bdt-link); UnlinkBDT la retira (DELETE).
	LinkBDT(ctx context.Context, key, bdtKey string) error
	UnlinkBDT(ctx context.Context, key string) error
}

// IGDTTarget: tenant con la integración activa (token cifrado) y su cursor de sincronización.
type IGDTTarget struct {
	TenantID uint64
	Slug     string
	KeyEnc   []byte
	Cursor   uint64
	Imported bool
	// HiresCursor: hasta qué evento de ingreso de IGDT se aplicó
	HiresCursor uint64
}

// IGDTRevokedHire: ingreso revocado en IGDT (el candidato ya no está ingresado; el RQ queda sin cubrir).
type IGDTRevokedHire struct {
	RQID, RQCode, RQTitle, CandidateID, TalentName, Reason string
}

// IGDTHiresApplied: resultado de aplicar una página de eventos de ingreso.
type IGDTHiresApplied struct {
	Closed  []string // códigos de los RQ cerrados como Atendido
	Revoked []IGDTRevokedHire
}

type IGDTRepository interface {
	IGDTOutboxRepository
	Status(ctx context.Context, m Meta, tenantID uint64) (json.RawMessage, error)
	Save(ctx context.Context, m Meta, tenantID uint64, keyEnc []byte, remoteTenant string) error
	Disable(ctx context.Context, m Meta, tenantID uint64) error
	Active(ctx context.Context, tenantID uint64) (bool, error)
	Targets(ctx context.Context, tenantID *uint64) ([]IGDTTarget, error)
	Export(ctx context.Context, tenantID uint64) (json.RawMessage, error)
	ApplyMap(ctx context.Context, tenantID uint64, m json.RawMessage) error
	ApplyChanges(ctx context.Context, tenantID uint64, changes json.RawMessage, cursor uint64) (int, error)
	SyncError(ctx context.Context, tenantID uint64, msg string) error
	// LinkKeyCreate crea la llave BDT automática (talent.read y talent.search, habilitada) si el tenant no tiene una vigente; created = false si no.
	LinkKeyCreate(ctx context.Context, tenantID uint64, uuid, name, prefix, hash string) (bool, error)
	// LinkKeyDrop descarta la llave automática que IGDT no recibió (revocada) y deja al tenant sin vincular.
	LinkKeyDrop(ctx context.Context, tenantID uint64, uuid, reason string) error
	TariffLookup(ctx context.Context, m Meta, tenantID uint64, profileID string, clientID *string, date string) (json.RawMessage, error)
}

// IGDTOutboxItem: envío pendiente a IGDT (Kind RQ: uuid del requerimiento; HIRE: uuid del candidato).
type IGDTOutboxItem struct {
	ID       uint64
	Kind     string
	EntityID string
}

// IGDTOutboxResult: resultado de un envío de la cola (OK 1/0 para el SP).
type IGDTOutboxResult struct {
	ID    uint64 `json:"id"`
	OK    int    `json:"ok"`
	Error string `json:"error,omitempty"`
}

// IGDTPushResult: resultado por elemento de un envío a IGDT.
type IGDTPushResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Event string `json:"event,omitempty"`
	Error string `json:"error,omitempty"`
}

// IGDTPusher: envíos de la bandeja de selección (lotes de hasta 50).
type IGDTPusher interface {
	PushRequirements(ctx context.Context, key string, items []json.RawMessage) ([]IGDTPushResult, error)
	PushHires(ctx context.Context, key string, items []json.RawMessage) ([]IGDTPushResult, error)
}

// IGDTOutboxRepository: cola de envíos a IGDT y snapshots (sin actor: los usa el proceso de envío).
type IGDTOutboxRepository interface {
	OutboxRQ(ctx context.Context, tenantID uint64, rqID string) error
	OutboxCandidate(ctx context.Context, tenantID uint64, candidateID string) error
	OutboxReconcile(ctx context.Context, tenantID uint64, full bool) (int, error)
	OutboxNext(ctx context.Context, tenantID uint64, limit int) ([]IGDTOutboxItem, error)
	OutboxResults(ctx context.Context, results []IGDTOutboxResult) error
	OutboxPurge(ctx context.Context) error
	OutboxStatus(ctx context.Context, tenantID uint64) (json.RawMessage, error)
	RQSnapshots(ctx context.Context, tenantID uint64, ids []string) ([]json.RawMessage, error)
	HireSnapshots(ctx context.Context, tenantID uint64, ids []string) ([]json.RawMessage, error)
	// ApplyHires aplica los eventos de ingreso (HIRED, START_CHANGED, REVOKED) y cierra los RQ cubiertos.
	ApplyHires(ctx context.Context, tenantID uint64, events json.RawMessage, cursor uint64) (*IGDTHiresApplied, error)
}

// IGDTHireEvents: una página de eventos de ingreso registrados en IGDT.
type IGDTHireEvents struct {
	Events json.RawMessage `json:"events"`
	Next   uint64          `json:"next"`
	More   bool            `json:"more"`
}

// IGDTHiresReader lee los ingresos registrados en IGDT (alcance hires.read).
type IGDTHiresReader interface {
	HireEvents(ctx context.Context, key string, since uint64, limit int) (*IGDTHireEvents, error)
}
