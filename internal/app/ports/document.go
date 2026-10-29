package ports

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// DocRule: regla del maestro de documentos de postulación del tenant.
type DocRule struct {
	TypeID       string `json:"typeId"`
	DefaultMode  string `json:"defaultMode"`  // REQUIRED | OPTIONAL
	Reusable     bool   `json:"reusable"`     // sirve para cualquier RQ mientras esté vigente
	ValidityDays *int   `json:"validityDays"` // vigencia desde la emisión
	Verification string `json:"verification"` // MANUAL | AUTO_CUL
	Active       bool   `json:"active"`
	SortOrder    int    `json:"sortOrder"`
}

// DocContext: lo necesario para validar un documento presentado.
type DocContext struct {
	ID, TypeCode, Verification string
	ValidityDays               *int
	TenantRUC                  string
	TalentDocEnc               []byte
	TalentDocType              string
	FileID, FileSHA256         string
}

// DocVerdict: resultado de una validación (manual o automática).
type DocVerdict struct {
	Status, Method, Reason string // PENDING | VERIFIED | REJECTED; MANUAL | AUTO
	Result                 json.RawMessage
	IssuedAt, ExpiresAt    *time.Time
}

type DocumentRepository interface {
	Rules(ctx context.Context, m Meta, tenantID uint64) ([]json.RawMessage, error)
	SaveRule(ctx context.Context, m Meta, tenantID uint64, r DocRule) error
	CandidateDetail(ctx context.Context, m Meta, tenantID uint64, candidateID, today string) (json.RawMessage, error)
	CandidateTalent(ctx context.Context, m Meta, tenantID uint64, candidateID string) (string, error)
	Create(ctx context.Context, m Meta, tenantID uint64, id, candidateID, typeID, fileID, today string) (*DocContext, error)
	Context(ctx context.Context, m Meta, tenantID uint64, id string) (*DocContext, error)
	Verify(ctx context.Context, m Meta, tenantID uint64, id, candidateID string, v DocVerdict) error
}

// CULCheck: respuesta del servicio de verificación del MTPE.
type CULCheck struct {
	Found, Vigente bool
	Code, Names    string
	IssuedAt       time.Time
	OfficialSHA256 string // huella del PDF oficial
	Message        string // motivo si no se encontró
}

var ErrCULDocType = errors.New("cul: tipo de documento no admitido por el MTPE")

// CULVerifier consulta el CUL con el RUC de la empresa que verifica.
type CULVerifier interface {
	Check(ctx context.Context, ruc, code, docType, docNumber string) (*CULCheck, error)
}
