package ports

import (
	"context"
	"encoding/json"
)

// RQDoc: documento JSON armado por el SP + nombre cifrado del responsable (lo descifra la app).
type RQDoc struct {
	Doc          json.RawMessage
	OwnerNameEnc []byte
}

// RQWrite: alta o edición de un RQ. Contacts, Billing y Vacancies van como JSON al SP.
type RQWrite struct {
	ID, Code, ClientID, Title, Description, StatusID, RequestDate, DueDate string
	DurationValue, ContractValue                                           *float64
	DurationUnitID, ContractUnitID, WorkModalityID, OwnerID                *string
	NeedsEmo, NeedsSctr, NeedsLaptop                                       bool
	Contacts, Billing, Vacancies                                           json.RawMessage
	ExpectedVersion                                                        *int
}

// RQChange: resultado de guardar o mover un RQ (para notificar).
type RQChange struct {
	ID             string `json:"id"`
	Created        bool   `json:"created"`
	Version        int    `json:"version"`
	Code           string `json:"code"`
	PreviousStatus string `json:"previousStatus,omitempty"`
	Status         string `json:"status"`
	Title          string `json:"-"`
	Client         string `json:"-"`
}

type ContactWrite struct {
	ID, ClientID, FirstName, LastName, SecondLastName, Position, Phone, Phone2, Email, Email2 string
}

type RequirementRepository interface {
	Board(ctx context.Context, m Meta, tenantID uint64, search string, clientID *string, mine bool, closedDays int) ([]RQDoc, error)
	Get(ctx context.Context, m Meta, tenantID uint64, id string) (*RQDoc, error)
	Save(ctx context.Context, m Meta, tenantID uint64, w RQWrite) (*RQChange, error)
	SetStatus(ctx context.Context, m Meta, tenantID uint64, id, statusID string, order *int) (*RQChange, error)
	Delete(ctx context.Context, m Meta, tenantID uint64, id string) error
	CandidateAdd(ctx context.Context, m Meta, tenantID uint64, rqID, id, talentID, vacancyID string) (bool, error)
	CandidateMove(ctx context.Context, m Meta, tenantID uint64, id, stageID string, vacancyID *string, order *int) (string, error)
	CandidateRemove(ctx context.Context, m Meta, tenantID uint64, id string) error
	ByTalent(ctx context.Context, m Meta, tenantID uint64, talentID string) ([]json.RawMessage, error)
	Contacts(ctx context.Context, m Meta, tenantID uint64, clientID string) ([]json.RawMessage, error)
	ContactSave(ctx context.Context, m Meta, tenantID uint64, c ContactWrite) error
	ContactDelete(ctx context.Context, m Meta, tenantID uint64, id string) error
}
