package mailer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResendInvitation(t *testing.T) {
	var got map[string]any
	var auth string
	status := 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"name":"validation_error","message":"domain not verified"}`))
	}))
	defer srv.Close()
	r := Resend{Key: "re_x", From: "no-reply@empresa.pe", FromName: "Banco de Talentos", BaseURL: srv.URL}
	if err := r.SendInvitation(context.Background(), "ana@x.pe", "Fractal", "https://bdt/x", "es"); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer re_x" || got["from"] != "Banco de Talentos <no-reply@empresa.pe>" || !strings.Contains(got["text"].(string), "https://bdt/x") {
		t.Fatalf("petición: %s %v", auth, got)
	}
	status = 403
	if err := r.SendInvitation(context.Background(), "ana@x.pe", "Fractal", "l", "en"); err == nil || strings.Contains(err.Error(), "re_x") {
		t.Fatalf("error: %v", err)
	}
}
