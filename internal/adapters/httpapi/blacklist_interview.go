package httpapi

import (
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

func (s *Server) blacklistRoutes(r chi.Router) {
	read := s.requirePerm("talent.read")
	manage := s.requirePerm("blacklist.manage")
	r.With(read).Post("/blacklist/search", s.blacklistSearch)
	r.With(read).Get("/talents/{id}/blacklist", s.blacklistStatus)
	r.With(read).Get("/talents/{id}/blacklist/history", s.blacklistHistory)
	r.With(manage).Post("/blacklist", s.blacklistCreate)
	r.With(manage).Patch("/blacklist/{id}", s.blacklistUpdate)
	r.With(manage).Post("/blacklist/{id}/remove", s.blacklistRemove)
	r.With(manage).Post("/blacklist/{id}/lift-global", s.blacklistLiftGlobal)
}

func (s *Server) interviewRoutes(r chi.Router) {
	read := s.requirePerm("interview.read")
	write := s.requirePerm("interview.write")
	r.Use(s.idempotent)
	r.With(read).Post("/interviews/search", s.interviewSearch)
	r.With(write).Post("/interviews", s.interviewSave)
	r.With(read).Get("/interviews/files/{fid}", s.interviewFileDownload)
	r.With(write).Delete("/interviews/files/{fid}", s.interviewFileDelete)
	r.With(read).Get("/interviews/{id}", s.interviewGet)
	r.With(write).Put("/interviews/{id}", s.interviewSave)
	r.With(read).Get("/interviews/{id}/ics", s.interviewICS)
	r.With(write).Post("/interviews/{id}/files", s.interviewFileUpload)
}

// ---------------------------------------------------------------- Lista negra

func (s *Server) blacklistSearch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Search     string `json:"search"`
		ClientID   string `json:"clientId"`
		GlobalOnly bool   `json:"globalOnly"`
		Page       int    `json:"page"`
		PageSize   int    `json:"pageSize"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	page, err := s.Blacklist.Search(r.Context(), principal(r), in.Search, in.ClientID, in.GlobalOnly, in.Page, in.PageSize)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) entriesOut(w http.ResponseWriter, r *http.Request, list []domain.BlacklistEntry, err error, status int) {
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, status, list)
}

func (s *Server) blacklistStatus(w http.ResponseWriter, r *http.Request) {
	list, err := s.Blacklist.Status(r.Context(), principal(r), chi.URLParam(r, "id"))
	s.entriesOut(w, r, list, err, http.StatusOK)
}

func (s *Server) blacklistHistory(w http.ResponseWriter, r *http.Request) {
	page, err := s.Blacklist.History(r.Context(), principal(r), chi.URLParam(r, "id"), qInt(r, "page", 1), qInt(r, "pageSize", 20))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) blacklistCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID       string `json:"id"`
		TalentID string `json:"talentId"`
		ClientID string `json:"clientId"` // vacío = global (todos los clientes)
		Reason   string `json:"reason"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	list, err := s.Blacklist.Create(r.Context(), principal(r), s.clientIP(r), in.ID, in.TalentID, in.ClientID, in.Reason)
	s.entriesOut(w, r, list, err, http.StatusCreated)
}

type reasonIn struct {
	Reason string `json:"reason"`
}

func (s *Server) blacklistUpdate(w http.ResponseWriter, r *http.Request) {
	var in reasonIn
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	list, err := s.Blacklist.Update(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), in.Reason)
	s.entriesOut(w, r, list, err, http.StatusOK)
}

func (s *Server) blacklistRemove(w http.ResponseWriter, r *http.Request) {
	var in reasonIn
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	list, err := s.Blacklist.Remove(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), in.Reason)
	s.entriesOut(w, r, list, err, http.StatusOK)
}

func (s *Server) blacklistLiftGlobal(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string                `json:"reason"`
		Keep   []ports.BlacklistKeep `json:"keep"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	list, err := s.Blacklist.LiftGlobal(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), in.Reason, in.Keep)
	s.entriesOut(w, r, list, err, http.StatusOK)
}

// ---------------------------------------------------------------- Entrevistas

func (s *Server) interviewSearch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Search   string `json:"search"`
		StatusID string `json:"statusId"`
		StageID  string `json:"stageId"`
		TypeID   string `json:"typeId"`
		ClientID string `json:"clientId"`
		Date     string `json:"date"`
		TalentID string `json:"talentId"`
		Page     int    `json:"page"`
		PageSize int    `json:"pageSize"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	page, err := s.Interviews.Search(r.Context(), principal(r), ports.InterviewFilter{Search: in.Search, StatusID: in.StatusID,
		StageID: in.StageID, TypeID: in.TypeID, ClientID: in.ClientID, Date: in.Date, TalentID: in.TalentID, Page: in.Page, PageSize: in.PageSize})
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) interviewGet(w http.ResponseWriter, r *http.Request) {
	iv, err := s.Interviews.Get(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, iv)
}

func (s *Server) interviewSave(w http.ResponseWriter, r *http.Request) {
	var in app.InterviewInput
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if id := chi.URLParam(r, "id"); id != "" {
		in.ID = id
	}
	res, err := s.Interviews.Save(r.Context(), principal(r), s.clientIP(r), in)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	status := http.StatusOK
	if res.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, res)
}

func (s *Server) interviewICS(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	ics, err := s.Interviews.ICS(r.Context(), p, chi.URLParam(r, "id"), p.Locale)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/calendar; charset=utf-8")
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "entrevista-" + chi.URLParam(r, "id") + ".ics"}))
	_, _ = w.Write([]byte(ics))
}

func (s *Server) interviewFileUpload(w http.ResponseWriter, r *http.Request) {
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
	out, err := s.Interviews.UploadFile(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), r.URL.Query().Get("typeId"), hdr.Filename, f)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) interviewFileDownload(w http.ResponseWriter, r *http.Request) {
	f, data, err := s.Interviews.OpenFile(r.Context(), principal(r), chi.URLParam(r, "fid"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	disp := "attachment"
	if r.URL.Query().Get("inline") == "1" && (f.MimeType == "application/pdf" || strings.HasPrefix(f.MimeType, "image/")) {
		disp = "inline"
	}
	h := w.Header()
	h.Set("Content-Type", f.MimeType)
	h.Set("Content-Length", strconv.Itoa(len(data)))
	h.Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": f.OriginalName}))
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "private, no-store")
	h.Set("ETag", fmt.Sprintf(`"%s"`, f.SHA256))
	_, _ = bytes.NewReader(data).WriteTo(w)
}

func (s *Server) interviewFileDelete(w http.ResponseWriter, r *http.Request) {
	s.noContent(w, r, s.Interviews.DeleteFile(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "fid")))
}
