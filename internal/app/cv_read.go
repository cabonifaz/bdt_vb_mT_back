package app

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// CVReader lee el texto de un CV y devuelve sus datos estructurados (OpenAI).
type CVReader interface {
	ExtractCV(ctx context.Context, text string) ([]byte, domain.AIUsage, error)
}

// CVRead: datos leídos de un CV para llenar el alta de un talento. Los catálogos van por código
// (país ISO-2, idioma ISO 639-1, nivel y grado); el front los cruza con los maestros del tenant.
type CVRead struct {
	FirstName      string        `json:"firstName"`
	LastName       string        `json:"lastName"`
	SecondLastName string        `json:"secondLastName"`
	DocNumber      string        `json:"docNumber"`
	Email          string        `json:"email"`
	PhoneCode      string        `json:"phoneCode"`
	PhoneNumber    string        `json:"phoneNumber"`
	CountryCode    string        `json:"countryCode"`
	City           string        `json:"city"`
	Linkedin       string        `json:"linkedin"`
	Github         string        `json:"github"`
	Headline       string        `json:"headline"`
	Summary        string        `json:"summary"`
	TechSkills     []CVReadSkill `json:"techSkills"`
	SoftSkills     []string      `json:"softSkills"`
	Experiences    []CVReadExp   `json:"experiences"`
	Educations     []CVReadEdu   `json:"educations"`
	Languages      []CVReadLang  `json:"languages"`
}

type CVReadSkill struct {
	Name  string  `json:"name"`
	Years float64 `json:"years"`
	// UsedIn: experiencias (índices) donde se usó; solo viene del modelo, sirve para calcular los años
	UsedIn []int `json:"usedIn,omitempty"`
	// YearsKnown: los años salen del CV (explícitos o calculados); si no, es el mínimo por defecto (1)
	YearsKnown bool `json:"yearsKnown"`
}

