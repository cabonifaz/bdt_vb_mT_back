package domain

import (
	"encoding/json"
	"slices"
	"time"
)

// Tipos de notificación: alta/actualización de talentos y de requerimientos (RQ).
const (
	NotifTalentCreated = "TALENT_CREATED"
	NotifTalentUpdated = "TALENT_UPDATED"
	NotifRQCreated     = "RQ_CREATED"
	NotifRQUpdated     = "RQ_UPDATED"
	NotifSupportNew    = "SUPPORT_NEW"   // al Super Admin: nueva solicitud
	NotifSupportReply  = "SUPPORT_REPLY" // respuesta en una solicitud (al autor o al Super Admin)
	NotifReleaseNote   = "RELEASE_NOTE"  // novedad de la plataforma, a todos
)

// NotificationTypes: tipos que el usuario puede configurar (en la app y push).
var NotificationTypes = []string{NotifTalentCreated, NotifTalentUpdated, NotifRQCreated, NotifRQUpdated, NotifSupportNew, NotifSupportReply, NotifReleaseNote}

func ValidNotificationType(t string) bool { return slices.Contains(NotificationTypes, t) }

// Notification: una notificación de la bandeja del usuario.
type Notification struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	EntityType string             `json:"entityType"`
	EntityID   string             `json:"entityId"`
	Payload    json.RawMessage    `json:"payload"`
	Count      int                `json:"count"` // cambios agrupados
	CreatedAt  time.Time          `json:"createdAt"`
	At         time.Time          `json:"at"` // última vez que se notificó al usuario
	ReadAt     *time.Time         `json:"readAt,omitempty"`
	Actor      *NotificationActor `json:"actor,omitempty"`
}

type NotificationActor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type NotificationInbox struct {
	Unread int            `json:"unread"`
	Items  []Notification `json:"items"`
}

type NotificationPref struct {
	Type  string `json:"type"`
	InApp bool   `json:"inApp"`
	Push  bool   `json:"push"`
}

type NotificationPrefs struct {
	Types   []NotificationPref `json:"types"`
	Devices int                `json:"devices"` // dispositivos con push activo
}
