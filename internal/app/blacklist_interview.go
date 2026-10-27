package app

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// ---------------------------------------------------------------- Lista negra

type BlacklistService struct {
	Repo   ports.BlacklistRepository
	Cipher ports.Cipher
}

func (s *BlacklistService) entries(recs []ports.BlacklistEntryRecord, err error) ([]domain.BlacklistEntry, error) {
	if err != nil {
		return nil, err
	}
	out := make([]domain.BlacklistEntry, 0, len(recs))
	for _, r := range recs {
		e := r.BlacklistEntry
		e.UserName, _ = s.Cipher.Decrypt(r.UserNameEnc)
		out = append(out, e)
	}
	return out, nil
}

func optUUID(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if !validUUID(s) {
		return "", domain.ErrValidation
	}
	return s, nil
}

func (s *BlacklistService) Search(ctx context.Context, p *domain.Principal, search, clientID string, globalOnly bool, page, size int) (*domain.Page[domain.BlacklistTalent], error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if clientID, err = optUUID(clientID); err != nil || len(search) > 150 {
		return nil, domain.ErrValidation
	}
	return s.Repo.Search(ctx, meta(p, ""), tid, search, clientID, globalOnly, page, size)
}

func (s *BlacklistService) Status(ctx context.Context, p *domain.Principal, talentID string) ([]domain.BlacklistEntry, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(talentID) {
		return nil, domain.NewError("TALENT_NOT_FOUND")
	}
	return s.entries(s.Repo.Status(ctx, meta(p, ""), tid, talentID))
}

func (s *BlacklistService) History(ctx context.Context, p *domain.Principal, talentID string, page, size int) (*domain.Page[domain.BlacklistMovement], error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(talentID) {
		return nil, domain.NewError("TALENT_NOT_FOUND")
	}
	total, recs, err := s.Repo.History(ctx, meta(p, ""), tid, talentID, page, size)
	if err != nil {
		return nil, err
	}
	out := &domain.Page[domain.BlacklistMovement]{Total: total, Items: make([]domain.BlacklistMovement, 0, len(recs))}
	for _, r := range recs {
		h := r.BlacklistMovement
		h.UserName, _ = s.Cipher.Decrypt(r.UserNameEnc)
		out.Items = append(out.Items, h)
	}
	return out, nil
}

func (s *BlacklistService) Create(ctx context.Context, p *domain.Principal, ip, id, talentID, clientID, reason string) ([]domain.BlacklistEntry, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if id, err = idOrNew(id); err != nil {
		return nil, err
	}
	if clientID, err = optUUID(clientID); err != nil || !validUUID(talentID) {
		return nil, domain.ErrValidation
	}
	return s.entries(s.Repo.Create(ctx, meta(p, ip), tid, id, talentID, clientID, reason))
}

func (s *BlacklistService) Update(ctx context.Context, p *domain.Principal, ip, id, reason string) ([]domain.BlacklistEntry, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(id) {
		return nil, domain.ErrNotFound
	}
	return s.entries(s.Repo.Update(ctx, meta(p, ip), tid, id, reason))
}

func (s *BlacklistService) Remove(ctx context.Context, p *domain.Principal, ip, id, reason string) ([]domain.BlacklistEntry, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(id) {
		return nil, domain.ErrNotFound
	}
	return s.entries(s.Repo.Remove(ctx, meta(p, ip), tid, id, reason))
}

func (s *BlacklistService) LiftGlobal(ctx context.Context, p *domain.Principal, ip, id, reason string, keep []ports.BlacklistKeep) ([]domain.BlacklistEntry, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(id) || len(keep) > 500 {
		return nil, domain.ErrValidation
	}
	for _, k := range keep {
		if !validUUID(k.Client) {
			return nil, domain.NewError("BLACKLIST_KEEP_INVALID")
		}
	}
	return s.entries(s.Repo.LiftGlobal(ctx, meta(p, ip), tid, id, reason, keep))
}

// ---------------------------------------------------------------- Entrevistas

type InterviewService struct {
	Repo     ports.InterviewRepository
	Cipher   ports.Cipher
	Storage  ports.FileStorage
	Timezone *time.Location
}

