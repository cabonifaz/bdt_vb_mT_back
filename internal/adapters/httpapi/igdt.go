package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Integración con IGDT (Administración › Integración IGDT) y tarifa sugerida desde el tarifario de IGDT.
func (s *Server) igdtRoutes(r chi.Router) {
	manage := s.requirePerm("tenant.integrations.manage")
	r.With(manage).Get("/integrations/igdt", s.igdtStatus)
	r.With(manage).Put("/integrations/igdt", s.igdtConnect)
	r.With(manage).Delete("/integrations/igdt", s.igdtDisconnect)
	r.With(manage).Post("/integrations/igdt/sync", s.igdtSync)
	r.With(s.requirePerm("rq.rates.read")).Get("/catalog/tariff", s.igdtTariff)
}

func (s *Server) igdtStatus(w http.ResponseWriter, r *http.Request) {
	out, err := s.IGDT.Status(r.Context(), principal(r))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

// igdtConnect recibe el token de integración de IGDT (nunca se devuelve ni se registra).
func (s *Server) igdtConnect(w http.ResponseWriter, r *http.Request) {
	var in struct {
		APIKey string `json:"apiKey"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	out, err := s.IGDT.Connect(r.Context(), principal(r), s.clientIP(r), in.APIKey)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) igdtDisconnect(w http.ResponseWriter, r *http.Request) {
	s.noContent(w, r, s.IGDT.Disconnect(r.Context(), principal(r), s.clientIP(r)))
}

func (s *Server) igdtSync(w http.ResponseWriter, r *http.Request) {
	out, err := s.IGDT.SyncNow(r.Context(), principal(r))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) igdtTariff(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out, err := s.IGDT.TariffLookup(r.Context(), principal(r), q.Get("profileId"), q.Get("clientId"), q.Get("date"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	if out == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
