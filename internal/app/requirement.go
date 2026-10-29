package app

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// RequirementService: requerimientos (RQ) con vacantes, tablero por estado, pipeline de candidatos y contactos del cliente.
// El tablero y la ficha vienen como JSON del SP (que ya omite tarifas sin rq.rates.read); aquí se valida la entrada,
// se descifra el nombre del responsable y se notifica el alta o cambio del RQ.
type RequirementService struct {
	Repo     ports.RequirementRepository
	Cipher   ports.Cipher
	Notifier RQNotifier
}

// RQNotifier avisa a los usuarios del tenant cuando se crea o cambia un RQ.
type RQNotifier interface {
	RQChanged(ctx context.Context, p *domain.Principal, c *ports.RQChange)
}

type RQSkillInput struct {
	SkillID  string   `json:"skillId"`
	Years    *float64 `json:"years"`
	Optional bool     `json:"optional"`
}

type RQCareerInput struct {
	Career   string  `json:"career"`
	GradeID  *string `json:"gradeId"`
	Optional bool    `json:"optional"`
}

type RQVacancyInput struct {
	ID          string          `json:"id"`
	ProfileID   string          `json:"profileId"`
	Quantity    int             `json:"quantity"`
	RateInitial *float64        `json:"rateInitial"`
	RateFinal   *float64        `json:"rateFinal"`
	Skills      []RQSkillInput  `json:"skills"`
	Careers     []RQCareerInput `json:"careers"`
}

type RQBillingInput struct {
	ContractID    string   `json:"contractId"`
	CurrencyID    string   `json:"currencyId"`
	BaseMin       *float64 `json:"baseMin"`
	BaseMax       *float64 `json:"baseMax"`
	MobilityMin   *float64 `json:"mobilityMin"`
	MobilityMax   *float64 `json:"mobilityMax"`
	MonthlyMin    *float64 `json:"monthlyMin"`
	MonthlyMax    *float64 `json:"monthlyMax"`
	QuarterlyMin  *float64 `json:"quarterlyMin"`
	QuarterlyMax  *float64 `json:"quarterlyMax"`
	SemiannualMin *float64 `json:"semiannualMin"`
	SemiannualMax *float64 `json:"semiannualMax"`
}

type RQInput struct {
	Code            string           `json:"code"` // vacío = autogenerado
	ClientID        string           `json:"clientId"`
	Title           string           `json:"title"`
	Description     string           `json:"description"`
	StatusID        string           `json:"statusId"`
	RequestDate     string           `json:"requestDate"`
	DueDate         string           `json:"dueDate"`
	DurationValue   *float64         `json:"durationValue"`
	DurationUnitID  *string          `json:"durationUnitId"`
	ContractValue   *float64         `json:"contractValue"`
	ContractUnitID  *string          `json:"contractUnitId"`
	WorkModalityID  *string          `json:"workModalityId"`
	OwnerID         *string          `json:"ownerId"`
	NeedsEmo        bool             `json:"needsEmo"`
	NeedsSctr       bool             `json:"needsSctr"`
	NeedsLaptop     bool             `json:"needsLaptop"`
	Contacts        []string         `json:"contacts"`
	Billing         []RQBillingInput `json:"billing"` // null = no tocar (o sin permiso de tarifas)
	Vacancies       []RQVacancyInput `json:"vacancies"`
	ExpectedVersion *int             `json:"expectedVersion"`
}

