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
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// Soporte, novedades y guías: para cualquier usuario autenticado (de un tenant o el Super Admin).
// Cada usuario ve sus solicitudes; el Super Admin (platform.support.manage) ve y atiende todas.
func (s *Server) supportRoutes(r chi.Router) {
	r.Use(s.idempotent)
	r.Post("/support/files", s.supportUpload)
	r.Get("/support/files/{fid}", s.supportFile)
	r.Post("/support", s.supportCreate)
	r.Get("/support", s.supportList)
	r.Get("/support/{id}", s.supportGet)
	r.Post("/support/{id}/messages", s.supportReply)
	r.Get("/releases", s.releaseList)
	r.With(s.requirePerm("platform.releases.manage")).Post("/releases", s.releasePublish)
	r.Get("/me/tours", s.tourList)
	r.Put("/me/tours/{key}", s.tourSeen)
}

func (s *Server) supportUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	if err := r.ParseMultipartForm(6 << 20); err != nil {
		s.writeErr(w, r, domain.NewError("FILE_TOO_LARGE"))
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		s.writeErr(w, r, domain.ErrValidation)
		return
	}
	defer f.Close()
	id, err := s.Support.UploadFile(r.Context(), principal(r), s.clientIP(r), hdr.Filename, f)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) supportFile(w http.ResponseWriter, r *http.Request) {
	f, data, err := s.Support.OpenFile(r.Context(), principal(r), chi.URLParam(r, "fid"))
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

func (s *Server) supportCreate(w http.ResponseWriter, r *http.Request) {
	var in app.SupportInput
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	id, number, err := s.Support.Create(r.Context(), principal(r), s.clientIP(r), in)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "number": number})
}

func (s *Server) supportList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out, err := s.Support.List(r.Context(), principal(r), q.Get("status"), q.Get("q"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) supportGet(w http.ResponseWriter, r *http.Request) {
	out, err := s.Support.Get(r.Context(), principal(r), chi.URLParam(r, "id"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) supportReply(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Body   string   `json:"body"`
		Status string   `json:"status"`
		Files  []string `json:"files"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	out, err := s.Support.Reply(r.Context(), principal(r), s.clientIP(r), chi.URLParam(r, "id"), in.Body, in.Status, in.Files)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": out.Status})
}

func (s *Server) releaseList(w http.ResponseWriter, r *http.Request) {
	out, err := s.Support.Releases(r.Context(), principal(r))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) releasePublish(w http.ResponseWriter, r *http.Request) {
	var in app.ReleaseInput
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	id, err := s.Support.PublishRelease(r.Context(), principal(r), s.clientIP(r), in)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) tourList(w http.ResponseWriter, r *http.Request) {
	out, err := s.Support.Tours(r.Context(), principal(r))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) tourSeen(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version int `json:"version"`
	}
	if err := decode(w, r, &in); err != nil {
		s.writeErr(w, r, err)
		return
	}
	if err := s.Support.TourSeen(r.Context(), principal(r), chi.URLParam(r, "key"), in.Version); err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
