package httpapi

import (
	"net/http"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app"
)

// Indicadores de selección por POST: el período y el cliente no quedan en URLs ni en logs de acceso.
func (s *Server) dashboardSelection(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Grain    string `json:"grain"`
		ClientID string `json:"clientId"`
		Today    string `json:"today"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	out, err := s.Dashboard.Selection(r.Context(), principal(r), app.DashboardFilter{From: in.From, To: in.To, Grain: in.Grain, ClientID: in.ClientID, Today: in.Today})
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}