var rqCode = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,40}$`)

func optRef(s *string) bool { return s == nil || *s == "" || validUUID(*s) }

func nilIfEmpty(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

func money(vals ...*float64) bool {
	for _, v := range vals {
		if v != nil && (*v < 0 || *v > 1e10) {
			return false
		}
	}
	return true
}

func rangeOK(min, max *float64) bool { return min == nil || max == nil || *max >= *min }

func validRQ(in *RQInput) bool {
	in.Title, in.Description, in.Code = strings.TrimSpace(in.Title), strings.TrimSpace(in.Description), strings.TrimSpace(in.Code)
	req, err1 := time.Parse("2006-01-02", in.RequestDate)
	due, err2 := time.Parse("2006-01-02", in.DueDate)
	if err1 != nil || err2 != nil || due.Before(req) || in.Title == "" || utf8.RuneCountInString(in.Title) > 200 ||
		utf8.RuneCountInString(in.Description) > 2000 || (in.Code != "" && !rqCode.MatchString(in.Code)) ||
		!validUUID(in.ClientID) || !validUUID(in.StatusID) || !optRef(in.DurationUnitID) || !optRef(in.ContractUnitID) ||
		!optRef(in.WorkModalityID) || !optRef(in.OwnerID) || len(in.Contacts) > 30 || len(in.Billing) > 10 ||
		len(in.Vacancies) == 0 || len(in.Vacancies) > 30 {
		return false
	}
	for _, d := range []*float64{in.DurationValue, in.ContractValue} {
		if d != nil && (*d <= 0 || *d > 9999) {
			return false
		}
	}
	for _, c := range in.Contacts {
		if !validUUID(c) {
			return false
		}
	}
	for _, b := range in.Billing {
		if !validUUID(b.ContractID) || !validUUID(b.CurrencyID) ||
			!money(b.BaseMin, b.BaseMax, b.MobilityMin, b.MobilityMax, b.MonthlyMin, b.MonthlyMax, b.QuarterlyMin, b.QuarterlyMax, b.SemiannualMin, b.SemiannualMax) ||
			!rangeOK(b.BaseMin, b.BaseMax) || !rangeOK(b.MobilityMin, b.MobilityMax) || !rangeOK(b.MonthlyMin, b.MonthlyMax) ||
			!rangeOK(b.QuarterlyMin, b.QuarterlyMax) || !rangeOK(b.SemiannualMin, b.SemiannualMax) {
			return false
		}
	}
	seen := map[string]bool{}
	for i := range in.Vacancies {
		v := &in.Vacancies[i]
		if !validUUID(v.ID) || seen[v.ID] || !validUUID(v.ProfileID) || v.Quantity < 1 || v.Quantity > 500 ||
			!money(v.RateInitial, v.RateFinal) || !rangeOK(v.RateInitial, v.RateFinal) || len(v.Skills) > 40 || len(v.Careers) > 20 {
			return false
		}
		seen[v.ID] = true
		for _, s := range v.Skills {
			if !validUUID(s.SkillID) || (s.Years != nil && (*s.Years < 0 || *s.Years > 60)) {
				return false
			}
		}
		for j := range v.Careers {
			v.Careers[j].Career = strings.TrimSpace(v.Careers[j].Career)
			if v.Careers[j].Career == "" || utf8.RuneCountInString(v.Careers[j].Career) > 150 || !optRef(v.Careers[j].GradeID) {
				return false
			}
			v.Careers[j].GradeID = nilIfEmpty(v.Careers[j].GradeID)
		}
	}
	return true
}

// withOwner agrega al documento el nombre (descifrado) del responsable.
func (s *RequirementService) withOwner(d ports.RQDoc) json.RawMessage {
	if len(d.OwnerNameEnc) == 0 {
		return d.Doc
	}
	name, err := s.Cipher.Decrypt(d.OwnerNameEnc)
	if err != nil {
		return d.Doc
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(d.Doc, &m) != nil {
		return d.Doc
	}
	m["ownerName"], _ = json.Marshal(name)
	out, _ := json.Marshal(m)
	return out
}

// BoardFilter: lo que pide el tablero. Scope vacío = ACTIVE (abiertos sin vencer); Today = fecha local del usuario.
type BoardFilter struct {
	Scope, Search, ClientID, ContactID, ProfileID, Today string
	Mine                                                 bool
}

// BoardResult: tarjetas de la vista pedida y cantidad de RQ por vista.
type BoardResult struct {
	Cards  []json.RawMessage `json:"cards"`
	Counts ports.BoardCounts `json:"counts"`
}

var boardScopes = map[string]bool{"ACTIVE": true, "SOON": true, "OVERDUE": true, "NEW": true, "CLOSED": true, "ALL": true}

// Board: tarjetas del tablero según la vista y los filtros.
func (s *RequirementService) Board(ctx context.Context, p *domain.Principal, f BoardFilter) (*BoardResult, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if f.Scope == "" {
		f.Scope = "ACTIVE"
	}
	if !boardScopes[f.Scope] || utf8.RuneCountInString(f.Search) > 100 {
		return nil, domain.ErrValidation
	}
	q := ports.BoardQuery{Scope: f.Scope, Search: strings.TrimSpace(f.Search), Mine: f.Mine, Today: boardToday(f.Today, time.Now())}
	for _, x := range []struct {
		in  string
		out **string
	}{{f.ClientID, &q.ClientID}, {f.ContactID, &q.ContactID}, {f.ProfileID, &q.ProfileID}} {
		if x.in == "" {
			continue
		}
		if !validUUID(x.in) {
			return nil, domain.ErrValidation
		}
		v := x.in
		*x.out = &v
	}
	docs, counts, err := s.Repo.Board(ctx, meta(p, ""), tid, q)
	if err != nil {
		return nil, err
	}
	out := &BoardResult{Cards: make([]json.RawMessage, 0, len(docs)), Counts: counts}
	for _, d := range docs {
		out.Cards = append(out.Cards, s.withOwner(d))
	}
	return out, nil
}

// boardToday: la fecha local que envía el navegador (define qué está vencido); si falta o se aleja más de un día
// de la fecha del servidor (zonas horarias de -12 a +14), se usa la del servidor en UTC.
func boardToday(in string, now time.Time) string {
	utc := now.UTC()
	if d, err := time.Parse("2006-01-02", in); err == nil && d.After(utc.AddDate(0, 0, -2)) && d.Before(utc.AddDate(0, 0, 2)) {
		return in
	}
	return utc.Format("2006-01-02")
}

func (s *RequirementService) Get(ctx context.Context, p *domain.Principal, id string) (json.RawMessage, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(id) {
		return nil, domain.ErrNotFound
	}
	d, err := s.Repo.Get(ctx, meta(p, ""), tid, id)
	if err != nil {
		return nil, err
	}
	return s.withOwner(*d), nil
}

func (s *RequirementService) Save(ctx context.Context, p *domain.Principal, ip, id string, in RQInput) (*ports.RQChange, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(id) || !validRQ(&in) {
		return nil, domain.ErrValidation
	}
	contacts, _ := json.Marshal(in.Contacts)
	vacancies, _ := json.Marshal(in.Vacancies)
	var billing json.RawMessage
	if in.Billing != nil {
		billing, _ = json.Marshal(in.Billing)
	}
	c, err := s.Repo.Save(ctx, meta(p, ip), tid, ports.RQWrite{
		ID: id, Code: in.Code, ClientID: in.ClientID, Title: in.Title, Description: in.Description, StatusID: in.StatusID,
		RequestDate: in.RequestDate, DueDate: in.DueDate, DurationValue: in.DurationValue, DurationUnitID: nilIfEmpty(in.DurationUnitID),
		ContractValue: in.ContractValue, ContractUnitID: nilIfEmpty(in.ContractUnitID), WorkModalityID: nilIfEmpty(in.WorkModalityID),
		OwnerID: nilIfEmpty(in.OwnerID), NeedsEmo: in.NeedsEmo, NeedsSctr: in.NeedsSctr, NeedsLaptop: in.NeedsLaptop,
		Contacts: contacts, Billing: billing, Vacancies: vacancies, ExpectedVersion: in.ExpectedVersion,
	})
	if err == nil && c != nil && s.Notifier != nil {
		s.Notifier.RQChanged(ctx, p, c)
	}
	return c, err
}

// SetStatus: mover la tarjeta en el tablero (estado y orden en la columna).
func (s *RequirementService) SetStatus(ctx context.Context, p *domain.Principal, ip, id, statusID string, order *int) (*ports.RQChange, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(id) || !validUUID(statusID) || (order != nil && (*order < 0 || *order > 1_000_000)) {
		return nil, domain.ErrValidation
	}
	c, err := s.Repo.SetStatus(ctx, meta(p, ip), tid, id, statusID, order)
	if err == nil && c != nil && c.PreviousStatus != c.Status && s.Notifier != nil {
		s.Notifier.RQChanged(ctx, p, c)
	}
	return c, err
}

// Reopen: reabre un RQ cerrado (RESIGNATION: renunció un talento; MORE_VACANCIES: se amplían las vacantes).
// Por renuncia, candidates son los confirmados que renunciaron (al menos uno). statusID opcional: estado abierto
// al que vuelve (por defecto En selección).
func (s *RequirementService) Reopen(ctx context.Context, p *domain.Principal, ip, id, reason, notes string, candidates []string, statusID string) (*ports.RQChange, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	notes = strings.TrimSpace(notes)
	if !validUUID(id) || (reason != "RESIGNATION" && reason != "MORE_VACANCIES") || utf8.RuneCountInString(notes) > 500 ||
		(statusID != "" && !validUUID(statusID)) || len(candidates) > 100 || (reason == "RESIGNATION") != (len(candidates) > 0) {
		return nil, domain.ErrValidation
	}
	for _, c := range candidates {
		if !validUUID(c) {
			return nil, domain.ErrValidation
		}
	}
	var st *string
	if statusID != "" {
		st = &statusID
	}
	c, err := s.Repo.Reopen(ctx, meta(p, ip), tid, id, reason, notes, candidates, st)
	if err == nil && c != nil && s.Notifier != nil {
		s.Notifier.RQChanged(ctx, p, c)
	}
	return c, err
}

func (s *RequirementService) Delete(ctx context.Context, p *domain.Principal, ip, id string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return s.Repo.Delete(ctx, meta(p, ip), tid, id)
}

func (s *RequirementService) AddCandidate(ctx context.Context, p *domain.Principal, ip, rqID, id, talentID, vacancyID string) (bool, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return false, err
	}
	if !validUUID(rqID) || !validUUID(id) || !validUUID(talentID) || !validUUID(vacancyID) {
		return false, domain.ErrValidation
	}
	return s.Repo.CandidateAdd(ctx, meta(p, ip), tid, rqID, id, talentID, vacancyID)
}

func (s *RequirementService) MoveCandidate(ctx context.Context, p *domain.Principal, ip, id, stageID string, vacancyID *string, order *int) (string, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return "", err
	}
	vacancyID = nilIfEmpty(vacancyID)
	if !validUUID(id) || !validUUID(stageID) || !optRef(vacancyID) || (order != nil && (*order < 0 || *order > 1_000_000)) {
		return "", domain.ErrValidation
	}
	return s.Repo.CandidateMove(ctx, meta(p, ip), tid, id, stageID, vacancyID, order)
}

func (s *RequirementService) RemoveCandidate(ctx context.Context, p *domain.Principal, ip, id string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return s.Repo.CandidateRemove(ctx, meta(p, ip), tid, id)
}

func (s *RequirementService) ByTalent(ctx context.Context, p *domain.Principal, talentID string) ([]json.RawMessage, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(talentID) {
		return nil, domain.ErrNotFound
	}
	return s.Repo.ByTalent(ctx, meta(p, ""), tid, talentID)
}

func (s *RequirementService) Contacts(ctx context.Context, p *domain.Principal, clientID string) ([]json.RawMessage, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(clientID) {
		return nil, domain.ErrValidation
	}
	return s.Repo.Contacts(ctx, meta(p, ""), tid, clientID)
}

func (s *RequirementService) SaveContact(ctx context.Context, p *domain.Principal, ip string, c ports.ContactWrite) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	for _, f := range []*string{&c.FirstName, &c.LastName, &c.SecondLastName, &c.Position, &c.Phone, &c.Phone2, &c.Email, &c.Email2} {
		*f = strings.TrimSpace(*f)
	}
	if !validUUID(c.ID) || !validUUID(c.ClientID) || c.FirstName == "" || c.LastName == "" ||
		utf8.RuneCountInString(c.FirstName) > 100 || utf8.RuneCountInString(c.LastName) > 100 || utf8.RuneCountInString(c.SecondLastName) > 100 ||
		utf8.RuneCountInString(c.Position) > 150 || len(c.Phone) > 30 || len(c.Phone2) > 30 ||
		(c.Email != "" && !ValidEmail(c.Email)) || (c.Email2 != "" && !ValidEmail(c.Email2)) {
		return domain.ErrValidation
	}
	return s.Repo.ContactSave(ctx, meta(p, ip), tid, c)
}

func (s *RequirementService) DeleteContact(ctx context.Context, p *domain.Principal, ip, id string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return s.Repo.ContactDelete(ctx, meta(p, ip), tid, id)
}
