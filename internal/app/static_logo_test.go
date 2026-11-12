package app

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStaticLogo(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tenants/fractal/logo.png" {
			_, _ = w.Write(png)
			return
		}
		_, _ = w.Write([]byte("<!doctype html><html></html>")) // SPA: index.html para rutas inexistentes
	}))
	defer srv.Close()
	s := &TalentService{PublicAppURL: srv.URL + "/"}
	ctx := context.Background()
	if u := s.staticLogo(ctx, "/tenants/fractal/logo.png"); u == nil || !strings.HasPrefix(*u, "data:image/png;base64,") {
		t.Fatalf("logo fijo: %v", u)
	}
	for _, p := range []string{"/tenants/fractal/nada.png", "/../etc/passwd", "https://otro/x.png", "/tenants/Fr/logo.svg", "/api/v1/public/files/x"} {
		if u := s.staticLogo(ctx, p); u != nil {
			t.Errorf("%s: no debía devolver logo", p)
		}
	}
	if u := (&TalentService{}).staticLogo(ctx, "/tenants/fractal/logo.png"); u != nil {
		t.Error("sin PUBLIC_APP_URL no hay logo")
	}
}
