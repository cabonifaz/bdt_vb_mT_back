package identity

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

func TestDecolecta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tk" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("numero") == "00000000" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, `{"first_name":"MARIA DEL PILAR","first_last_name":"QUISPE","second_last_name":"ÑAHUI","full_name":"x","document_number":"46027897"}`)
	}))
	defer srv.Close()
	url := srv.URL + "/?numero=%s"
	c := New(Config{Token: "tk", BaseURL: url})
	p, err := c.LookupDNI(context.Background(), "46027897")
	if err != nil || p.FirstName != "Maria Del Pilar" || p.LastName != "Quispe" || p.SecondLastName != "Ñahui" || p.BirthDate != nil {
		t.Fatalf("got %+v %v", p, err)
	}
	if _, err := c.LookupDNI(context.Background(), "00000000"); domain.AsError(err).Code != "IDENTITY_NOT_FOUND" {
		t.Fatalf("want not found, got %v", err)
	}
	if _, err := New(Config{BaseURL: url}).LookupDNI(context.Background(), "1"); domain.AsError(err).Code != "IDENTITY_NOT_CONFIGURED" {
		t.Fatalf("want not configured, got %v", err)
	}
	if _, err := New(Config{Token: "bad", BaseURL: url}).LookupDNI(context.Background(), "1"); domain.AsError(err).Code != "IDENTITY_NOT_CONFIGURED" {
		t.Fatalf("want not configured on 401, got %v", err)
	}
}

func TestFactiliza(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":200,"success":true,"data":{"numero":"46027897","nombres":"JUAN","apellido_paterno":"PEREZ","apellido_materno":"LOPEZ","fecha_nacimiento":"15/03/1990"}}`)
	}))
	defer srv.Close()
	p, err := New(Config{Provider: "factiliza", Token: "tk", BaseURL: srv.URL + "/%s"}).LookupDNI(context.Background(), "46027897")
	if err != nil || p.FirstName != "Juan" || p.BirthDate == nil || *p.BirthDate != "1990-03-15" || p.Provider != "factiliza" {
		t.Fatalf("got %+v %v", p, err)
	}
}
