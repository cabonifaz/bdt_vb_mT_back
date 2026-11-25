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

// MAIL_REDIRECT_TO: todo correo va solo a la dirección de redirección (sin CC/BCC), con [STAGING] en el asunto y el
// destinatario original al inicio del texto; con una redirección inválida no se envía nada.
func TestResendRedirect(t *testing.T) {
	var got map[string]any
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		got = nil
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	r := Resend{Key: "re_x", From: "no-reply@empresa.pe", RedirectTo: " qa@empresa.pe ", BaseURL: srv.URL}
	if err := r.SendInvitation(context.Background(), "Ana <ana@x.pe>", "Fractal", "https://bdt/x", "es"); err != nil {
		t.Fatal(err)
	}
	to, _ := got["to"].([]any)
	if len(to) != 1 || to[0] != "qa@empresa.pe" {
		t.Fatalf("to: %v", got["to"])
	}
	for _, k := range []string{"cc", "bcc", "reply_to"} {
		if _, ok := got[k]; ok {
			t.Fatalf("no debe llevar %s: %v", k, got)
		}
	}
	if got["subject"] != "[STAGING] Invitación al Banco de Talentos" {
		t.Fatalf("asunto: %v", got["subject"])
	}
	text := got["text"].(string)
	if !strings.HasPrefix(text, "Destinatario original: ana@x.pe\n") || !strings.Contains(text, "https://bdt/x") {
		t.Fatalf("texto: %q", text)
	}
	bad := Resend{Key: "re_x", From: "no-reply@empresa.pe", RedirectTo: "no-es-correo", BaseURL: srv.URL}
	if err := bad.SendInvitation(context.Background(), "ana@x.pe", "Fractal", "l", "es"); err == nil || calls != 1 {
		t.Fatalf("redirección inválida: %v (llamadas %d)", err, calls)
	}
	plain := Resend{Key: "re_x", From: "no-reply@empresa.pe", BaseURL: srv.URL}
	if err := plain.SendInvitation(context.Background(), "ana@x.pe", "Fractal", "l", "es"); err != nil {
		t.Fatal(err)
	}
	if to, _ := got["to"].([]any); len(to) != 1 || to[0] != "ana@x.pe" || strings.HasPrefix(got["subject"].(string), "[STAGING]") {
		t.Fatalf("sin redirección: %v", got)
	}
}
