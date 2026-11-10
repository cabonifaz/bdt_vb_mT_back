package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// API de integración (key con alcance talent.read; la usa IGDT con la llave BDT automática): ficha completa, CV y
// postulaciones de los talentos que son candidatos en algún RQ del tenant de la key. Sin tarifas. Cada lectura se audita
// con la key.

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

// IntegrationFile: archivo del candidato (CV original o formato empresa ES/EN, documento o certificado), descifrado.
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
