// Package httpapi es el adaptador de entrada HTTP (REST JSON).
package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type errorBody struct {
	Error struct {
		Code      string `json:"code"`
		RequestID string `json:"requestId,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// writeErr nunca expone detalles internos: solo un código estable que la UI traduce.
func (s *Server) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	de := domain.AsError(err)
	if de == domain.ErrInternal {
		s.log.Error("error interno", "err", err, "path", r.URL.Path, "rid", requestID(r))
	}
	var b errorBody
	b.Error.Code = de.Code
	b.Error.RequestID = requestID(r)
	writeJSON(w, de.Status, b)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return domain.ErrValidation
	}
	if dec.More() {
		return domain.ErrValidation
	}
	return nil
}

func qInt(r *http.Request, key string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(key)); err == nil {
		return v
	}
	return def
}
