package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

// Resend: correo saliente por la API HTTP de Resend (https://resend.com/docs/api-reference/emails/send-email).
// La llave viene de RESEND_API_KEY; el remitente (MAIL_FROM) debe ser de un dominio verificado en Resend.
// RedirectTo (MAIL_REDIRECT_TO, para staging): si tiene valor, TODO correo va solo a esa dirección (sin CC ni BCC), con
// el asunto prefijado "[STAGING] " y una primera línea "Destinatario original: <lista>" en el texto.
type Resend struct {
	Key, From, FromName string
	RedirectTo          string
	BaseURL             string // por defecto https://api.resend.com (las pruebas lo cambian)
	HTTP                *http.Client
}

func (r Resend) Enabled() bool { return r.Key != "" && r.From != "" }

func (r Resend) SendInvitation(ctx context.Context, to, tenantName, link, locale string) error {
	subject := "Invitación al Banco de Talentos"
	body := fmt.Sprintf("Te invitaron a %s en el Banco de Talentos.\n\nActiva tu cuenta aquí: %s\n\nSi no esperabas esta invitación, ignora este correo.\n", tenantName, link)
	if strings.HasPrefix(locale, "en") {
		subject = "Talent Bank invitation"
		body = fmt.Sprintf("You were invited to %s on the Talent Bank.\n\nActivate your account: %s\n\nIf you did not expect this invitation, ignore this email.\n", tenantName, link)
	}
	return r.send(ctx, to, subject, body)
}

func (r Resend) send(ctx context.Context, to, subject, text string) error {
	addr, err := mail.ParseAddress(strings.TrimSpace(to))
	if err != nil {
		return fmt.Errorf("dirección inválida")
	}
	recipients := []string{addr.Address}
	if redirect := strings.TrimSpace(r.RedirectTo); redirect != "" {
		// staging: nunca se escribe al destinatario real; con una redirección inválida no se envía nada
		ra, err := mail.ParseAddress(redirect)
		if err != nil {
			return fmt.Errorf("MAIL_REDIRECT_TO inválido")
		}
		text = "Destinatario original: " + strings.Join(recipients, ", ") + "\n\n" + text
		subject = "[STAGING] " + subject
		recipients = []string{ra.Address}
	}
	from := r.From
	if r.FromName != "" {
		from = fmt.Sprintf("%s <%s>", strings.NewReplacer("<", "", ">", "", "\"", "").Replace(r.FromName), r.From)
	}
	raw, err := json.Marshal(map[string]any{"from": from, "to": recipients, "subject": subject, "text": text})
	if err != nil {
		return err
	}
	base := r.BaseURL
	if base == "" {
		base = "https://api.resend.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/emails", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.Key)
	req.Header.Set("Content-Type", "application/json")
	client := r.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("resend: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		var e struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<10))
		_ = json.Unmarshal(b, &e)
		return fmt.Errorf("resend %d: %s %s", res.StatusCode, e.Name, e.Message)
	}
	return nil
}