type InterviewInput struct {
	ID                   string               `json:"id"`
	TalentID             string               `json:"talentId"`
	TypeID               string               `json:"typeId"`
	StageID              string               `json:"stageId"`
	StatusID             string               `json:"statusId"`
	DurationID           *string              `json:"durationId"`
	ClientID             *string              `json:"clientId"`
	RequirementRef       *string              `json:"requirementRef"`
	Profile              string               `json:"profile"`
	Date                 string               `json:"date"`
	Time                 string               `json:"time"`
	MeetingLink          *string              `json:"meetingLink"`
	LocationURL          *string              `json:"locationUrl"`
	Address              *string              `json:"address"`
	RatingGeneral        int                  `json:"ratingGeneral"`
	RatingPersonal       int                  `json:"ratingPersonal"`
	RatingExperience     int                  `json:"ratingExperience"`
	RatingLanguages      int                  `json:"ratingLanguages"`
	RatingEducation      int                  `json:"ratingEducation"`
	NotesPersonal        *string              `json:"notesPersonal"`
	NotesExperience      *string              `json:"notesExperience"`
	NotesLanguages       *string              `json:"notesLanguages"`
	NotesEducation       *string              `json:"notesEducation"`
	CancelReason         *string              `json:"cancelReason"`
	Interviewers         []domain.Interviewer `json:"interviewers"`
	Recordings           []domain.Recording   `json:"recordings"`
	Answers              []ports.AnswerWrite  `json:"answers"`
	AcknowledgeBlacklist bool                 `json:"acknowledgeBlacklist"`
	ExpectedVersion      *int                 `json:"expectedVersion"`
}

var timeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func (s *InterviewService) Search(ctx context.Context, p *domain.Principal, f ports.InterviewFilter) (*domain.Page[domain.InterviewSummary], error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	for _, id := range []string{f.StatusID, f.StageID, f.TypeID, f.ClientID, f.TalentID} {
		if _, err := optUUID(id); err != nil {
			return nil, err
		}
	}
	if f.Date != "" && !validDate(&f.Date) || len(f.Search) > 150 {
		return nil, domain.ErrValidation
	}
	return s.Repo.Search(ctx, meta(p, ""), tid, f)
}

func (s *InterviewService) Get(ctx context.Context, p *domain.Principal, id string) (*domain.Interview, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(id) {
		return nil, domain.NewError("INTERVIEW_NOT_FOUND")
	}
	rec, err := s.Repo.Get(ctx, meta(p, ""), tid, id)
	if err != nil {
		return nil, err
	}
	iv := rec.Interview
	for i := range iv.Interviewers {
		if i < len(rec.InterviewerEmailEnc) && len(rec.InterviewerEmailEnc[i]) > 0 {
			iv.Interviewers[i].Email, _ = s.Cipher.Decrypt(rec.InterviewerEmailEnc[i])
		}
	}
	return &iv, nil
}

