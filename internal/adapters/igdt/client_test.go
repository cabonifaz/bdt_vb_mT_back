package igdt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

func TestLinkBDT(t *testing.T) {
	var got struct {
		method, key, apiKey string
	}
	status := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/integration/v1/bdt-link" {
			http.NotFound(w, r)
			return
		}
		got.method, got.key = r.Method, r.Header.Get("X-Api-Key")
		var in struct {
			APIKey string `json:"apiKey"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		got.apiKey = in.APIKey
		w.WriteHeader(status)
	}))
	defer srv.Close()
	c := New(srv.URL)
	ctx := context.Background()

	if err := c.LinkBDT(ctx, "igdt_token", "bdt_abc_secret"); err != nil {
		t.Fatalf("LinkBDT: %v", err)
	}
	if got.method != http.MethodPut || got.key != "igdt_token" || got.apiKey != "bdt_abc_secret" {
		t.Fatalf("petición inesperada: %+v", got)
	}
	if err := c.UnlinkBDT(ctx, "igdt_token"); err != nil || got.method != http.MethodDelete {
		t.Fatalf("UnlinkBDT: %v (%s)", err, got.method)
	}

	status = http.StatusNotFound // IGDT sin el endpoint
	if err := c.LinkBDT(ctx, "igdt_token", "bdt_abc_secret"); domain.AsError(err).Code != "IGDT_LINK_UNSUPPORTED" {
		t.Fatalf("404: esperaba IGDT_LINK_UNSUPPORTED, obtuve %v", err)
	}
	status = http.StatusBadGateway
	if err := c.LinkBDT(ctx, "igdt_token", "bdt_abc_secret"); domain.AsError(err).Code != "IGDT_UNAVAILABLE" {
		t.Fatalf("502: esperaba IGDT_UNAVAILABLE, obtuve %v", err)
	}
	status = http.StatusUnauthorized
	if err := c.LinkBDT(ctx, "igdt_token", "bdt_abc_secret"); domain.AsError(err).Code != "IGDT_KEY_INVALID" {
		t.Fatalf("401: esperaba IGDT_KEY_INVALID, obtuve %v", err)
	}
}

// Managers: GET /integration/v1/managers con el token; 403 = falta el alcance.
func TestManagers(t *testing.T) {
	status := http.StatusOK
	var key, method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/integration/v1/managers" {
			http.NotFound(w, r)
			return
		}
		key, method = r.Header.Get("X-Api-Key"), r.Method
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"managers":[{"id":"m1","name":"Ana"}],"clients":[{"clientId":"c1","primaryId":"m1","secondaryId":""}]}`))
	}))
	defer srv.Close()
	c := New(srv.URL)
	out, err := c.Managers(context.Background(), "igdt_token")
	if err != nil || method != http.MethodGet || key != "igdt_token" {
		t.Fatalf("Managers: %v %s %s", err, method, key)
	}
	if len(out.Managers) != 1 || out.Managers[0].Name != "Ana" || out.Clients[0].PrimaryID != "m1" || out.Clients[0].SecondaryID != "" {
		t.Fatalf("respuesta: %+v", out)
	}
	status = http.StatusForbidden
	if _, err := c.Managers(context.Background(), "igdt_token"); domain.AsError(err).Code != "IGDT_SCOPE_MISSING" {
		t.Fatalf("403: %v", err)
	}
}
