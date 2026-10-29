package ports

import (
	"context"
	"encoding/json"
)

// RQDoc: documento JSON armado por el SP; los responsables traen el nombre cifrado (owners[].nameEnc, lo descifra la app).
type RQDoc struct {
	Doc json.RawMessage
}

// RQWrite: alta o edición de un RQ. Contacts, Billing y Vacancies van como JSON al SP.
type RQWrite struct {
	ID, Code, ClientID, Title, Description, StatusID, RequestDate, DueDate string
	DurationValue, ContractValue                                           *float64
	DurationUnitID, ContractUnitID, WorkModalityID                         *string
	NeedsEmo, NeedsSctr, NeedsLaptop                                       bool
	Owners, Contacts, Billing, Vacancies                                   json.RawMessage
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

// BoardQuery: vista del tablero (ACTIVE, SOON, OVERDUE, NEW, CLOSED, ALL) y filtros; Today es la fecha local del usuario.
type BoardQuery struct {
	Scope, Search, Today           string
	ClientID, ContactID, ProfileID *string
	Mine                           bool
}

// BoardCounts: cantidad de RQ por vista con los mismos filtros.
type BoardCounts struct {
	Active  int `json:"active"`
	Soon    int `json:"soon"`
	Overdue int `json:"overdue"`
	New     int `json:"new"`
	Closed  int `json:"closed"`
	All     int `json:"all"`
}

type ContactWrite struct {
	ID, ClientID, FirstName, LastName, SecondLastName, Position, Phone, Phone2, Email, Email2 string
}

type RequirementRepository interface {
	Board(ctx context.Context, m Meta, tenantID uint64, f BoardQuery) ([]RQDoc, BoardCounts, error)
	Get(ctx context.Context, m Meta, tenantID uint64, id string) (*RQDoc, error)
	Save(ctx context.Context, m Meta, tenantID uint64, w RQWrite) (*RQChange, error)
	SetStatus(ctx context.Context, m Meta, tenantID uint64, id, statusID string, order *int) (*RQChange, error)
	Reopen(ctx context.Context, m Meta, tenantID uint64, id, reason, notes string, candidates []string, statusID *string) (*RQChange, error)
	Delete(ctx context.Context, m Meta, tenantID uint64, id string) error
	CandidateAdd(ctx context.Context, m Meta, tenantID uint64, rqID, id, talentID, vacancyID string) (bool, error)
	CandidateMove(ctx context.Context, m Meta, tenantID uint64, id, stageID string, vacancyID *string, order *int) (string, error)
	CandidateRemove(ctx context.Context, m Meta, tenantID uint64, id string) error
	ByTalent(ctx context.Context, m Meta, tenantID uint64, talentID string) ([]json.RawMessage, error)
	Contacts(ctx context.Context, m Meta, tenantID uint64, clientID string) ([]json.RawMessage, error)
	ContactSave(ctx context.Context, m Meta, tenantID uint64, c ContactWrite) error
	ContactDelete(ctx context.Context, m Meta, tenantID uint64, id string) error
}
