package ports

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// NotifyInput: evento a notificar. GroupWith y GroupMinutes agrupan cambios seguidos del mismo actor y entidad.
type NotifyInput struct {
	ID, Type, GroupWith, EntityType, EntityID string
	Payload                                   json.RawMessage
	RecipientPerm                             string
	GroupMinutes                              int
}

// PushTarget: suscripción push de un destinatario (con su idioma para el texto).
type PushTarget struct {
	Hash, Endpoint, P256dh, Auth, Locale string
}

type NotifyResult struct {
	ID           string
	New          bool
	ActorNameEnc []byte
	Targets      []PushTarget // solo si la notificación es nueva
}

// NotificationRecord trae el nombre del actor cifrado tal como está en la BD.
type NotificationRecord struct {
	domain.Notification
	ActorID      string
	ActorNameEnc []byte
}

type NotificationRepository interface {
	Notify(ctx context.Context, m Meta, tenantID uint64, in NotifyInput) (*NotifyResult, error)
	List(ctx context.Context, m Meta, tenantID uint64, unreadOnly bool, before *time.Time, limit int) (int, []NotificationRecord, error)
	MarkRead(ctx context.Context, m Meta, tenantID uint64, id *string) error
	Subscribe(ctx context.Context, m Meta, tenantID uint64, hash, endpoint, p256dh, auth, ua string) (int, error)
	Unsubscribe(ctx context.Context, m Meta, tenantID uint64, hash string) error
	PushResult(ctx context.Context, hash string, ok, gone bool) error
	Prefs(ctx context.Context, m Meta, tenantID uint64) ([]domain.NotificationPref, int, error)
	SetPref(ctx context.Context, m Meta, tenantID uint64, p domain.NotificationPref) error
}

// PushSender envía notificaciones push del sistema operativo (Web Push con VAPID).
type PushSender interface {
	Enabled() bool
	PublicKey() string
	// Send devuelve gone = true si la suscripción ya no existe (404/410) y debe borrarse.
	Send(ctx context.Context, t PushTarget, payload []byte) (gone bool, err error)
}
