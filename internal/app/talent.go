package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type TalentService struct {
	Repo    ports.TalentRepository
	Cipher  ports.Cipher
	Storage ports.FileStorage
	// Translator traduce el CV (OpenAI); sin key responde AI_NOT_CONFIGURED
	Translator Translator
	Usage      ports.UsageRepository
}

func tenantOf(p *domain.Principal) (uint64, error) {
	if p == nil || p.TenantID == nil {
		return 0, domain.ErrForbidden
	}
	return *p.TenantID, nil
}

func validUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// idOrNew: el cliente puede enviar su propio uuid (alta offline); si no, se genera.
func idOrNew(s string) (string, error) {
	if s == "" {
		return uuid.NewString(), nil
	}
	if !validUUID(s) {
		return "", domain.ErrValidation
	}
	return strings.ToLower(s), nil
}

// validBirthDate: YYYY-MM-DD entre 1900 y hace 14 años.
func validBirthDate(s string) bool {
	d, err := time.Parse("2006-01-02", s)
	return err == nil && d.Year() >= 1900 && d.Before(time.Now().AddDate(-14, 0, 0))
}

func maxLen(s *string, n int) bool { return s == nil || utf8.RuneCountInString(*s) <= n }

func (s *TalentService) Search(ctx context.Context, p *domain.Principal, f ports.TalentFilter) (*domain.Page[domain.TalentSummary], error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if f.Skills != "" {
		for _, id := range strings.Split(f.Skills, ",") {
			if !validUUID(id) {
				return nil, domain.ErrValidation
			}
		}
	}
	if (f.GradeID != "" && !validUUID(f.GradeID)) || (f.CollectionID != "" && !validUUID(f.CollectionID)) {
		return nil, domain.ErrValidation
	}
	if len(f.Search) > 150 || len(f.Position) > 150 || len(f.Education) > 150 {
		return nil, domain.ErrValidation
	}
	return s.Repo.Search(ctx, meta(p, ""), tid, f)
}

func (s *TalentService) Get(ctx context.Context, p *domain.Principal, id string) (*domain.Talent, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(id) {
		return nil, domain.NewError("TALENT_NOT_FOUND")
	}
	rec, err := s.Repo.Get(ctx, meta(p, ""), tid, id)
	if err != nil {
		return nil, err
	}
	t := rec.Talent
	if t.DocNumber, err = s.Cipher.Decrypt(rec.DocEnc); err != nil {
		return nil, err
	}
	if t.Email, err = s.Cipher.Decrypt(rec.EmailEnc); err != nil {
		return nil, err
	}
	if t.Phone, err = s.Cipher.Decrypt(rec.PhoneEnc); err != nil {
		return nil, err
	}
	if t.BirthDate, err = s.Cipher.Decrypt(rec.BirthEnc); err != nil {
		return nil, err
	}
	if len(rec.RatesEnc) > 0 {
		raw, err := s.Cipher.DecryptBytes(rec.RatesEnc)
		if err != nil {
			return nil, err
		}
		var r domain.Rates
		if json.Unmarshal(raw, &r) == nil {
			t.Rates = &r
		}
	}
	for i := range t.Feedback {
		if i < len(rec.FeedbackAuthorEnc) {
			t.Feedback[i].AuthorName, _ = s.Cipher.Decrypt(rec.FeedbackAuthorEnc[i])
		}
	}
	return &t, nil
}

// TalentInput es lo que envía la UI (texto plano); aquí se valida y cifra.
type TalentInput struct {
	ID              string        `json:"id"`
	FirstName       string        `json:"firstName"`
	LastName        string        `json:"lastName"`
	SecondLastName  *string       `json:"secondLastName"`
	Headline        *string       `json:"headline"`
	DocTypeID       *string       `json:"docTypeId"`
	DocNumber       string        `json:"docNumber"`
	Email           string        `json:"email"`
	Phone           string        `json:"phone"`
	BirthDate       string        `json:"birthDate"`
	CountryID       *string       `json:"countryId"`
	City            *string       `json:"city"`
	OriginCountryID *string       `json:"originCountryId"`
	OriginCity      *string       `json:"originCity"`
	LinkedinURL     *string       `json:"linkedinUrl"`
	GithubURL       *string       `json:"githubUrl"`
	Summary         *string       `json:"summary"`
	AvailabilityID  *string       `json:"availabilityId"`
	HasEquipment    *bool         `json:"hasEquipment"`
	Rates           *domain.Rates `json:"rates"`
	ExpectedVersion *int          `json:"expectedVersion"`
}

