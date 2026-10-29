package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// Documentos de postulación: maestro del tenant y documentos de cada postulante dentro de un RQ.
func (s *Server) documentRoutes(r chi.Router) {
	read := s.requirePerm("rq.read")
	cand := s.requirePerm("rq.candidates")
	r.Use(s.idempotent)

	r.With(read).Get("/document-rules", s.docRules)
	r.With(s.requirePerm("tenant.masters.manage")).Put("/document-rules/{type}", s.docSaveRule)
	// ficha por POST: lleva la fecha local del usuario (define qué está vencido)
	r.With(read).Post("/requirements/candidates/{cid}/detail", s.candidateDetail)
	r.With(cand).Post("/requirements/candidates/{cid}/documents", s.docUpload)
	r.With(cand).Post("/requirements/candidates/{cid}/documents/{did}/verify", s.docVerify)
	r.With(cand).Post("/requirements/candidates/{cid}/documents/{did}/recheck", s.docRecheck)
}

func (s *Server) docRules(w http.ResponseWriter, r *http.Request) {
	out, err := s.Documents.Rules(r.Context(), principal(r))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) docSaveRule(w http.ResponseWriter, r *http.Request) {
	var in ports.DocRule
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	in.TypeID = chi.URLParam(r, "type")
	if err := s.Documents.SaveRule(r.Context(), principal(r), s.clientIP(r), in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) candidateDetail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Today string `json:"today"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	out, err := s.Documents.CandidateDetail(r.Context(), principal(r), chi.URLParam(r, "cid"), in.Today)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

// docUpload: multipart con file, typeId, today y, para el CUL, culCode y culDoc (leídos del QR en el navegador).
func (s *Server) docUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
	if err := r.ParseMultipartForm(11 << 20); err != nil {
		s.writeErr(w, r, domain.NewError("FILE_TOO_LARGE"))
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		s.writeErr(w, r, domain.ErrValidation)
		return
	}
	defer f.Close()
	hint := app.CULHint{Code: r.FormValue("culCode"), DocNumber: r.FormValue("culDoc")}
	out, err := s.Documents.Upload(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "cid"), r.FormValue("typeId"),
		hdr.Filename, f, hint, r.FormValue("today"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, verdictOut(out))
}

func (s *Server) docVerify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Approve bool   `json:"approve"`
		Reason  string `json:"reason"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if err := s.Documents.Verify(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "cid"), chi.URLParam(r, "did"), in.Approve, in.Reason); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) docRecheck(w http.ResponseWriter, r *http.Request) {
	var in app.CULHint
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	out, err := s.Documents.Recheck(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "cid"), chi.URLParam(r, "did"), in)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, verdictOut(out))
}

func verdictOut(v *ports.DocVerdict) map[string]any {
	return map[string]any{"status": v.Status, "method": v.Method, "reason": v.Reason}
}
