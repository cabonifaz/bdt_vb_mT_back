package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GET /tenant/identity/dni/{dni}: nombres, apellidos y (según el proveedor) fecha de nacimiento.
func (s *Server) lookupDNI(w http.ResponseWriter, r *http.Request) {
	out, err := s.Identity.LookupDNI(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "dni"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}