func (s *TalentService) Save(ctx context.Context, p *domain.Principal, ip string, in TalentInput) (*ports.TalentWriteResult, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	id, err := idOrNew(in.ID)
	if err != nil {
		return nil, err
	}
	in.FirstName, in.LastName = strings.TrimSpace(in.FirstName), strings.TrimSpace(in.LastName)
	if in.FirstName == "" || in.LastName == "" || utf8.RuneCountInString(in.FirstName) > 100 || utf8.RuneCountInString(in.LastName) > 100 ||
		!maxLen(in.SecondLastName, 100) || !maxLen(in.Headline, 150) || !maxLen(in.City, 120) || !maxLen(in.OriginCity, 120) ||
		!maxLen(in.Summary, 5000) || !maxLen(in.LinkedinURL, 300) || !maxLen(in.GithubURL, 300) ||
		len(in.DocNumber) > 30 || len(in.Phone) > 30 {
		return nil, domain.ErrValidation
	}
	for _, ref := range []*string{in.DocTypeID, in.CountryID, in.OriginCountryID, in.AvailabilityID} {
		if ref != nil && *ref != "" && !validUUID(*ref) {
			return nil, domain.ErrValidation
		}
	}
	w := ports.TalentWrite{
		UUID: id, FirstName: in.FirstName, LastName: in.LastName, SecondLastName: in.SecondLastName, Headline: in.Headline,
		DocTypeID: in.DocTypeID, CountryID: in.CountryID, City: in.City, OriginCountryID: in.OriginCountryID, OriginCity: in.OriginCity, Linkedin: in.LinkedinURL, Github: in.GithubURL,
		Summary: in.Summary, AvailabilityID: in.AvailabilityID, HasEquipment: in.HasEquipment, ExpectedVersion: in.ExpectedVersion,
		Source: "MANUAL",
	}
	if email := strings.ToLower(strings.TrimSpace(in.Email)); email != "" {
		if !ValidEmail(email) {
			return nil, domain.NewError("INVALID_EMAIL")
		}
		if w.EmailEnc, err = s.Cipher.Encrypt(email); err != nil {
			return nil, err
		}
		b := s.Cipher.BlindIndex(email)
		w.EmailBidx = &b
	}
	if doc := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(in.DocNumber), " ", "")); doc != "" {
		if w.DocEnc, err = s.Cipher.Encrypt(doc); err != nil {
			return nil, err
		}
		b := s.Cipher.BlindIndex(doc)
		w.DocBidx = &b
	}
	if phone := strings.TrimSpace(in.Phone); phone != "" {
		if w.PhoneEnc, err = s.Cipher.Encrypt(phone); err != nil {
			return nil, err
		}
	}
	if bd := strings.TrimSpace(in.BirthDate); bd != "" {
		if !validBirthDate(bd) {
			return nil, domain.NewError("INVALID_BIRTH_DATE")
		}
		if w.BirthEnc, err = s.Cipher.Encrypt(bd); err != nil {
			return nil, err
		}
	}
	// Tarifas: solo quien tiene talent.rates.read puede enviarlas (el SP lo vuelve a verificar)
	if in.Rates != nil {
		if !p.Has("talent.rates.read") {
			return nil, domain.ErrForbidden
		}
		raw, _ := json.Marshal(in.Rates)
		if w.RatesEnc, err = s.Cipher.EncryptBytes(raw); err != nil {
			return nil, err
		}
		w.SetRates = true
	}
	return s.Repo.Upsert(ctx, meta(p, ip), tid, w)
}

func (s *TalentService) SetStatus(ctx context.Context, p *domain.Principal, ip, id, status string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	return s.Repo.SetStatus(ctx, meta(p, ip), tid, id, status)
}

// SetAvailability y SetEquipment: edición rápida desde la ficha (nil = sin dato)
func (s *TalentService) SetAvailability(ctx context.Context, p *domain.Principal, ip, id string, availabilityID *string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(id) || (availabilityID != nil && *availabilityID != "" && !validUUID(*availabilityID)) {
		return domain.ErrValidation
	}
	return s.Repo.SetAvailability(ctx, meta(p, ip), tid, id, availabilityID)
}

