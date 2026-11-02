package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// VersionService: versión publicada en el ambiente. Al iniciar la API la registra (si es nueva, avisa a todos los
// usuarios); la versión vigente es pública (se muestra junto al logo). El Super Admin ve el historial, marca el estado
// (en pruebas, candidata, estable, retirada) y compara qué versión tiene cada ambiente.
type VersionService struct {
	Repo     ports.VersionRepository
	Notifier *NotificationService
	Env      string            // staging | preprod | production | local
	Version  string            // de VERSION, p. ej. "1.4"
	Commit   string            // commit desplegado
	Notes    []string          // notas de la versión (CHANGELOG)
	Peers    map[string]string // ambiente -> URL base de su API, para comparar versiones
	HTTP     *http.Client
}

var (
	versionStatuses = map[string]bool{"TESTING": true, "CANDIDATE": true, "STABLE": true, "WITHDRAWN": true}
	versionRe       = regexp.MustCompile(`^\d{1,3}\.\d{1,4}$`)
)

// defaultStatus: en prod se publica estable; en preprod, candidata; en staging, en pruebas.
func (s *VersionService) defaultStatus() string {
	switch s.Env {
	case "production", "prod":
		return "STABLE"
	case "preprod":
		return "CANDIDATE"
	}
	return "TESTING"
}

// Register: registra la versión del despliegue y, si es nueva en el ambiente, notifica a todos. En local no registra.
func (s *VersionService) Register(ctx context.Context) {
	if s.Env == "local" || s.Env == "" || !versionRe.MatchString(s.Version) {
		return
	}
	notes, _ := json.Marshal(s.Notes)
	isNew, err := s.Repo.Register(ctx, s.Version, s.Env, s.defaultStatus(), s.Commit, notes)
	if err != nil {
		slog.Warn("no se pudo registrar la versión", "version", s.Version, "err", err)
		return
	}
	if !isNew || s.Notifier == nil {
		return
	}
	slog.Info("versión nueva publicada", "version", s.Version, "env", s.Env)
	payload, _ := json.Marshal(map[string]any{"version": s.Version, "environment": s.Env, "status": s.defaultStatus(), "title": "v" + s.Version})
	n := s.Notifier
	in := ports.NotifyInput{ID: uuid.NewString(), Type: domain.NotifVersionPublished, EntityType: "VERSION",
		EntityID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("version:"+s.Env+":"+s.Version)).String(), Payload: payload}
	res, err := n.Repo.NotifyUsers(context.WithoutCancel(ctx), ports.Meta{}, 0, in, ports.AudienceAll, 0, nil)
	if err != nil {
		slog.Warn("no se pudo notificar la versión", "err", err)
		return
	}
	if len(res.Targets) == 0 || n.Push == nil || !n.Push.Enabled() {
		return
	}
	go n.deliver(res.ID, res.Targets, func(locale string) pushMessage {
		h, b := "Nueva versión publicada", "Ya está disponible la v"+s.Version+". Recarga la app para usarla."
		if strings.HasPrefix(locale, "en") {
			h, b = "New version released", "Version v"+s.Version+" is available. Reload the app to use it."
		}
		return pushMessage{Title: h, Body: b, URL: "/versiones", Tag: "version-" + s.Version, ID: res.ID}
	})
}

// Current: versión vigente del ambiente (pública). En local, la del binario como "en pruebas".
func (s *VersionService) Current(ctx context.Context) (json.RawMessage, error) {
	doc, err := s.Repo.Current(ctx, s.Env, s.Version)
	if err == nil && len(doc) > 0 {
		return doc, nil
	}
	notes, _ := json.Marshal(s.Notes)
	out, _ := json.Marshal(map[string]any{"version": s.Version, "environment": s.Env, "status": "TESTING", "commit": s.Commit, "notes": json.RawMessage(notes)})
	return out, nil
}

func (s *VersionService) List(ctx context.Context) ([]json.RawMessage, error) {
	return s.Repo.List(ctx, s.Env)
}

func (s *VersionService) SetStatus(ctx context.Context, p *domain.Principal, ip, version, status string) error {
	if !versionRe.MatchString(version) || !versionStatuses[status] {
		return domain.ErrValidation
	}
	return s.Repo.SetStatus(ctx, meta(p, ip), s.Env, version, status)
}

// Environments: versión vigente de cada ambiente (consulta la API pública de los demás).
func (s *VersionService) Environments(ctx context.Context) []json.RawMessage {
	own, _ := s.Current(ctx)
	out := []json.RawMessage{withEnv(own, s.Env, true, "")}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for env, base := range s.Peers {
		if env == s.Env || base == "" {
			continue
		}
		wg.Add(1)
		go func(env, base string) {
			defer wg.Done()
			doc, err := s.fetch(ctx, base)
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			mu.Lock()
			out = append(out, withEnv(doc, env, false, msg))
			mu.Unlock()
		}(env, base)
	}
	wg.Wait()
	return out
}

func (s *VersionService) fetch(ctx context.Context, base string) (json.RawMessage, error) {
	c := s.HTTP
	if c == nil {
		c = &http.Client{Timeout: 6 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/api/v1/version", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, domain.NewError("PEER_UNAVAILABLE")
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil || !json.Valid(b) {
		return nil, domain.NewError("PEER_UNAVAILABLE")
	}
	return b, nil
}

// withEnv agrega al documento el ambiente, si es el actual y el error de consulta (si lo hubo).
func withEnv(doc json.RawMessage, env string, current bool, errMsg string) json.RawMessage {
	m := map[string]any{}
	_ = json.Unmarshal(doc, &m)
	m["environment"], m["current"] = env, current
	if errMsg != "" {
		m["error"] = "PEER_UNAVAILABLE"
	}
	out, _ := json.Marshal(m)
	return out
}

// ParsePeers: "staging=https://...,preprod=https://...,production=https://..." (ENV_URLS).
func ParsePeers(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok && strings.HasPrefix(v, "https://") {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}
