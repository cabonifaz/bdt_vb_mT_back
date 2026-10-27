package domain

import (
	"encoding/json"
	"time"
)

type SkillRef struct {
	ID    string          `json:"id"`
	Label json.RawMessage `json:"label"`
	Years *float64        `json:"years,omitempty"`
}

// TalentSummary es la fila del listado de búsqueda.
type TalentSummary struct {
	ID              string          `json:"id"`
	FirstName       string          `json:"firstName"`
	LastName        string          `json:"lastName"`
	SecondLastName  *string         `json:"secondLastName,omitempty"`
	Headline        *string         `json:"headline,omitempty"`
	Country         json.RawMessage `json:"country,omitempty"`
	City            *string         `json:"city,omitempty"`
	AvgRating       *float64        `json:"avgRating,omitempty"`
	PhotoID         *string         `json:"photoId,omitempty"`
	YearsExperience float64         `json:"yearsExperience"`
	IsFavorite      bool            `json:"isFavorite"`
	Skills          []SkillRef      `json:"skills"`
	EnglishLevel    *string         `json:"englishLevel,omitempty"`
	Version         int             `json:"version"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// Rates: expectativa salarial (se guarda cifrada; solo visible con talent.rates.read).
type Rates struct {
	ModalityID        *string  `json:"modalityId,omitempty"`
	PayrollMin        *float64 `json:"payrollMin,omitempty"`
	PayrollMax        *float64 `json:"payrollMax,omitempty"`
	PayrollCurrencyID *string  `json:"payrollCurrencyId,omitempty"`
	RxhMin            *float64 `json:"rxhMin,omitempty"`
	RxhMax            *float64 `json:"rxhMax,omitempty"`
	RxhCurrencyID     *string  `json:"rxhCurrencyId,omitempty"`
	Notes             *string  `json:"notes,omitempty"`
}

type TalentTechSkill struct {
	ID      string          `json:"id"`
	SkillID string          `json:"skillId"`
	Label   json.RawMessage `json:"label"`
	Years   float64         `json:"years"`
}

type TalentSoftSkill struct {
	ID      string          `json:"id"`
	SkillID string          `json:"skillId"`
	Label   json.RawMessage `json:"label"`
}

type TalentExperience struct {
	ID        string  `json:"id"`
	Company   string  `json:"company"`
	Position  string  `json:"position"`
	StartDate string  `json:"startDate"`
	EndDate   *string `json:"endDate,omitempty"`
	IsCurrent bool    `json:"isCurrent"`
	Functions *string `json:"functions,omitempty"`
	Months    int     `json:"months"`
}

type TalentEducation struct {
	ID            string          `json:"id"`
	Institution   string          `json:"institution"`
	Career        string          `json:"career"`
	GradeID       *string         `json:"gradeId,omitempty"`
	Grade         json.RawMessage `json:"grade,omitempty"`
	StartDate     *string         `json:"startDate,omitempty"`
	EndDate       *string         `json:"endDate,omitempty"`
	IsCurrent     bool            `json:"isCurrent"`
	DatePrecision string          `json:"datePrecision"`
}

type TalentLanguage struct {
	ID         string          `json:"id"`
	LanguageID string          `json:"languageId"`
	Language   json.RawMessage `json:"language"`
	LevelID    string          `json:"levelId"`
	Level      json.RawMessage `json:"level"`
	LevelCode  string          `json:"levelCode"`
	Stars      int             `json:"stars"`
}

type TalentFeedback struct {
	ID         string    `json:"id"`
	Stars      int       `json:"stars"`
	Comment    string    `json:"comment"`
	AuthorID   string    `json:"authorId"`
	AuthorName string    `json:"authorName"`
	CreatedAt  time.Time `json:"createdAt"`
	IsMine     bool      `json:"isMine"`
	CanDelete  bool      `json:"canDelete"`
}

type TalentFile struct {
	ID           string    `json:"id"`
	Category     string    `json:"category"`
	OriginalName string    `json:"name"`
	MimeType     string    `json:"mimeType"`
	Size         int64     `json:"size"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Talent struct {
	ID             string             `json:"id"`
	FirstName      string             `json:"firstName"`
	LastName       string             `json:"lastName"`
	SecondLastName *string            `json:"secondLastName,omitempty"`
	Headline       *string            `json:"headline,omitempty"`
	DocTypeID      *string            `json:"docTypeId,omitempty"`
	DocNumber      string             `json:"docNumber,omitempty"`
	Email          string             `json:"email,omitempty"`
	Phone          string             `json:"phone,omitempty"`
	CountryID      *string            `json:"countryId,omitempty"`
	Country        json.RawMessage    `json:"country,omitempty"`
	City           *string            `json:"city,omitempty"`
	LinkedinURL    *string            `json:"linkedinUrl,omitempty"`
	GithubURL      *string            `json:"githubUrl,omitempty"`
	Summary        *string            `json:"summary,omitempty"`
	Availability   *string            `json:"availability,omitempty"`
	HasEquipment   *bool              `json:"hasEquipment,omitempty"`
	Rates          *Rates             `json:"rates,omitempty"`
	CanSeeRates    bool               `json:"canSeeRates"`
	PhotoID        *string            `json:"photoId,omitempty"`
	CvID           *string            `json:"cvId,omitempty"`
	AvgRating      *float64           `json:"avgRating,omitempty"`
	Status         string             `json:"status"`
	Source         string             `json:"source"`
	Version        int                `json:"version"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
	TechSkills     []TalentTechSkill  `json:"techSkills"`
	SoftSkills     []TalentSoftSkill  `json:"softSkills"`
	Experiences    []TalentExperience `json:"experiences"`
	Educations     []TalentEducation  `json:"educations"`
	Languages      []TalentLanguage   `json:"languages"`
	Feedback       []TalentFeedback   `json:"feedback"`
	Files          []TalentFile       `json:"files"`
	Collections    []string           `json:"collections"`
}

type Collection struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	TalentCount int       `json:"talentCount"`
	Access      string    `json:"access"` // OWNER | EDIT | VIEW
	OwnerID     string    `json:"ownerId"`
	OwnerName   string    `json:"ownerName"`
	ShareCount  int       `json:"shareCount"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CollectionShare struct {
	UserID     string    `json:"userId"`
	Name       string    `json:"name"`
	Email      string    `json:"email"`
	Permission string    `json:"permission"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Colleague struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}
