package domain

import "encoding/json"

// HomeSummary: lo que el usuario tiene hoy y en los próximos días, y su carga del mes.
type HomeSummary struct {
	Today      string           `json:"today"`
	Interviews []HomeInterview  `json:"interviews"`
	Talents    []HomeTalent     `json:"talents"`
	Months     []HomeMonthStats `json:"months"`
}

type HomeInterview struct {
	ID             string          `json:"id"`
	TalentID       string          `json:"talentId"`
	FirstName      string          `json:"firstName"`
	LastName       string          `json:"lastName"`
	SecondLastName *string         `json:"secondLastName,omitempty"`
	TypeCode       string          `json:"typeCode"`
	Stage          json.RawMessage `json:"stage"`
	StatusCode     string          `json:"statusCode"`
	Status         json.RawMessage `json:"status"`
	Client         json.RawMessage `json:"client,omitempty"`
	RequirementRef *string         `json:"requirementRef,omitempty"`
	Profile        string          `json:"profile"`
	Date           string          `json:"date"`
	Time           string          `json:"time"`
	IsMine         bool            `json:"isMine"`
}

type HomeTalent struct {
	ID             string  `json:"id"`
	FirstName      string  `json:"firstName"`
	LastName       string  `json:"lastName"`
	SecondLastName *string `json:"secondLastName,omitempty"`
	Headline       *string `json:"headline,omitempty"`
	PhotoID        *string `json:"photoId,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	IsNew          bool    `json:"isNew"`
}

type HomeMonthStats struct {
	Month        string `json:"month"` // YYYY-MM
	MyPlanned    int    `json:"myPlanned"`
	MyDone       int    `json:"myDone"`
	AreaPlanned  int    `json:"areaPlanned"`
	AreaDone     int    `json:"areaDone"`
	TalentsNew   int    `json:"talentsNew"`
	MyTalentsNew int    `json:"myTalentsNew"`
}
