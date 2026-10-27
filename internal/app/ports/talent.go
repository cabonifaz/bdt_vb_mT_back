package ports

import (
	"context"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type TalentFilter struct {
	Search, Skills, EnglishLevel, Position, Education, GradeID, CollectionID, Status string
	MinYears                                                                         *float64
	Page, PageSize                                                                   int
}

// TalentRecord trae los campos cifrados tal como están en la BD.
type TalentRecord struct {
	domain.Talent
	DocEnc, EmailEnc, PhoneEnc, RatesEnc []byte
	BirthEnc                             []byte
	FeedbackAuthorEnc                    [][]byte
}

// TalentWrite son los datos principales ya cifrados por el caso de uso.
type TalentWrite struct {
	UUID, FirstName, LastName           string
	SecondLastName, Headline, DocTypeID *string
	DocEnc                              []byte
	DocBidx                             *string
	EmailEnc                            []byte
	EmailBidx                           *string
	PhoneEnc, BirthEnc                  []byte
	CountryID, City, Linkedin, Github   *string
	Summary, AvailabilityID             *string
	HasEquipment                        *bool
	RatesEnc                            []byte
	SetRates                            bool
	ExpectedVersion                     *int
	Source                              string
}

type TalentWriteResult struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Created bool   `json:"created"`
}

type TalentRepository interface {
	Search(ctx context.Context, m Meta, tenantID uint64, f TalentFilter) (*domain.Page[domain.TalentSummary], error)
	Get(ctx context.Context, m Meta, tenantID uint64, uuid string) (*TalentRecord, error)
	Upsert(ctx context.Context, m Meta, tenantID uint64, w TalentWrite) (*TalentWriteResult, error)
	SetStatus(ctx context.Context, m Meta, tenantID uint64, uuid, status string) error
	SetAvailability(ctx context.Context, m Meta, tenantID uint64, uuid string, availabilityID *string) error
	SetEquipment(ctx context.Context, m Meta, tenantID uint64, uuid string, hasEquipment *bool) error

	EnsureSkill(ctx context.Context, m Meta, tenantID uint64, table, uuid, label string) (*domain.SkillRef, error)
	UpsertTechSkill(ctx context.Context, m Meta, tenantID uint64, talentUUID, uuid, skillUUID string, years float64) error
	AddSoftSkill(ctx context.Context, m Meta, tenantID uint64, talentUUID, uuid, skillUUID string) error
	DeleteSection(ctx context.Context, m Meta, tenantID uint64, section, uuid string) error
	UpsertExperience(ctx context.Context, m Meta, tenantID uint64, talentUUID string, e domain.TalentExperience) error
	UpsertEducation(ctx context.Context, m Meta, tenantID uint64, talentUUID string, e domain.TalentEducation) error
	UpsertLanguage(ctx context.Context, m Meta, tenantID uint64, talentUUID, uuid, languageUUID, levelUUID string, stars int) error
	UpsertFeedback(ctx context.Context, m Meta, tenantID uint64, talentUUID, uuid string, stars int, comment string) (*float64, error)
	DeleteFeedback(ctx context.Context, m Meta, tenantID uint64, uuid string) error

	CreateFile(ctx context.Context, m Meta, tenantID uint64, talentUUID string, f domain.FileObject) (*domain.TalentFile, error)
	GetFile(ctx context.Context, m Meta, tenantID uint64, uuid string) (*domain.FileObject, error)
	DeleteFile(ctx context.Context, m Meta, tenantID uint64, uuid string) error

	Collections(ctx context.Context, m Meta, tenantID uint64) ([]CollectionRecord, error)
	CreateCollection(ctx context.Context, m Meta, tenantID uint64, uuid, name string) ([]CollectionRecord, error)
	RenameCollection(ctx context.Context, m Meta, tenantID uint64, uuid, name string) error
	CollectionShares(ctx context.Context, m Meta, tenantID uint64, uuid string) ([]PersonRecord, error)
	ShareCollection(ctx context.Context, m Meta, tenantID uint64, uuid, userUUID, permission string) ([]PersonRecord, error)
	UnshareCollection(ctx context.Context, m Meta, tenantID uint64, uuid, userUUID string) error
	Colleagues(ctx context.Context, m Meta, tenantID uint64) ([]PersonRecord, error)
	DeleteCollection(ctx context.Context, m Meta, tenantID uint64, uuid string) error
	SetCollectionTalent(ctx context.Context, m Meta, tenantID uint64, collectionUUID, talentUUID string, in bool) error
}

// CollectionRecord trae el nombre del dueño cifrado.
type CollectionRecord struct {
	domain.Collection
	OwnerNameEnc []byte
}

// PersonRecord: usuario con nombre/correo cifrados (compartidos, directorio).
type PersonRecord struct {
	ID                string
	NameEnc, EmailEnc []byte
	Permission        string
	CreatedAt         time.Time
}

// SyncRepository registra operaciones de la cola offline (idempotencia).
type SyncRepository interface {
	Register(ctx context.Context, tenantID, userID uint64, opID, entity, op string) (duplicate bool, status string, result []byte, err error)
	Complete(ctx context.Context, tenantID uint64, opID, status string, result []byte) error
}
