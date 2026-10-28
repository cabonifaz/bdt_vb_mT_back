package httpapi

import "net/http"

// usageReport: consumo de DNI y tokens de IA por tenant (?from=YYYY-MM-DD&to=YYYY-MM-DD).
func (s *Server) usageReport(w http.ResponseWriter, r *http.Request) {
	out, err := s.Usage.Report(r.Context(), principal(r), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
