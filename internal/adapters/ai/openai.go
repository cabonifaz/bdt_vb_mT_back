// Package ai implementa con OpenAI la traducción del CV y la lectura de un CV (OPENAI_API_KEY, OPENAI_MODEL).
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

var (
	errNotConfigured = domain.NewError("AI_NOT_CONFIGURED")
	errUnavailable   = domain.NewError("AI_UNAVAILABLE")
)

type OpenAI struct {
	Key, Model, URL string
	HTTP            *http.Client
}

func NewOpenAI(key, model string) *OpenAI {
	if model == "" {
		model = "gpt-4.1-mini"
	}
	// leer un CV largo puede tardar: el límite cubre la respuesta completa del modelo
	return &OpenAI{Key: key, Model: model, URL: "https://api.openai.com/v1/chat/completions", HTTP: &http.Client{Timeout: 150 * time.Second}}
}

var langName = map[string]string{"es": "Spanish", "en": "English"}

// TranslateJSON traduce los valores de texto de un JSON al idioma indicado y devuelve un JSON con la misma estructura.
func (o *OpenAI) TranslateJSON(ctx context.Context, in []byte, lang string) ([]byte, domain.AIUsage, error) {
	target, ok := langName[lang]
	if !ok && o.Key != "" {
		return nil, domain.AIUsage{Model: o.Model}, domain.ErrValidation
	}
	return o.chatJSON(ctx, 0.2, "You translate professional résumé (CV) content for a recruiting company. "+
		"Translate every string value of the JSON you receive into "+target+", keeping a professional tone. "+
		"Keep the exact same JSON structure, keys and \"id\" values. Do not translate company names, institution names, "+
		"product or technology names. If a value is already in "+target+", return it unchanged. Reply with the JSON only.", string(in))
}

// cvPrompt: estructura que devuelve la lectura del CV (equivalente al analizador del sistema heredado, con códigos
// de los maestros en vez de ids). Las reglas de funciones y orden se conservan del prompt original.
const cvPrompt = `You are an assistant specialized in reading résumés (CVs) for a recruiting company.
You receive the text extracted from a PDF; it may come from OCR, so some words may be broken or misspelled.
Rebuild and correct only what is needed to make the information coherent, but NEVER invent data that is not present.
Keep every text in the language it was written in (do not translate).

Reply ONLY with a JSON object with exactly these keys (use null when a value is not in the CV, [] for empty lists):
{
  "firstName": "Juan Carlos",            // given names
  "lastName": "Pérez",                    // first (paternal) surname
  "secondLastName": "García",             // second (maternal) surname or null
  "docNumber": "12345678",                // national ID number (DNI) if present
  "email": "juan.perez@email.com",
  "phoneCode": "51",                      // international dialing code, digits only, no "+"
  "phoneNumber": "987654321",             // local number, digits only
  "countryCode": "PE",                    // ISO 3166-1 alpha-2 of the country of residence
  "city": "Lima",
  "linkedin": "https://linkedin.com/in/juanperez",
  "github": "https://github.com/juanperez",
  "headline": "Desarrollador Backend",    // current or main job title, short
  "summary": "...",                       // the profile/presentation paragraph exactly as in the CV
  "techSkills": [{ "name": "Java", "years": 0, "usedIn": [0, 2] }],
  "softSkills": ["Trabajo en equipo"],
  "experiences": [{ "company": "TechCorp", "position": "Desarrollador Backend", "functions": "...",
                    "startDate": "2020-01-15", "endDate": null, "current": true }],
  "educations": [{ "institution": "Universidad Nacional", "career": "Ingeniería de Sistemas",
                   "grade": "BACHELOR", "startDate": "2015-03-01", "endDate": "2019-12-15", "current": false, "yearOnly": true }],
  "languages": [{ "code": "EN", "name": "Inglés", "level": "ADVANCED" }]
}

Rules:
- Technical skills: list every technology, language, framework, database, tool or methodology mentioned anywhere
  (skills section, summary and the functions of each experience), one entry per technology with its usual name.
  "usedIn" = indexes (0-based, in the "experiences" array you return) of EVERY experience whose position or functions
  mention or clearly imply that technology. "years" = only when the CV states the years explicitly for that technology
  (e.g. "Java (8 años)", "5+ years of React"); otherwise 0 — the years are computed from the experience dates.
- Work experiences: most recent first (current ones first, then by end date, then by start date).
- Functions: do not summarize; keep them as close as possible to the original text, fixing only obvious spelling and
  grammar errors. Include BOTH the initial descriptive paragraph AND every bullet point under the experience, joined in a
  single string separated by line breaks (\n). Do not remove technical skills mentioned in the functions.
  You may summarize only from the 10th experience on, keeping the essence.
- Dates in yyyy-MM-dd. If only month/year is known use the first day of the month; if only the year, use January 1st
  and set "yearOnly": true (education). If "current" is true, "endDate" must be null.
- Education "grade" is one of: BACHELOR (bachiller), DEGREE (título / licenciatura / maestría / doctorado), COURSE
  (curso, certificación, diplomado), TECHNICAL (técnico), GRADUATE (egresado), STUDENT (estudiante); null if unknown.
- Certifications and courses go to "educations" with grade COURSE; if no institution is given use the issuer (e.g. AWS, Oracle)
  or, failing that, the certification name as "institution". Do not drop any certification.
- Language "code" is ISO 639-1 uppercase (ES, EN, FR, DE, ZH, PT, IT, ...); "level" one of BASIC, INTERMEDIATE, ADVANCED, NATIVE.
- Phone: split the international code from the local number; never include "+".
- No text outside the JSON.`

// ExtractCV lee el texto de un CV y devuelve sus datos estructurados (JSON con la forma de cvPrompt).
func (o *OpenAI) ExtractCV(ctx context.Context, text string) ([]byte, domain.AIUsage, error) {
	return o.chatJSON(ctx, 0, cvPrompt, "CV text:\n"+text)
}

// chatJSON llama al modelo pidiendo un objeto JSON y devuelve el contenido y los tokens consumidos.
func (o *OpenAI) chatJSON(ctx context.Context, temperature float64, system, user string) ([]byte, domain.AIUsage, error) {
	usage := domain.AIUsage{Model: o.Model}
	if o.Key == "" {
		return nil, usage, errNotConfigured
	}
	body, _ := json.Marshal(map[string]any{
		"model":           o.Model,
		"temperature":     temperature,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.URL, bytes.NewReader(body))
	if err != nil {
		return nil, usage, err
	}
	req.Header.Set("Authorization", "Bearer "+o.Key)
	req.Header.Set("Content-Type", "application/json")
	res, err := o.HTTP.Do(req)
	if err != nil {
		return nil, usage, errUnavailable
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode == http.StatusUnauthorized {
		return nil, usage, errNotConfigured
	}
	if res.StatusCode != http.StatusOK {
		return nil, usage, errUnavailable
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Model string `json:"model"`
		Usage struct {
			Prompt     int64 `json:"prompt_tokens"`
			Completion int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	err = json.Unmarshal(raw, &out)
	if out.Model != "" {
		usage.Model = out.Model
	}
	usage.PromptTokens, usage.CompletionTokens = out.Usage.Prompt, out.Usage.Completion
	if err != nil || len(out.Choices) == 0 || !json.Valid([]byte(out.Choices[0].Message.Content)) {
		return nil, usage, errUnavailable
	}
	return []byte(out.Choices[0].Message.Content), usage, nil
}