func (s *InterviewService) Save(ctx context.Context, p *domain.Principal, ip string, in InterviewInput) (*ports.InterviewWriteResult, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	id, err := idOrNew(in.ID)
	if err != nil {
		return nil, err
	}
	for _, ref := range []string{in.TalentID, in.TypeID, in.StageID, in.StatusID} {
		if !validUUID(ref) {
			return nil, domain.ErrValidation
		}
	}
	for _, ref := range []*string{in.DurationID, in.ClientID} {
		if ref != nil && *ref != "" && !validUUID(*ref) {
			return nil, domain.ErrValidation
		}
	}
	if !validDate(&in.Date) || in.Date == "" || !timeRe.MatchString(in.Time) || utf8.RuneCountInString(in.Profile) > 200 ||
		!maxLen(in.RequirementRef, 100) || !maxLen(in.Address, 500) || !maxLen(in.MeetingLink, 500) || !maxLen(in.LocationURL, 500) ||
		!maxLen(in.NotesPersonal, 5000) || !maxLen(in.NotesExperience, 5000) || !maxLen(in.NotesLanguages, 5000) ||
		!maxLen(in.NotesEducation, 5000) || !maxLen(in.CancelReason, 2000) || len(in.Interviewers) > 30 || len(in.Recordings) > 30 {
		return nil, domain.ErrValidation
	}
	ivs := make([]ports.InterviewerWrite, 0, len(in.Interviewers))
	for _, x := range in.Interviewers {
		w := ports.InterviewerWrite{Name: strings.TrimSpace(x.Name)}
		if utf8.RuneCountInString(w.Name) > 150 {
			return nil, domain.ErrValidation
		}
		if email := strings.ToLower(strings.TrimSpace(x.Email)); email != "" {
			if !ValidEmail(email) {
				return nil, domain.NewError("INVALID_EMAIL")
			}
			enc, err := s.Cipher.Encrypt(email)
			if err != nil {
				return nil, err
			}
			w.EmailEnc = hex.EncodeToString(enc)
		}
		if x.Notify {
			w.Notify = 1
		}
		ivs = append(ivs, w)
	}
	res, err := s.Repo.Save(ctx, meta(p, ip), tid, ports.InterviewWrite{
		UUID: id, TalentID: in.TalentID, TypeID: in.TypeID, StageID: in.StageID, StatusID: in.StatusID,
		DurationID: in.DurationID, ClientID: in.ClientID, RequirementRef: in.RequirementRef, Profile: in.Profile,
		Date: in.Date, Time: in.Time + ":00", MeetingLink: in.MeetingLink, LocationURL: in.LocationURL, Address: in.Address,
		RGeneral: in.RatingGeneral, RPersonal: in.RatingPersonal, RExperience: in.RatingExperience, RLanguages: in.RatingLanguages,
		REducation: in.RatingEducation, NPersonal: in.NotesPersonal, NExperience: in.NotesExperience, NLanguages: in.NotesLanguages,
		NEducation: in.NotesEducation, CancelReason: in.CancelReason, Interviewers: ivs, Recordings: in.Recordings,
		AckBlacklist: in.AcknowledgeBlacklist, ExpectedVersion: in.ExpectedVersion,
	})
	if err != nil {
		return nil, err
	}
	// Cuestionario: solo en entrevistas telefónicas (las respuestas vacías se eliminan)
	if res.TypeCode == "PHONE" && in.Answers != nil {
		for _, a := range in.Answers {
			if !validUUID(a.Question) || utf8.RuneCountInString(a.Answer) > 5000 {
				return nil, domain.ErrValidation
			}
		}
		if err := s.Repo.SaveAnswers(ctx, meta(p, ip), tid, res.ID, in.Answers); err != nil {
			return nil, err
		}
	}
	return res, nil
}

