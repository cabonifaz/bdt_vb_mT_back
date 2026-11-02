package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GET /version (público): versión vigente del ambiente, para mostrarla junto al logo (también en el login).
func (s *Server) versionCurrent(w http.ResponseWriter, r *http.Request) {
	out, err := s.Versions.Current(r.Context())
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, out)
}

// GET /versions: historial del ambiente con sus notas (cualquier usuario autenticado).
func (s *Server) versionList(w http.ResponseWriter, r *http.Request) {
	out, err := s.Versions.List(r.Context())
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

// GET /platform/versions/environments: qué versión tiene cada ambiente (Super Admin).
func (s *Server) versionEnvironments(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	writeJSON(w, http.StatusOK, s.Versions.Environments(r.Context()))
}

// PATCH /platform/versions/{version}: estado de una versión del ambiente (Super Admin).
func (s *Server) versionStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.noContent(w, r, s.Versions.SetStatus(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "version"), in.Status))
}
