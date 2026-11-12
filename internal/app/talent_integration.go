package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// API de integración (key con alcance talent.read; la usa IGDT con la llave BDT automática): ficha completa, CV y
// postulaciones de los talentos que son candidatos en algún RQ del tenant de la key. Con el alcance talent.search la key
// además busca en todo el banco de talentos del tenant y abre la ficha, CV y documentos de cualquier talento (no solo
// candidatos). Sin tarifas. Cada lectura se audita con la key.

const (
	integrationPageSize    = 20
	integrationMaxPageSize = 50
	integrationMaxMinYears = 100
	integrationMaxSkills   = 30
)

var englishLevelCode = regexp.MustCompile(`^[A-Z_]{1,30}$`)

// ParseIntegrationTalentQuery valida los parámetros de GET /integration/v1/talents (get devuelve el valor del query
// param): textos de hasta 150 caracteres, english como código de nivel, skills hasta 30 UUID separados por coma (sin
// repetidos), gradeId UUID, status vacío o ARCHIVED, minYears número entre 0 y 100, page ≥ 1 y pageSize entre 1 y 50
// (por defecto 20). Cualquier valor inválido es VALIDATION.
func ParseIntegrationTalentQuery(get func(string) string) (ports.IntegrationTalentFilter, error) {
	f := ports.IntegrationTalentFilter{
		Search: strings.TrimSpace(get("q")), Position: strings.TrimSpace(get("position")),
		Education: strings.TrimSpace(get("education")), Skill: strings.TrimSpace(get("skill")),
		EnglishLevel: strings.ToUpper(strings.TrimSpace(get("english"))), Page: 1, PageSize: integrationPageSize,
		GradeID: strings.ToLower(strings.TrimSpace(get("gradeId"))), Status: strings.ToUpper(strings.TrimSpace(get("status"))),
	}
	for _, v := range []string{f.Search, f.Position, f.Education, f.Skill} {
		if len(v) > 150 {
			return f, domain.ErrValidation
		}
	}
	if f.EnglishLevel != "" && !englishLevelCode.MatchString(f.EnglishLevel) {
		return f, domain.ErrValidation
	}
	if v := strings.TrimSpace(get("skills")); v != "" {
		parts := strings.Split(v, ",")
		if len(parts) > integrationMaxSkills {
			return f, domain.ErrValidation
		}
		for _, p := range parts {
			p = strings.ToLower(strings.TrimSpace(p))
			if len(p) != 36 || !validUUID(p) {
				return f, domain.ErrValidation
			}
			if !slices.Contains(f.Skills, p) {
				f.Skills = append(f.Skills, p)
			}
		}
	}
	if f.GradeID != "" && (len(f.GradeID) != 36 || !validUUID(f.GradeID)) {
		return f, domain.ErrValidation
	}
	if f.Status != "" && f.Status != "ARCHIVED" {
		return f, domain.ErrValidation
	}
	if v := strings.TrimSpace(get("minYears")); v != "" {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(n) || n < 0 || n > integrationMaxMinYears {
			return f, domain.ErrValidation
		}
		f.MinYears = &n
	}
	if v := strings.TrimSpace(get("page")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100000 {
			return f, domain.ErrValidation
		}
		f.Page = n
	}
	if v := strings.TrimSpace(get("pageSize")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > integrationMaxPageSize {
			return f, domain.ErrValidation
		}
		f.PageSize = n
	}
	return f, nil
}

// IntegrationSearch: búsqueda en el banco de talentos del tenant de la key (ACTIVE o, con status=ARCHIVED, archivados),
// sin datos personales cifrados ni tarifas; de la lista negra solo isBlacklisted. Auditada con la key (una fila por búsqueda).
func (s *TalentService) IntegrationSearch(ctx context.Context, c *domain.ApiClient, ip string, f ports.IntegrationTalentFilter) (*ports.IntegrationTalentPage, error) {
	if c == nil {
		return nil, domain.NewError("APIKEY_INVALID")
	}
	return s.Repo.IntegrationSearch(ctx, c.TenantID, c.KeyID, f, ip)
}

