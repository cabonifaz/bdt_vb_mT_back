package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

func TestTranslateJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer k" || !strings.Contains(string(b), "into English") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		content, _ := json.Marshal(`{"headline":"Software architect"}`)
		_, _ = w.Write([]byte(`{"model":"gpt-x","usage":{"prompt_tokens":40,"completion_tokens":12},"choices":[{"message":{"content":` + string(content) + `}}]}`))
	}))
	defer srv.Close()
	o := NewOpenAI("k", "")
	o.URL = srv.URL
	out, u, err := o.TranslateJSON(context.Background(), []byte(`{"headline":"Arquitecto de software"}`), "en")
	if err != nil || string(out) != `{"headline":"Software architect"}` {
		t.Fatalf("got %s %v", out, err)
	}
	if u.Model != "gpt-x" || u.PromptTokens != 40 || u.CompletionTokens != 12 {
		t.Fatalf("usage %+v", u)
	}
	if _, _, err := NewOpenAI("", "").TranslateJSON(context.Background(), []byte(`{}`), "en"); domain.AsError(err).Code != "AI_NOT_CONFIGURED" {
		t.Fatalf("want not configured, got %v", err)
	}
	if _, _, err := o.TranslateJSON(context.Background(), []byte(`{}`), "fr"); domain.AsError(err).Code != "VALIDATION" {
		t.Fatalf("want validation, got %v", err)
	}
}
