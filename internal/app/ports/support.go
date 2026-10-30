package ports

import (
	"context"
	"encoding/json"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// SupportNew: alta de una solicitud al Super Admin (observación, incidencia o mejora).
type SupportNew struct {
	ID, MessageID, Kind, Impact, Title, Body string
	Origin, Refs, Files                      json.RawMessage
}

// SupportReplied: resultado de una respuesta (para notificar al otro lado).
type SupportReplied struct {
	ID, Title, Status, TenantSlug string
	Number                        int
	Staff                         bool
	TenantID, ReporterID          uint64
}

type SupportRepository interface {
	Create(ctx context.Context, m Meta, tenantID uint64, in SupportNew) (int, error)
	Reply(ctx context.Context, m Meta, id, messageID, body, status string, files json.RawMessage) (*SupportReplied, error)
	List(ctx context.Context, m Meta, status, search string, limit int) ([]json.RawMessage, error)
	Get(ctx context.Context, m Meta, id string) (json.RawMessage, error)
	File(ctx context.Context, m Meta, fileID string) (*domain.FileObject, error)
	CreateFile(ctx context.Context, m Meta, tenantID *uint64, f domain.FileObject) error
	PublishRelease(ctx context.Context, m Meta, id string, title, body json.RawMessage, module, tour string) error
	Releases(ctx context.Context, m Meta, limit int) ([]json.RawMessage, error)
	Tours(ctx context.Context, m Meta) (map[string]int, error)
	TourSeen(ctx context.Context, m Meta, key string, version int) error
}

// Audiencias de una notificación sin agrupar (soporte y novedades).
const (
	AudienceSuperAdmins = "SUPER_ADMINS"
	AudienceUser        = "USER"
	AudienceAll         = "ALL"
)
