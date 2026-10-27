package httpapi

import (
	_ "embed"
	"net/http"
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
