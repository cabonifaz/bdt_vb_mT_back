package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Catálogo de funcionalidades (Super Admin): suspender o reactivar globalmente y activar o suspender por tenant.
func (s *Server) featureRoutes(r chi.Router) {
	manage := s.requirePerm("platform.features.manage")
	r.With(manage).Get("/features", s.featureList)
	r.With(manage).Patch("/features/{code}", s.featureStatus)
	r.With(manage).Get("/features/{code}/tenants", s.featureTenants)
	r.With(manage).Put("/features/{code}/tenants/{tenant}", s.featureTenantSet)
}

func (s *Server) featureList(w http.ResponseWriter, r *http.Request) {
	out, err := s.Features.List(r.Context(), principal(r))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) featureStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.noContent(w, r, s.Features.SetStatus(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "code"), in.Status))
}

func (s *Server) featureTenants(w http.ResponseWriter, r *http.Request) {
	out, err := s.Features.Tenants(r.Context(), principal(r), chi.URLParam(r, "code"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) featureTenantSet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	s.noContent(w, r, s.Features.SetTenant(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "tenant"), chi.URLParam(r, "code"), in.Enabled))
}