func (s *TalentService) SetEquipment(ctx context.Context, p *domain.Principal, ip, id string, hasEquipment *bool) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(id) {
		return domain.ErrValidation
	}
	return s.Repo.SetEquipment(ctx, meta(p, ip), tid, id, hasEquipment)
}

// ---------------------------------------------------------------- Secciones

type SkillInput struct {
	ID      string  `json:"id"`
	SkillID string  `json:"skillId"`
	Label   string  `json:"label"` // si no hay skillId, se busca/crea por nombre
	Years   float64 `json:"years"`
}

func (s *TalentService) resolveSkill(ctx context.Context, p *domain.Principal, tid uint64, table string, in SkillInput) (string, error) {
	if in.SkillID != "" {
		if !validUUID(in.SkillID) {
			return "", domain.ErrValidation
		}
		return in.SkillID, nil
	}
	if strings.TrimSpace(in.Label) == "" {
		return "", domain.ErrValidation
	}
	ref, err := s.Repo.EnsureSkill(ctx, meta(p, ""), tid, table, uuid.NewString(), in.Label)
	if err != nil {
		return "", err
	}
	return ref.ID, nil
}

func (s *TalentService) SaveTechSkill(ctx context.Context, p *domain.Principal, talentID string, in SkillInput) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	id, err := idOrNew(in.ID)
	if err != nil {
		return err
	}
	skill, err := s.resolveSkill(ctx, p, tid, "TECH_SKILL", in)
	if err != nil {
		return err
	}
	return s.Repo.UpsertTechSkill(ctx, meta(p, ""), tid, talentID, id, skill, in.Years)
}

func (s *TalentService) AddSoftSkill(ctx context.Context, p *domain.Principal, talentID string, in SkillInput) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	id, err := idOrNew(in.ID)
	if err != nil {
		return err
	}
	skill, err := s.resolveSkill(ctx, p, tid, "SOFT_SKILL", in)
	if err != nil {
		return err
	}
	return s.Repo.AddSoftSkill(ctx, meta(p, ""), tid, talentID, id, skill)
}

var sections = map[string]string{"tech-skills": "TECH_SKILL", "soft-skills": "SOFT_SKILL", "experiences": "EXPERIENCE",
	"educations": "EDUCATION", "languages": "LANGUAGE"}

func (s *TalentService) DeleteSection(ctx context.Context, p *domain.Principal, ip, section, id string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	code, ok := sections[section]
	if !ok || !validUUID(id) {
		return domain.ErrNotFound
	}
	return s.Repo.DeleteSection(ctx, meta(p, ip), tid, code, id)
}

func validDate(s *string) bool {
	if s == nil || *s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", *s)
	return err == nil
}

func (s *TalentService) SaveExperience(ctx context.Context, p *domain.Principal, talentID string, e domain.TalentExperience) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if e.ID, err = idOrNew(e.ID); err != nil {
		return err
	}
	if !validDate(&e.StartDate) || e.StartDate == "" || !validDate(e.EndDate) || !maxLen(e.Functions, 5000) ||
		utf8.RuneCountInString(e.Company) > 150 || utf8.RuneCountInString(e.Position) > 150 {
		return domain.ErrValidation
	}
	return s.Repo.UpsertExperience(ctx, meta(p, ""), tid, talentID, e)
}

func (s *TalentService) SaveEducation(ctx context.Context, p *domain.Principal, talentID string, e domain.TalentEducation) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if e.ID, err = idOrNew(e.ID); err != nil {
		return err
	}
	if !validDate(e.StartDate) || !validDate(e.EndDate) || (e.GradeID != nil && *e.GradeID != "" && !validUUID(*e.GradeID)) ||
		utf8.RuneCountInString(e.Institution) > 150 || utf8.RuneCountInString(e.Career) > 150 {
		return domain.ErrValidation
	}
	return s.Repo.UpsertEducation(ctx, meta(p, ""), tid, talentID, e)
}

func (s *TalentService) SaveLanguage(ctx context.Context, p *domain.Principal, talentID string, l domain.TalentLanguage) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if l.ID, err = idOrNew(l.ID); err != nil {
		return err
	}
	if !validUUID(l.LanguageID) || !validUUID(l.LevelID) {
		return domain.ErrValidation
	}
	return s.Repo.UpsertLanguage(ctx, meta(p, ""), tid, talentID, l.ID, l.LanguageID, l.LevelID, l.Stars)
}