var interviewFileMimes = map[string]string{"application/pdf": "pdf", "application/msword": "doc",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "docx",
	"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}

func (s *InterviewService) UploadFile(ctx context.Context, p *domain.Principal, ip, interviewID, typeID, name string, r io.Reader) (*domain.TalentFile, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(interviewID) || !validUUID(typeID) {
		return nil, domain.ErrValidation
	}
	data, err := io.ReadAll(io.LimitReader(r, maxTalentFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxTalentFile {
		return nil, domain.NewError("FILE_TOO_LARGE")
	}
	if len(data) == 0 {
		return nil, domain.ErrValidation
	}
	head := data
	if len(head) > 1024 {
		head = head[:1024]
	}
	mime := sniff(head, name)
	ext, ok := interviewFileMimes[mime]
	if !ok {
		return nil, domain.NewError("FILE_TYPE_NOT_ALLOWED")
	}
	ct, err := s.Cipher.EncryptBytes(data)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	key := path.Join("tenants", p.TenantUUID, "interviews", interviewID, id+"."+ext+".enc")
	if _, _, err := s.Storage.Put(ctx, key, bytes.NewReader(ct), int64(len(ct))); err != nil {
		return nil, err
	}
	base := path.Base(strings.ReplaceAll(name, "\\", "/"))
	if utf8.RuneCountInString(base) > 200 {
		base = string([]rune(base)[:200])
	}
	f, err := s.Repo.CreateFile(ctx, meta(p, ip), tid, interviewID, typeID, domain.FileObject{UUID: id, StorageKey: key,
		OriginalName: base, MimeType: mime, Size: int64(len(data)), SHA256: sha256Hex(data), Encrypted: true})
	if err != nil {
		_ = s.Storage.Delete(ctx, key)
		return nil, err
	}
	return f, nil
}

func (s *InterviewService) OpenFile(ctx context.Context, p *domain.Principal, id string) (*domain.FileObject, []byte, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, nil, err
	}
	if !validUUID(id) {
		return nil, nil, domain.ErrNotFound
	}
	f, err := s.Repo.GetFile(ctx, meta(p, ""), tid, id)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.Storage.Open(ctx, f.StorageKey)
	if err != nil {
		return nil, nil, domain.ErrNotFound
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, nil, err
	}
	if f.Encrypted {
		if raw, err = s.Cipher.DecryptBytes(raw); err != nil {
			return nil, nil, err
		}
	}
	return f, raw, nil
}

func (s *InterviewService) DeleteFile(ctx context.Context, p *domain.Principal, ip, id string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return s.Repo.DeleteFile(ctx, meta(p, ip), tid, id)
}

// ICS genera la invitación de calendario (RFC 5545) de la entrevista.
func (s *InterviewService) ICS(ctx context.Context, p *domain.Principal, id, locale string) (string, error) {
	iv, err := s.Get(ctx, p, id)
	if err != nil {
		return "", err
	}
	loc := s.Timezone
	if loc == nil {
		loc = time.UTC
	}
	start, err := time.ParseInLocation("2006-01-02 15:04", iv.Date+" "+iv.Time, loc)
	if err != nil {
		return "", domain.ErrValidation
	}
	minutes := 60
	if iv.DurationMinutes != nil && *iv.DurationMinutes > 0 {
		minutes = *iv.DurationMinutes
	}
	label := func(raw []byte) string { return labelOf(raw, locale) }
	name := strings.TrimSpace(strings.Join([]string{iv.FirstName, iv.LastName, deref(iv.SecondLastName)}, " "))
	desc := []string{"Etapa: " + label(iv.Stage), "Tipo: " + label(iv.Type), "Perfil: " + iv.Profile}
	if len(iv.Client) > 0 {
		desc = append(desc, "Cliente: "+label(iv.Client))
	}
	if iv.RequirementRef != nil {
		desc = append(desc, "Requerimiento: "+*iv.RequirementRef)
	}
	if len(iv.Interviewers) > 0 {
		names := make([]string, 0, len(iv.Interviewers))
		for _, x := range iv.Interviewers {
			names = append(names, x.Name)
		}
		desc = append(desc, "Entrevistadores: "+strings.Join(names, ", "))
	}
	var b strings.Builder
	w := func(line string) { b.WriteString(foldICS(line)); b.WriteString("\r\n") }
	w("BEGIN:VCALENDAR")
	w("VERSION:2.0")
	w("PRODID:-//BancoDeTalentos//Entrevistas//ES")
	w("METHOD:PUBLISH")
	w("BEGIN:VEVENT")
	w("UID:interview-" + iv.ID + "@bdt")
	w("DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z"))
	w("DTSTART:" + start.UTC().Format("20060102T150405Z"))
	w("DTEND:" + start.Add(time.Duration(minutes)*time.Minute).UTC().Format("20060102T150405Z"))
	w("SUMMARY:" + icsText(fmt.Sprintf("Entrevista - %s | %s", name, iv.Profile)))
	w("DESCRIPTION:" + icsText(strings.Join(desc, "\n")))
	switch {
	case iv.Address != nil:
		w("LOCATION:" + icsText(*iv.Address))
		if iv.LocationURL != nil {
			w("URL:" + *iv.LocationURL)
		}
	case iv.MeetingLink != nil:
		w("LOCATION:" + icsText(*iv.MeetingLink))
		w("URL:" + *iv.MeetingLink)
	}
	w("END:VEVENT")
	w("END:VCALENDAR")
	return b.String(), nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// icsText escapa texto según RFC 5545.
func icsText(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`)
	return r.Replace(s)
}

// foldICS parte líneas de más de 75 octetos.
func foldICS(line string) string {
	if len(line) <= 75 {
		return line
	}
	var b strings.Builder
	n := 0
	for _, r := range line {
		l := utf8.RuneLen(r)
		if n+l > 75 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += l
	}
	return b.String()
}

// labelOf devuelve la etiqueta i18n en el idioma pedido (o español).
func labelOf(raw []byte, locale string) string {
	var m map[string]string
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil {
		return ""
	}
	if v := m[locale]; v != "" {
		return v
	}
	return m["es"]
}