type CVReadExp struct {
	Company   string `json:"company"`
	Position  string `json:"position"`
	Functions string `json:"functions"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	Current   bool   `json:"current"`
}

type CVReadEdu struct {
	Institution string `json:"institution"`
	Career      string `json:"career"`
	Grade       string `json:"grade"`
	StartDate   string `json:"startDate"`
	EndDate     string `json:"endDate"`
	Current     bool   `json:"current"`
	YearOnly    bool   `json:"yearOnly"`
}

type CVReadLang struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Level string `json:"level"`
}

const (
	cvMinChars = 40     // menos texto: PDF escaneado (imagen) o vacío
	cvMaxChars = 60_000 // se recorta lo que exceda (CV muy largos) para acotar tokens
)

var (
	isoDate    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	twoLetters = regexp.MustCompile(`^[A-Z]{2}$`)
	nonDigits  = regexp.MustCompile(`\D`)
	grades     = map[string]bool{"BACHELOR": true, "DEGREE": true, "COURSE": true, "TECHNICAL": true, "GRADUATE": true, "STUDENT": true}
	levels     = map[string]bool{"BASIC": true, "INTERMEDIATE": true, "ADVANCED": true, "NATIVE": true}
)

// ReadCV lee el texto de un CV (extraído del PDF en el navegador) para autocompletar el alta del talento.
func (s *TalentService) ReadCV(ctx context.Context, p *domain.Principal, text string) (*CVRead, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) < cvMinChars {
		return nil, domain.NewError("CV_NO_TEXT")
	}
	if r := []rune(text); len(r) > cvMaxChars {
		text = string(r[:cvMaxChars])
	}
	if s.Reader == nil {
		return nil, domain.NewError("AI_NOT_CONFIGURED")
	}
	raw, usage, err := s.Reader.ExtractCV(ctx, text)
	if code := domain.AsError(err).Code; err == nil || code != "AI_NOT_CONFIGURED" {
		recordUsage(ctx, s.Usage, tid, domain.UsageAICVRead, usage.Model, err == nil, usage)
	}
	if err != nil {
		return nil, err
	}
	var out CVRead
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, domain.NewError("AI_UNAVAILABLE")
	}
	return sanitizeCVRead(out, time.Now()), nil
}

// sanitizeCVRead acota lo que devuelve el modelo a lo que acepta el alta (largos, formatos y catálogos).
func sanitizeCVRead(in CVRead, now time.Time) *CVRead {
	out := CVRead{
		FirstName: clip(in.FirstName, 100), LastName: clip(in.LastName, 100), SecondLastName: clip(in.SecondLastName, 100),
		DocNumber: clip(in.DocNumber, 30), City: clip(in.City, 120), Headline: clip(in.Headline, 150), Summary: clip(in.Summary, 5000),
		PhoneCode: clip(nonDigits.ReplaceAllString(in.PhoneCode, ""), 4), PhoneNumber: clip(nonDigits.ReplaceAllString(in.PhoneNumber, ""), 15),
		CountryCode: code2(in.CountryCode),
		TechSkills:  []CVReadSkill{}, SoftSkills: []string{}, Experiences: []CVReadExp{}, Educations: []CVReadEdu{}, Languages: []CVReadLang{},
	}
	if e := strings.TrimSpace(in.Email); strings.Contains(e, "@") && len(e) <= 254 && !strings.ContainsAny(e, " \t\n") {
		out.Email = e
	}
	out.Linkedin, out.Github = url(in.Linkedin), url(in.Github)
	seen := map[string]bool{}
	for _, sk := range in.TechSkills {
		name := clip(sk.Name, 100)
		if name == "" || seen[strings.ToLower(name)] || len(out.TechSkills) >= 80 {
			continue
		}
		seen[strings.ToLower(name)] = true
		// años: los que dice el CV; si no los dice, el tiempo de las experiencias donde se usó (sin contar dos veces los solapes)
		years := sk.Years
		if years <= 0 || years > 60 {
			years = skillYears(in.Experiences, sk.UsedIn, now)
		}
		out.TechSkills = append(out.TechSkills, CVReadSkill{Name: name, Years: halfYears(years), YearsKnown: years > 0})
	}
	for _, sk := range in.SoftSkills {
		if name := clip(sk, 100); name != "" && !seen["soft:"+strings.ToLower(name)] && len(out.SoftSkills) < 40 {
			seen["soft:"+strings.ToLower(name)] = true
			out.SoftSkills = append(out.SoftSkills, name)
		}
	}
	for _, e := range in.Experiences {
		x := CVReadExp{Company: clip(e.Company, 150), Position: clip(e.Position, 150), Functions: clip(e.Functions, 5000),
			StartDate: date(e.StartDate), EndDate: date(e.EndDate), Current: e.Current}
		if x.Current {
			x.EndDate = ""
		}
		if x.Company != "" && len(out.Experiences) < 40 {
			out.Experiences = append(out.Experiences, x)
		}
	}
	for _, e := range in.Educations {
		x := CVReadEdu{Institution: clip(e.Institution, 150), Career: clip(e.Career, 150), Grade: strings.ToUpper(strings.TrimSpace(e.Grade)),
			StartDate: date(e.StartDate), EndDate: date(e.EndDate), Current: e.Current, YearOnly: e.YearOnly}
		if !grades[x.Grade] {
			x.Grade = ""
		}
		if x.Current {
			x.EndDate = ""
		}
		if x.Institution != "" && len(out.Educations) < 30 {
			out.Educations = append(out.Educations, x)
		}
	}
	for _, l := range in.Languages {
		x := CVReadLang{Code: code2(l.Code), Name: clip(l.Name, 60), Level: strings.ToUpper(strings.TrimSpace(l.Level))}
		if !levels[x.Level] {
			x.Level = ""
		}
		if (x.Code != "" || x.Name != "") && len(out.Languages) < 15 {
			out.Languages = append(out.Languages, x)
		}
	}
	return &out
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		s = strings.TrimSpace(string(r[:n]))
	}
	return s
}

func date(s string) string {
	if s = strings.TrimSpace(s); isoDate.MatchString(s) {
		return s
	}
	return ""
}

func code2(s string) string {
	if s = strings.ToUpper(strings.TrimSpace(s)); twoLetters.MatchString(s) {
		return s
	}
	return ""
}

func url(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 300 || strings.ContainsAny(s, " \t\n") {
		return ""
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		s = "https://" + s
	}
	return s
}

// skillYears suma la duración de las experiencias indicadas uniendo los periodos que se solapan.
// Una experiencia actual cuenta hasta hoy; sin fecha de inicio (o sin fin y no actual) no se puede medir y se omite.
func skillYears(exps []CVReadExp, idx []int, now time.Time) float64 {
	type span struct{ from, to time.Time }
	var spans []span
	for _, i := range idx {
		if i < 0 || i >= len(exps) {
			continue
		}
		e := exps[i]
		from, err := time.Parse("2006-01-02", strings.TrimSpace(e.StartDate))
		if err != nil {
			continue
		}
		to := now
		if !e.Current {
			if to, err = time.Parse("2006-01-02", strings.TrimSpace(e.EndDate)); err != nil {
				continue
			}
		}
		if to.After(now) {
			to = now
		}
		if to.After(from) {
			spans = append(spans, span{from, to})
		}
	}
	sort.Slice(spans, func(a, b int) bool { return spans[a].from.Before(spans[b].from) })
	var total time.Duration
	var cur *span
	for i := range spans {
		if cur != nil && !spans[i].from.After(cur.to) {
			if spans[i].to.After(cur.to) {
				cur.to = spans[i].to
			}
			continue
		}
		if cur != nil {
			total += cur.to.Sub(cur.from)
		}
		cur = &spans[i]
	}
	if cur != nil {
		total += cur.to.Sub(cur.from)
	}
	return total.Hours() / 24 / 365.25
}

// halfYears redondea a medio año (el alta acepta pasos de 0,5); el mínimo es 1 año (también si el CV no permite saberlo).
func halfYears(y float64) float64 {
	if y < 1 {
		return 1
	}
	if y > 60 {
		y = 60
	}
	return float64(int(y*2+0.5)) / 2
}

// FindExisting busca un talento con el mismo correo o documento (misma normalización que al guardar) o, si no,
// con el mismo nombre completo (el CV puede no traer correo ni documento).
// La carga rápida con CV lo usa para actualizar ese talento en vez de crear uno repetido.
func (s *TalentService) FindExisting(ctx context.Context, p *domain.Principal, email, doc, first, last, second string) (*domain.TalentMatch, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	var eb, db *string
	if e := strings.ToLower(strings.TrimSpace(email)); e != "" {
		b := s.Cipher.BlindIndex(e)
		eb = &b
	}
	if d := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(doc), " ", "")); d != "" {
		b := s.Cipher.BlindIndex(d)
		db = &b
	}
	fn, ln, sn := nameKey(first), nameKey(last), nameKey(second)
	if fn == nil || ln == nil {
		fn, ln, sn = nil, nil, nil // con nombre incompleto no se compara por nombre
	}
	if eb == nil && db == nil && fn == nil {
		return nil, nil
	}
	return s.Repo.Match(ctx, meta(p, ""), tid, eb, db, fn, ln, sn)
}

// nameKey: nombre sin espacios sobrantes (la BD compara sin distinguir mayúsculas ni tildes); nil si está vacío.
func nameKey(s string) *string {
	if s = strings.Join(strings.Fields(s), " "); s == "" || utf8.RuneCountInString(s) > 100 {
		return nil
	}
	return &s
}
