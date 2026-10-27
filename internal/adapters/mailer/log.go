// Package mailer: adaptador de correo. Por ahora registra el envío en el log
// (sin el enlace completo); el adaptador SMTP/Graph se conecta al definir el proveedor.
package mailer

import (
	"context"
	"log/slog"
)

type LogMailer struct{ Log *slog.Logger }

func (m LogMailer) SendInvitation(_ context.Context, to, tenantName, link, locale string) error {
	m.Log.Info("invitación generada (mailer de desarrollo)", "tenant", tenantName, "locale", locale, "to_domain", domainOf(to))
	return nil
}

func domainOf(email string) string {
	for i := len(email) - 1; i >= 0; i-- {
		if email[i] == '@' {
			return email[i+1:]
		}
	}
	return ""
}
