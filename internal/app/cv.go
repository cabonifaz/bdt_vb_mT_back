package app

import (
	"context"
	"encoding/json"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// Translator traduce los textos de un JSON conservando su estructura (OpenAI).
type Translator interface {
	TranslateJSON(ctx context.Context, in []byte, lang string) ([]byte, domain.AIUsage, error)
}

// CVText son los textos libres del CV que se traducen; nombres de empresas, instituciones y tecnologías no se tocan.
// Los catálogos (idiomas, niveles, grados, habilidades del maestro) ya tienen su etiqueta en/es.
type CVText struct {
	Headline    string       `json:"headline"`
	Summary     string       `json:"summary"`
	Experiences []CVTextItem `json:"experiences"`
	Educations  []CVTextItem `json:"educations"`
}

type CVTextItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`               // puesto o carrera
	Functions string `json:"functions,omitempty"` // funciones (experiencia)
}

// TranslateCV devuelve los textos del CV del talento en el idioma pedido (es | en).
func (s *TalentService) TranslateCV(ctx context.Context, p *domain.Principal, id, lang string) (*CVText, error) {
	if lang != "es" && lang != "en" {
		return nil, domain.ErrValidation
	}
	if s.Translator == nil {
		return nil, domain.NewError("AI_NOT_CONFIGURED")
	}
	t, err := s.Get(ctx, p, id)
	if err != nil {
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
	in, _ := json.Marshal(src)
	raw, usage, err := s.Translator.TranslateJSON(ctx, in, lang)
	// los tokens se cuentan por tenant aunque la respuesta no sirva; sin key no hay llamada
	if code := domain.AsError(err).Code; err == nil || (code != "AI_NOT_CONFIGURED" && code != "VALIDATION") {
		recordUsage(ctx, s.Usage, *p.TenantID, domain.UsageAITranslate, usage.Model, err == nil, usage)
	}
	if err != nil {
		return nil, err
	}
	var got CVText
	if err := json.Unmarshal(raw, &got); err != nil {
		return nil, domain.NewError("AI_UNAVAILABLE")
	}
	// solo se aceptan los elementos que existían (por id): el modelo no puede agregar contenido
	out := CVText{Headline: got.Headline, Summary: got.Summary, Experiences: keep(src.Experiences, got.Experiences), Educations: keep(src.Educations, got.Educations)}
	return &out, nil
}

func keep(src, got []CVTextItem) []CVTextItem {
	byID := map[string]CVTextItem{}
	for _, g := range got {
		byID[g.ID] = g
	}
	out := make([]CVTextItem, 0, len(src))
	for _, s := range src {
		if g, ok := byID[s.ID]; ok && g.Title != "" {
			if s.Functions == "" {
				g.Functions = ""
			}
			out = append(out, CVTextItem{ID: s.ID, Title: g.Title, Functions: g.Functions})
		} else {
			out = append(out, s)
		}
	}
	return out
}