// IntegrationDossier: ficha del candidato con los datos personales descifrados.
func (s *TalentService) IntegrationDossier(ctx context.Context, c *domain.ApiClient, ip, talentID string) (json.RawMessage, error) {
	if c == nil {
		return nil, domain.NewError("APIKEY_INVALID")
	}
	if !validUUID(talentID) {
		return nil, domain.NewError("TALENT_NOT_FOUND")
	}
	doc, err := s.Repo.IntegrationDossier(ctx, c.TenantID, c.KeyID, talentID, ip)
	if err != nil {
		return nil, err
	}
	return decryptKeysOpt(s.Cipher, doc, true, "docNumberEnc", "emailEnc", "phoneEnc", "birthDateEnc", "authorNameEnc"), nil
}

// IntegrationFile: archivo del candidato (CV original o formato empresa ES/EN, documento, certificado o su foto
// vigente), descifrado.
func (s *TalentService) IntegrationFile(ctx context.Context, c *domain.ApiClient, ip, talentID, fileID string) (*domain.FileObject, []byte, error) {
	if c == nil {
		return nil, nil, domain.NewError("APIKEY_INVALID")
	}
	if !validUUID(talentID) || !validUUID(fileID) {
		return nil, nil, domain.NewError("FILE_NOT_FOUND")
	}
	f, err := s.Repo.IntegrationFile(ctx, c.TenantID, c.KeyID, talentID, fileID, ip)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.Storage.Open(ctx, f.StorageKey)
	if err != nil {
		return nil, nil, domain.NewError("FILE_NOT_FOUND")
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, maxTalentFile*2))
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

// maxCVImage: tope por imagen incrustada en cv-source (cabecera, pie, logo, foto); si lo supera sale null.
const maxCVImage = 2 << 20

// CVSourceTemplate: plantilla del CV del tenant (como buildCvTemplate del front) con las imágenes en data URL.
type CVSourceTemplate struct {
	DisplayName    string          `json:"displayName"`
	ColorPrimary   string          `json:"colorPrimary"`
	ColorSecondary string          `json:"colorSecondary"`
	CVFooterText   *string         `json:"cvFooterText"`
	CVShowPhoto    bool            `json:"cvShowPhoto"`
	CVStyle        json.RawMessage `json:"cvStyle"`
	Header         *string         `json:"header"`
	Footer         *string         `json:"footer"`
	Logo           *string         `json:"logo"`
	Photo          *string         `json:"photo"`
	// LogoPath: logo fijo del front de BDT (logo_url del seed). Solo se usa para incrustarlo; no sale en la respuesta.
	LogoPath *string `json:"logoPath,omitempty"`
}

// CVSource: insumos para generar el CV con la marca del tenant igual que BDT (buildCvData + CvDocument).
type CVSource struct {
	Talent        json.RawMessage  `json:"talent"`
	CourseGradeID *string          `json:"courseGradeId"`
	Text          *CVText          `json:"text,omitempty"` // solo lang=en: textos traducidos (como POST /talents/{id}/cv/translate)
	Template      CVSourceTemplate `json:"template"`
}

