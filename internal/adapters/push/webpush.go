// Package push envía notificaciones push del sistema operativo con Web Push (RFC 8030/8291) firmadas con VAPID.
// Claves en VAPID_PUBLIC_KEY / VAPID_PRIVATE_KEY (generarlas con: go run ./cmd/vapid); sin claves el push queda apagado.
package push

import (
	"context"
	"io"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

type WebPush struct {
	Public, Private, Subject string
	HTTP                     *http.Client
}

func New(public, private, subject string) *WebPush {
	if subject == "" {
		subject = "mailto:soporte@bancodetalentos.local"
	}
	return &WebPush{Public: public, Private: private, Subject: subject, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (w *WebPush) Enabled() bool     { return w.Public != "" && w.Private != "" }
func (w *WebPush) PublicKey() string { return w.Public }

func (w *WebPush) Send(ctx context.Context, t ports.PushTarget, payload []byte) (bool, error) {
	res, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: t.Endpoint, Keys: webpush.Keys{P256dh: t.P256dh, Auth: t.Auth},
	}, &webpush.Options{
		HTTPClient: w.HTTP, Subscriber: w.Subject, VAPIDPublicKey: w.Public, VAPIDPrivateKey: w.Private,
		TTL: 24 * 3600, Urgency: webpush.UrgencyNormal,
	})
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		return true, nil
	case res.StatusCode >= 300:
		return false, &statusError{res.StatusCode}
	}
	return false, nil
}

type statusError struct{ code int }

func (e *statusError) Error() string {
	return "push: el servicio respondió " + http.StatusText(e.code)
}
