package httpapi

import (
	"bytes"
	_ "embed"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"
)

//go:embed openapi/integration.json
var integrationSpec []byte

//go:embed openapi/docs.html
var docsPage []byte

//go:embed openapi/docs.js
var docsScript []byte

// integrationDocs sirve una página estática sin datos: pide la API key y con ella
// descarga la especificación (que también exige key). Swagger UI se carga desde CDN con versión fijada.
func (s *Server) integrationDocs(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self' https://cdn.jsdelivr.net; "+
		"style-src 'self' https://cdn.jsdelivr.net 'unsafe-inline'; img-src 'self' data: https://cdn.jsdelivr.net; "+
		"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	h.Set("Cache-Control", "no-cache")
	_, _ = w.Write(docsPage)
}

func (s *Server) integrationDocsJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(docsScript)
}

func (s *Server) openAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(integrationSpec)
}

func (s *Server) integrationWhoAmI(w http.ResponseWriter, r *http.Request) {
	c := apiClient(r)
	writeJSON(w, http.StatusOK, map[string]any{"tenant": c.TenantSlug, "keyId": c.KeyUUID, "scopes": c.Scopes})
}

func (s *Server) integrationMasters(w http.ResponseWriter, r *http.Request) {
	c := apiClient(r)
	tid := c.TenantID
	items, err := s.Masters.Items(r.Context(), &tid, r.URL.Query().Get("tables"), false)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /integration/v1/talents/{id}: ficha del candidato (scope talent.read).
func (s *Server) integrationTalent(w http.ResponseWriter, r *http.Request) {
	doc, err := s.Talents.IntegrationDossier(r.Context(), apiClient(r), s.clientIP(r), chi.URLParam(r, "id"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(doc)
}

// GET /integration/v1/talents/{id}/files/{fid}: CV, documento o certificado del candidato (scope talent.read).
func (s *Server) integrationTalentFile(w http.ResponseWriter, r *http.Request) {
	f, data, err := s.Talents.IntegrationFile(r.Context(), apiClient(r), s.clientIP(r), chi.URLParam(r, "id"), chi.URLParam(r, "fid"))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	ct := f.MimeType
	if ct == "" {
		ct = "application/octet-stream"
	}
	disp := mime.FormatMediaType("inline", map[string]string{"filename": safeFileName(f.OriginalName, f.UUID, f.MimeType)})
	if disp == "" {
		disp = "inline"
	}
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("Content-Length", strconv.Itoa(len(data)))
	h.Set("Content-Disposition", disp)
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	if f.SHA256 != "" {
		h.Set("ETag", fmt.Sprintf(`"%s"`, f.SHA256))
	}
	_, _ = bytes.NewReader(data).WriteTo(w)
}

var mimeExt = map[string]string{"application/pdf": ".pdf", "application/msword": ".doc",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx"}

// safeFileName: nombre sin rutas, comillas ni caracteres de control (máx. 150); si queda vacío, cv-<id>.<ext>.
func safeFileName(name, id, mimeType string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune("\\\"/:*?<>|;", r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if rs := []rune(name); len(rs) > 150 {
		name = string(rs[:150])
	}
	if name == "" || name == "." || name == "/" {
		name = "cv-" + id + mimeExt[mimeType]
	}
	return name
}

// GET /integration/v1/talents/{id}/cv-source?lang=es|en: insumos para generar el CV con la marca del tenant
// (scope talent.read).
func (s *Server) integrationTalentCVSource(w http.ResponseWriter, r *http.Request) {
	out, err := s.Talents.IntegrationCVSource(r.Context(), apiClient(r), s.clientIP(r), chi.URLParam(r, "id"),
		strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lang"))))
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}