func (s *TalentService) SaveFeedback(ctx context.Context, p *domain.Principal, talentID, id string, stars int, comment string) (*float64, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if id, err = idOrNew(id); err != nil {
		return nil, err
	}
	return s.Repo.UpsertFeedback(ctx, meta(p, ""), tid, talentID, id, stars, comment)
}

func (s *TalentService) DeleteFeedback(ctx context.Context, p *domain.Principal, ip, id string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return s.Repo.DeleteFeedback(ctx, meta(p, ip), tid, id)
}

// ---------------------------------------------------------------- Archivos

const maxTalentFile = 10 << 20 // 10 MB

var docMimes = map[string]string{"application/pdf": "pdf", "application/msword": "doc",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "docx"}
var imgMimes = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}

// sniff detecta el tipo real por contenido (no se confía en la extensión ni en el navegador).
func sniff(head []byte, name string) string {
	m := http.DetectContentType(head)
	switch {
	case bytes.HasPrefix(head, []byte{0xD0, 0xCF, 0x11, 0xE0}) && strings.HasSuffix(strings.ToLower(name), ".doc"):
		return "application/msword"
	case m == "application/zip" && strings.HasSuffix(strings.ToLower(name), ".docx") && bytes.Contains(head, []byte("word/")) ||
		m == "application/zip" && strings.HasSuffix(strings.ToLower(name), ".docx") && bytes.Contains(head, []byte("[Content_Types].xml")):
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	}
	return m
}

