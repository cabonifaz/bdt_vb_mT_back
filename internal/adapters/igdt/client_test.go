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
