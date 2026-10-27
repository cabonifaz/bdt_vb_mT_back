package httpapi

import "net/http"

func (s *Server) homeSummary(w http.ResponseWriter, r *http.Request) {
	out, err := s.Home.Summary(r.Context(), principal(r))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