// UploadFile guarda el archivo CIFRADO (AES-256-GCM) en el volumen persistente.
func (s *TalentService) UploadFile(ctx context.Context, p *domain.Principal, ip, talentID, category, name string, r io.Reader) (*domain.TalentFile, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(talentID) {
		return nil, domain.NewError("TALENT_NOT_FOUND")
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
	allowed := docMimes
	if category == "PHOTO" {
		allowed = imgMimes
	} else if category == "CERTIFICATE" || category == "OTHER" {
		allowed = map[string]string{}
		for k, v := range docMimes {
			allowed[k] = v
		}
		for k, v := range imgMimes {
			allowed[k] = v
		}
	}
	ext, ok := allowed[mime]
	if !ok {
		return nil, domain.NewError("FILE_TYPE_NOT_ALLOWED")
	}
	ct, err := s.Cipher.EncryptBytes(data)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	key := path.Join("tenants", p.TenantUUID, "talents", talentID, id+"."+ext+".enc")
	if _, _, err := s.Storage.Put(ctx, key, bytes.NewReader(ct), int64(len(ct))); err != nil {
		return nil, err
	}
	base := path.Base(strings.ReplaceAll(name, "\\", "/"))
	if utf8.RuneCountInString(base) > 200 {
		base = string([]rune(base)[:200])
	}
	sum := sha256Hex(data)
	f, err := s.Repo.CreateFile(ctx, meta(p, ip), tid, talentID, domain.FileObject{UUID: id, Category: category, StorageKey: key,
		OriginalName: base, MimeType: mime, Size: int64(len(data)), SHA256: sum, Encrypted: true})
	if err != nil {
		_ = s.Storage.Delete(ctx, key)
		return nil, err
	}
	return f, nil
}

// OpenFile devuelve el archivo descifrado (solo usuarios del mismo tenant con talent.read).
func (s *TalentService) OpenFile(ctx context.Context, p *domain.Principal, id string) (*domain.FileObject, []byte, error) {
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

func (s *TalentService) DeleteFile(ctx context.Context, p *domain.Principal, ip, id string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return s.Repo.DeleteFile(ctx, meta(p, ip), tid, id)
}

// ---------------------------------------------------------------- Favoritos (compartibles)

func (s *TalentService) collections(recs []ports.CollectionRecord) ([]domain.Collection, error) {
	out := make([]domain.Collection, 0, len(recs))
	for _, r := range recs {
		c := r.Collection
		name, err := s.Cipher.Decrypt(r.OwnerNameEnc)
		if err != nil {
			return nil, err
		}
		c.OwnerName = name
		out = append(out, c)
	}
	return out, nil
}

func (s *TalentService) people(recs []ports.PersonRecord) ([]domain.CollectionShare, error) {
	out := make([]domain.CollectionShare, 0, len(recs))
	for _, r := range recs {
		name, err := s.Cipher.Decrypt(r.NameEnc)
		if err != nil {
			return nil, err
		}
		email, err := s.Cipher.Decrypt(r.EmailEnc)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.CollectionShare{UserID: r.ID, Name: name, Email: email, Permission: r.Permission, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

func (s *TalentService) Collections(ctx context.Context, p *domain.Principal) ([]domain.Collection, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	recs, err := s.Repo.Collections(ctx, meta(p, ""), tid)
	if err != nil {
		return nil, err
	}
	return s.collections(recs)
}

func (s *TalentService) CreateCollection(ctx context.Context, p *domain.Principal, id, name string) ([]domain.Collection, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if id, err = idOrNew(id); err != nil {
		return nil, err
	}
	recs, err := s.Repo.CreateCollection(ctx, meta(p, ""), tid, id, name)
	if err != nil {
		return nil, err
	}
	return s.collections(recs)
}

func (s *TalentService) RenameCollection(ctx context.Context, p *domain.Principal, id, name string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return s.Repo.RenameCollection(ctx, meta(p, ""), tid, id, name)
}

func (s *TalentService) DeleteCollection(ctx context.Context, p *domain.Principal, id string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(id) {
		return domain.ErrNotFound
	}
	return s.Repo.DeleteCollection(ctx, meta(p, ""), tid, id)
}

func (s *TalentService) SetCollectionTalent(ctx context.Context, p *domain.Principal, collectionID, talentID string, in bool) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(collectionID) || !validUUID(talentID) {
		return domain.ErrNotFound
	}
	return s.Repo.SetCollectionTalent(ctx, meta(p, ""), tid, collectionID, talentID, in)
}

func (s *TalentService) CollectionShares(ctx context.Context, p *domain.Principal, id string) ([]domain.CollectionShare, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(id) {
		return nil, domain.ErrNotFound
	}
	recs, err := s.Repo.CollectionShares(ctx, meta(p, ""), tid, id)
	if err != nil {
		return nil, err
	}
	return s.people(recs)
}

func (s *TalentService) ShareCollection(ctx context.Context, p *domain.Principal, ip, id, userID, permission string) ([]domain.CollectionShare, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(id) || !validUUID(userID) {
		return nil, domain.ErrValidation
	}
	recs, err := s.Repo.ShareCollection(ctx, meta(p, ip), tid, id, userID, strings.ToUpper(permission))
	if err != nil {
		return nil, err
	}
	return s.people(recs)
}

func (s *TalentService) UnshareCollection(ctx context.Context, p *domain.Principal, ip, id, userID string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	if !validUUID(id) || !validUUID(userID) {
		return domain.ErrNotFound
	}
	return s.Repo.UnshareCollection(ctx, meta(p, ip), tid, id, userID)
}

// Colleagues busca usuarios del tenant para compartir (nombre/correo cifrados: se filtra tras descifrar).
// Nunca devuelve el directorio completo: exige al menos 2 caracteres y limita a ColleagueLimit.
func (s *TalentService) Colleagues(ctx context.Context, p *domain.Principal, q string) ([]domain.Colleague, error) {
	needle := foldText(q)
	if utf8.RuneCountInString(needle) < 2 {
		return []domain.Colleague{}, nil
	}
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	recs, err := s.Repo.Colleagues(ctx, meta(p, ""), tid)
	if err != nil {
		return nil, err
	}
	people, err := s.people(recs)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Colleague, 0, ColleagueLimit)
	for _, x := range people {
		if !strings.Contains(foldText(x.Name), needle) && !strings.Contains(foldText(x.Email), needle) {
			continue
		}
		out = append(out, domain.Colleague{ID: x.UserID, Name: x.Name, Email: x.Email})
		if len(out) == ColleagueLimit {
			break
		}
	}
	return out, nil
}

// ColleagueLimit es el máximo de resultados de la búsqueda de usuarios.
const ColleagueLimit = 10

var accentFold = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n", "à", "a", "è", "e", "ì", "i", "ò", "o", "ù", "u")

// foldText normaliza para comparar: minúsculas, sin tildes ni espacios extremos.
func foldText(v string) string { return accentFold.Replace(strings.ToLower(strings.TrimSpace(v))) }
