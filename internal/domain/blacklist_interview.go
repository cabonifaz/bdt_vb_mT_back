package domain

import (
	"encoding/json"
	"time"
)

// ---------------------------------------------------------------- Lista negra

// BlacklistEntry es una restricción activa. Client nil = global (todos los clientes).
type BlacklistEntry struct {
	ID        string          `json:"id"`
	ClientID  *string         `json:"clientId,omitempty"`
	Client    json.RawMessage `json:"client,omitempty"`
	Reason    string          `json:"reason"`
	ChangedAt time.Time       `json:"changedAt"`
	UserName  string          `json:"userName"`
}

type BlacklistMovement struct {
	ID        uint64          `json:"id"`
	Movement  string          `json:"movement"` // CREATE | UPDATE | DELETE
	Client    json.RawMessage `json:"client,omitempty"`
	Reason    string          `json:"reason"`
	CreatedAt time.Time       `json:"createdAt"`
	UserName  string          `json:"userName"`
}

type BlacklistTalent struct {
	TalentID       string    `json:"talentId"`
	FirstName      string    `json:"firstName"`
	LastName       string    `json:"lastName"`
	SecondLastName *string   `json:"secondLastName,omitempty"`
	Headline       *string   `json:"headline,omitempty"`
	LastChange     time.Time `json:"lastChange"`
	IsGlobal       bool      `json:"isGlobal"`
	ClientCount    int       `json:"clientCount"`
}

// ---------------------------------------------------------------- Entrevistas

type InterviewSummary struct {
	ID             string          `json:"id"`
	TalentID       string          `json:"talentId"`
	FirstName      string          `json:"firstName"`
	LastName       string          `json:"lastName"`
	SecondLastName *string         `json:"secondLastName,omitempty"`
	TypeCode       string          `json:"typeCode"`
	Type           json.RawMessage `json:"type"`
	StageCode      string          `json:"stageCode"`
	Stage          json.RawMessage `json:"stage"`
	StatusCode     string          `json:"statusCode"`
	Status         json.RawMessage `json:"status"`
	Client         json.RawMessage `json:"client,omitempty"`
	RequirementRef *string         `json:"requirementRef,omitempty"`
	Profile        string          `json:"profile"`
	Date           string          `json:"date"`
	Time           string          `json:"time"`
	RatingGeneral  int             `json:"ratingGeneral"`
}

type Interviewer struct {
	Name   string `json:"name"`
	Email  string `json:"email,omitempty"`
	Notify bool   `json:"notify"`
}

type Recording struct {
	URL  string `json:"url"`
	Date string `json:"date"`
}

type InterviewAnswer struct {
	QuestionID string          `json:"questionId"`
	Question   json.RawMessage `json:"question"`
	Answer     string          `json:"answer"`
	Active     bool            `json:"active"`
}

type Interview struct {
	InterviewSummary
	Headline          *string           `json:"headline,omitempty"`
	TypeID            string            `json:"typeId"`
	StageID           string            `json:"stageId"`
	StatusID          string            `json:"statusId"`
	DurationID        *string           `json:"durationId,omitempty"`
	DurationMinutes   *int              `json:"durationMinutes,omitempty"`
	ClientID          *string           `json:"clientId,omitempty"`
	MeetingLink       *string           `json:"meetingLink,omitempty"`
	LocationURL       *string           `json:"locationUrl,omitempty"`
	Address           *string           `json:"address,omitempty"`
	RatingPersonal    int               `json:"ratingPersonal"`
	RatingExperience  int               `json:"ratingExperience"`
	RatingLanguages   int               `json:"ratingLanguages"`
	RatingEducation   int               `json:"ratingEducation"`
	NotesPersonal     *string           `json:"notesPersonal,omitempty"`
	NotesExperience   *string           `json:"notesExperience,omitempty"`
	NotesLanguages    *string           `json:"notesLanguages,omitempty"`
	NotesEducation    *string           `json:"notesEducation,omitempty"`
	CancelReason      *string           `json:"cancelReason,omitempty"`
	Version           int               `json:"version"`
	CreatedAt         time.Time         `json:"createdAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
	TalentBlacklisted bool              `json:"talentBlacklisted"`
	Interviewers      []Interviewer     `json:"interviewers"`
	Recordings        []Recording       `json:"recordings"`
	Answers           []InterviewAnswer `json:"answers"`
	Files             []TalentFile      `json:"files"`
}
