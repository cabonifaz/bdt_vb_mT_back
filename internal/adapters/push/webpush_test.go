package push

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

// El push sale cifrado (aes128gcm) y firmado con VAPID; 404/410 indican suscripción caducada.
func TestSend(t *testing.T) {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	status := http.StatusCreated
	var got *http.Request
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(status)
	}))
	defer srv.Close()

	key, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	target := ports.PushTarget{Endpoint: srv.URL + "/push/abc", P256dh: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),
		Auth: base64.RawURLEncoding.EncodeToString(auth)}

	w := New(pub, priv, "mailto:test@example.com")
	w.HTTP = srv.Client()
	if !w.Enabled() || w.PublicKey() != pub {
		t.Fatal("con claves el push queda activo")
	}
	gone, err := w.Send(context.Background(), target, []byte(`{"title":"Nuevo talento"}`))
	if err != nil || gone {
		t.Fatalf("envío: gone=%v err=%v", gone, err)
	}
	if got.Header.Get("Content-Encoding") != "aes128gcm" || !strings.HasPrefix(got.Header.Get("Authorization"), "vapid t=") || got.Header.Get("TTL") == "" {
		t.Fatalf("cabeceras: %v", got.Header)
	}
	status = http.StatusGone
	if gone, err = w.Send(context.Background(), target, []byte(`{}`)); !gone || err != nil {
		t.Fatalf("410 = caducada: gone=%v err=%v", gone, err)
	}
	status = http.StatusInternalServerError
	if _, err = w.Send(context.Background(), target, []byte(`{}`)); err == nil {
		t.Fatal("5xx es error")
	}
	if New("", "", "").Enabled() {
		t.Fatal("sin claves el push queda apagado")
	}
}
