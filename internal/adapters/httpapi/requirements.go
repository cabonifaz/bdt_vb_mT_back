package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

// Requerimientos: tablero por estado, ficha, alta/edición, pipeline de candidatos y contactos del cliente.
func (s *Server) requirementRoutes(r chi.Router) {
	read := s.requirePerm("rq.read")
	write := s.requirePerm("rq.write")
	cand := s.requirePerm("rq.candidates")
	r.Use(s.idempotent)

	// tablero por POST: los filtros no quedan en URLs ni en logs de acceso
	r.With(read).Post("/requirements/board", s.rqBoard)
	r.With(read).Get("/requirements/{id}", s.rqGet)
	r.With(write).Put("/requirements/{id}", s.rqSave)
	r.With(write).Patch("/requirements/{id}/status", s.rqStatus)
	r.With(s.requirePerm("rq.delete")).Delete("/requirements/{id}", s.rqDelete)
	r.With(s.requirePerm("rq.reopen")).Post("/requirements/{id}/reopen", s.rqReopen)
	r.With(cand).Post("/requirements/{id}/candidates", s.rqAddCandidate)
	r.With(cand).Patch("/requirements/candidates/{cid}", s.rqMoveCandidate)
	r.With(cand).Delete("/requirements/candidates/{cid}", s.rqRemoveCandidate)
	r.With(read).Get("/talents/{id}/applications", s.rqByTalent)
	r.With(read).Get("/clients/{client}/contacts", s.rqContacts)
	r.With(write).Put("/clients/{client}/contacts/{id}", s.rqSaveContact)
	r.With(write).Delete("/client-contacts/{id}", s.rqDeleteContact)
}

func (s *Server) rqBoard(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Scope     string `json:"scope"`
		Search    string `json:"search"`
		ClientID  string `json:"clientId"`
		ContactID string `json:"contactId"`
		ProfileID string `json:"profileId"`
		Mine      bool   `json:"mine"`
		Today     string `json:"today"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	out, err := s.Requirements.Board(r.Context(), principal(r), app.BoardFilter{Scope: in.Scope, Search: in.Search,
		ClientID: in.ClientID, ContactID: in.ContactID, ProfileID: in.ProfileID, Mine: in.Mine, Today: in.Today})
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) rqGet(w http.ResponseWriter, r *http.Request) {
	out, err := s.Requirements.Get(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) rqSave(w http.ResponseWriter, r *http.Request) {
	var in app.RQInput
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	c, err := s.Requirements.Save(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), in)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	status := http.StatusOK
	if c.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, c)
}

func (s *Server) rqStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		StatusID string `json:"statusId"`
		Order    *int   `json:"order"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	c, err := s.Requirements.SetStatus(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), in.StatusID, in.Order)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) rqReopen(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason     string   `json:"reason"`
		Notes      string   `json:"notes"`
		Candidates []string `json:"candidates"`
		StatusID   string   `json:"statusId"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	c, err := s.Requirements.Reopen(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), in.Reason, in.Notes, in.Candidates, in.StatusID)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) rqDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.Requirements.Delete(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id")); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) rqAddCandidate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID        string `json:"id"`
		TalentID  string `json:"talentId"`
		VacancyID string `json:"vacancyId"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	created, err := s.Requirements.AddCandidate(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), in.ID, in.TalentID, in.VacancyID)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]string{"id": in.ID})
}

func (s *Server) rqMoveCandidate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		StageID   string  `json:"stageId"`
		VacancyID *string `json:"vacancyId"`
		Order     *int    `json:"order"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	stage, err := s.Requirements.MoveCandidate(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "cid"), in.StageID, in.VacancyID, in.Order)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"stage": stage})
}

func (s *Server) rqRemoveCandidate(w http.ResponseWriter, r *http.Request) {
	if err := s.Requirements.RemoveCandidate(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "cid")); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) rqByTalent(w http.ResponseWriter, r *http.Request) {
	out, err := s.Requirements.ByTalent(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) rqContacts(w http.ResponseWriter, r *http.Request) {
	out, err := s.Requirements.Contacts(r.Context(), principal(r), chi.URLParam(r, "client"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) rqSaveContact(w http.ResponseWriter, r *http.Request) {
	var in ports.ContactWrite
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	in.ID, in.ClientID = chi.URLParam(r, "id"), chi.URLParam(r, "client")
	if err := s.Requirements.SaveContact(r.Context(), principal(r), s.clientIP(r), in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) rqDeleteContact(w http.ResponseWriter, r *http.Request) {
	if err := s.Requirements.DeleteContact(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id")); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
