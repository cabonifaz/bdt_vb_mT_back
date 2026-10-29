package cul

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Imita el MTPE: búsqueda en ISO-8859-1 (Ñ) y PDF como arreglo de bytes con signo.
func TestCheck(t *testing.T) {
	pdf := []byte("%PDF-1.4\xe2\xe3\xcf\xd3")
	var gotRUC, gotTipo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json;charset=ISO-8859-1")
		switch r.URL.Path {
		case "/buscarCertiJoven.html":
			gotRUC, gotTipo = r.URL.Query().Get("numeroRUC"), r.URL.Query().Get("tipoDocumento")
			w.Write([]byte("{\"estadoRespuesta\":2,\"T\":{\"a_fecoperac\":\"20/08/2026 15:40:34\",\"vNuDocNacimiento\":\"tok==\"," +
				"\"a_numsecuen\":14,\"v_apellpat\":\"PE\xd1A\",\"v_apellmat\":\"RIOS\",\"v_nombres\":\"ANA\",\"a_numdocume\":\"20269409807\",\"esVigente\":1}}"))
		case "/VerPDF.html":
			if r.URL.Query().Get("numeroDni") != "tok==" || r.URL.Query().Get("numeroSecuencia") != "14" {
				http.Error(w, "bad", http.StatusBadRequest)
				return
			}
			w.Write([]byte(`{"estadoRespuesta":0,"T":[37,80,68,70,45,49,46,52,-30,-29,-49,-45]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	m := &MTPE{Client: srv.Client(), BaseURL: srv.URL + "/"}
	c, err := m.Check(context.Background(), "20608337602", "20269409807", "DNI", "43989632")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(pdf)
	if !c.Found || !c.Vigente || c.Names != "ANA PEÑA RIOS" || c.OfficialSHA256 != hex.EncodeToString(sum[:]) ||
		c.IssuedAt.Format("2006-01-02 15:04") != "2026-08-20 15:40" || gotRUC != "20608337602" || gotTipo != "10080399" {
		t.Errorf("resultado: %+v ruc=%s tipo=%s", c, gotRUC, gotTipo)
	}
	if _, err := m.Check(context.Background(), "1", "2", "PASSPORT", "3"); err == nil {
		t.Error("pasaporte no lo admite el MTPE")
	}
}