// IntegrationCVSource: ficha del talento en la forma que usa el CV, plantilla del tenant con sus imágenes y, en inglés,
// los textos traducidos con IA (sin IA configurada: AI_NOT_CONFIGURED).
func (s *TalentService) IntegrationCVSource(ctx context.Context, c *domain.ApiClient, ip, talentID, lang string) (*CVSource, error) {
	if c == nil {
		return nil, domain.NewError("APIKEY_INVALID")
	}
	if lang == "" {
		lang = "es"
	}
	if lang != "es" && lang != "en" {
		return nil, domain.ErrValidation
	}
	if !validUUID(talentID) {
		return nil, domain.NewError("TALENT_NOT_FOUND")
	}
	doc, imgs, err := s.Repo.IntegrationCVSource(ctx, c.TenantID, c.KeyID, talentID, lang, ip)
	if err != nil {
		return nil, err
	}
	var out CVSource
	if err := json.Unmarshal(doc, &out); err != nil {
		return nil, err
	}
	if len(out.Template.CVStyle) == 0 {
		out.Template.CVStyle = json.RawMessage("null")
	}
	if lang == "en" {
		var t struct {
			Headline    *string `json:"headline"`
			Summary     *string `json:"summary"`
			Experiences []struct {
				ID        string  `json:"id"`
				Position  string  `json:"position"`
				Functions *string `json:"functions"`
			} `json:"experiences"`
			Educations []struct {
				ID     string `json:"id"`
				Career string `json:"career"`
			} `json:"educations"`
		}
		if err := json.Unmarshal(out.Talent, &t); err != nil {
			return nil, err
		}
		src := CVText{Experiences: []CVTextItem{}, Educations: []CVTextItem{}}
		if t.Headline != nil {
			src.Headline = *t.Headline
		}
		if t.Summary != nil {
			src.Summary = *t.Summary
		}
		for _, e := range t.Experiences {
			it := CVTextItem{ID: e.ID, Title: e.Position}
			if e.Functions != nil {
				it.Functions = *e.Functions
			}
			src.Experiences = append(src.Experiences, it)
		}
		for _, e := range t.Educations {
			src.Educations = append(src.Educations, CVTextItem{ID: e.ID, Title: e.Career})
		}
		if out.Text, err = s.translateCVText(ctx, c.TenantID, src, lang); err != nil {
			return nil, err
		}
	}
	for _, im := range imgs {
		url := s.imageDataURL(ctx, im)
		switch im.Kind {
		case "HEADER":
			out.Template.Header = url
		case "FOOTER":
			out.Template.Footer = url
		case "LOGO":
			out.Template.Logo = url
		case "PHOTO":
			out.Template.Photo = url
		}
	}
	// como BDT (buildCvTemplate): sin cabecera ni logo subido, el CV lleva el logo fijo del tenant (logoUrl del branding)
	if out.Template.Header == nil && out.Template.Logo == nil && out.Template.LogoPath != nil {
		out.Template.Logo = s.staticLogo(ctx, *out.Template.LogoPath)
	}
	out.Template.LogoPath = nil
	return &out, nil
}

// imageDataURL lee la imagen del almacenamiento (descifrada) como data:<mime>;base64,...; nil si falta, no se puede
// leer, no es una imagen o supera maxCVImage.
func (s *TalentService) imageDataURL(ctx context.Context, im ports.CVImage) *string {
	if im.StorageKey == "" || im.Size > maxCVImage {
		return nil
	}
	rc, err := s.Storage.Open(ctx, im.StorageKey)
	if err != nil {
		return nil
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, maxCVImage+64<<10))
	if err != nil {
		return nil
	}
	if im.Encrypted {
		if raw, err = s.Cipher.DecryptBytes(raw); err != nil {
			return nil
		}
	}
	if len(raw) == 0 || len(raw) > maxCVImage {
		return nil
	}
	mt := im.MimeType
	if mt == "" || mt == "application/octet-stream" {
		mt = http.DetectContentType(raw)
	}
	if !strings.HasPrefix(mt, "image/") {
		return nil
	}
	url := "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(raw)
	return &url
}

// staticLogoPath: logos fijos que publica el front de BDT (public/tenants/<slug>/...).
var staticLogoPath = regexp.MustCompile(`^/tenants/[a-z0-9-]+/[A-Za-z0-9._-]+\.(png|jpe?g|webp)$`)

// staticLogo descarga del front de BDT (PUBLIC_APP_URL) el logo fijo del tenant y lo devuelve como data URL; nil si
// la ruta no es un logo fijo, no hay URL del front o la descarga falla.
func (s *TalentService) staticLogo(ctx context.Context, p string) *string {
	if s.PublicAppURL == "" || !staticLogoPath.MatchString(p) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.PublicAppURL, "/")+p, nil)
	if err != nil {
		return nil
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxCVImage+1))
	if err != nil || len(raw) == 0 || len(raw) > maxCVImage {
		return nil
	}
	mt := http.DetectContentType(raw)
	if !strings.HasPrefix(mt, "image/") {
		return nil // p. ej. el index.html del SPA cuando el archivo no existe
	}
	url := "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(raw)
	return &url
}
