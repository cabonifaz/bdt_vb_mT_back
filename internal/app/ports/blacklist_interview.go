package ports

import (
	"context"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// ---------------------------------------------------------------- Lista negra

type BlacklistEntryRecord struct {
	domain.BlacklistEntry
	UserNameEnc []byte
}

type BlacklistMovementRecord struct {
	domain.BlacklistMovement
	UserNameEnc []byte
}

type BlacklistKeep struct {
	Client string `json:"client"`
	Reason string `json:"reason"`
}

type BlacklistRepository interface {
	Search(ctx context.Context, m Meta, tenantID uint64, search, clientUUID string, globalOnly bool, page, size int) (*domain.Page[domain.BlacklistTalent], error)
	Status(ctx context.Context, m Meta, tenantID uint64, talentUUID string) ([]BlacklistEntryRecord, error)
	History(ctx context.Context, m Meta, tenantID uint64, talentUUID string, page, size int) (int, []BlacklistMovementRecord, error)
	Create(ctx context.Context, m Meta, tenantID uint64, uuid, talentUUID, clientUUID, reason string) ([]BlacklistEntryRecord, error)
	Update(ctx context.Context, m Meta, tenantID uint64, uuid, reason string) ([]BlacklistEntryRecord, error)
	Remove(ctx context.Context, m Meta, tenantID uint64, uuid, reason string) ([]BlacklistEntryRecord, error)
	LiftGlobal(ctx context.Context, m Meta, tenantID uint64, uuid, reason string, keep []BlacklistKeep) ([]BlacklistEntryRecord, error)
}

// ---------------------------------------------------------------- Entrevistas

type InterviewFilter struct {
	Search, StatusID, StageID, TypeID, ClientID, Date, TalentID string
	Page, PageSize                                              int
}

// InterviewerWrite: email ya cifrado y en hex (el SP lo guarda con UNHEX).
type InterviewerWrite struct {
	Name     string `json:"name"`
	EmailEnc string `json:"emailEnc,omitempty"`
	Notify   int    `json:"notify"`
}

type InterviewWrite struct {
	UUID, TalentID, TypeID, StageID, StatusID      string
	DurationID, ClientID, RequirementRef           *string
	Profile, Date, Time                            string
	MeetingLink, LocationURL, Address              *string
	RGeneral, RPersonal, RExperience, RLanguages   int
	REducation                                     int
	NPersonal, NExperience, NLanguages, NEducation *string
	CancelReason                                   *string
	Interviewers                                   []InterviewerWrite
	Recordings                                     []domain.Recording
	AckBlacklist                                   bool
	ExpectedVersion                                *int
}

type InterviewWriteResult struct {
	ID       string `json:"id"`
	Version  int    `json:"version"`
	Created  bool   `json:"created"`
	TypeCode string `json:"typeCode"`
}

type AnswerWrite struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type InterviewRecord struct {
	domain.Interview
	InterviewerEmailEnc [][]byte
}

type InterviewRepository interface {
	Search(ctx context.Context, m Meta, tenantID uint64, f InterviewFilter) (*domain.Page[domain.InterviewSummary], error)
	Get(ctx context.Context, m Meta, tenantID uint64, uuid string) (*InterviewRecord, error)
	Save(ctx context.Context, m Meta, tenantID uint64, w InterviewWrite) (*InterviewWriteResult, error)
	SaveAnswers(ctx context.Context, m Meta, tenantID uint64, uuid string, answers []AnswerWrite) error
	CreateFile(ctx context.Context, m Meta, tenantID uint64, interviewUUID, typeUUID string, f domain.FileObject) (*domain.TalentFile, error)
	GetFile(ctx context.Context, m Meta, tenantID uint64, uuid string) (*domain.FileObject, error)
	DeleteFile(ctx context.Context, m Meta, tenantID uint64, uuid string) error
}
