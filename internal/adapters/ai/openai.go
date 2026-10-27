// Package ai implementa la traducción de contenido con OpenAI (OPENAI_API_KEY, OPENAI_MODEL).
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
	return &OpenAI{Key: key, Model: model, URL: "https://api.openai.com/v1/chat/completions", HTTP: &http.Client{Timeout: 60 * time.Second}}
}

var langName = map[string]string{"es": "Spanish", "en": "English"}

// TranslateJSON traduce los valores de texto de un JSON al idioma indicado y devuelve un JSON con la misma estructura.
func (o *OpenAI) TranslateJSON(ctx context.Context, in []byte, lang string) ([]byte, error) {
	if o.Key == "" {
		return nil, errNotConfigured
	}
	target, ok := langName[lang]
	if !ok {
		return nil, domain.ErrValidation
	}
	body, _ := json.Marshal(map[string]any{
		"model":           o.Model,
		"temperature":     0.2,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "system", "content": "You translate professional résumé (CV) content for a recruiting company. " +
				"Translate every string value of the JSON you receive into " + target + ", keeping a professional tone. " +
				"Keep the exact same JSON structure, keys and \"id\" values. Do not translate company names, institution names, " +
				"product or technology names. If a value is already in " + target + ", return it unchanged. Reply with the JSON only."},
			{"role": "user", "content": string(in)},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+o.Key)
	req.Header.Set("Content-Type", "application/json")
	res, err := o.HTTP.Do(req)
	if err != nil {
		return nil, errUnavailable
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode == http.StatusUnauthorized {
		return nil, errNotConfigured
	}
	if res.StatusCode != http.StatusOK {
		return nil, errUnavailable
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 || !json.Valid([]byte(out.Choices[0].Message.Content)) {
		return nil, errUnavailable
	}
	return []byte(out.Choices[0].Message.Content), nil
}
