package domain

import (
	"encoding/json"
	"slices"
	"time"
)

// Tipos de notificación. Los de requerimientos (RQ) se agregan cuando se integre AutFMI.
const (
	NotifTalentCreated = "TALENT_CREATED"
	NotifTalentUpdated = "TALENT_UPDATED"
)

// NotificationTypes: tipos que el usuario puede configurar (en la app y push).
var NotificationTypes = []string{NotifTalentCreated, NotifTalentUpdated}

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
