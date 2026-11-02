package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// FeatureService: catálogo de funcionalidades. El Super Admin las suspende globalmente o por tenant y las activa por
// tenant; la API consulta qué tiene habilitado cada tenant (con caché corto) para permitir cada módulo.
// Avisa al Super Admin de las funcionalidades nuevas (llegan con un despliegue) y, al activar una en un tenant, a sus
// usuarios con los roles impactados.
type FeatureService struct {
	Repo     ports.FeatureRepository
	Notifier *NotificationService

	mu    sync.Mutex
	cache map[uint64]featureSet
}

type featureSet struct {
	codes map[string]bool
	at    time.Time
}

const featureCacheTTL = 30 * time.Second

var featureCodeRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)

// Enabled: funcionalidades habilitadas para el tenant (caché de 30 s; se invalida al cambiar).
func (s *FeatureService) Enabled(ctx context.Context, tenantID uint64) (map[string]bool, error) {
	s.mu.Lock()
	if c, ok := s.cache[tenantID]; ok && time.Since(c.at) < featureCacheTTL {
		s.mu.Unlock()
		return c.codes, nil
	}
	s.mu.Unlock()
	codes, err := s.Repo.TenantFeatures(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(codes))
	for _, c := range codes {
		set[c] = true
	}
	s.mu.Lock()
	if s.cache == nil {
		s.cache = map[uint64]featureSet{}
	}
	s.cache[tenantID] = featureSet{set, time.Now()}
	s.mu.Unlock()
	return set, nil
}

// Allowed: el Super Admin no depende del catálogo; un usuario de tenant, si su tenant la tiene habilitada.
func (s *FeatureService) Allowed(ctx context.Context, p *domain.Principal, code string) (bool, error) {
	if p == nil || p.IsPlatform() {
		return true, nil
	}
	set, err := s.Enabled(ctx, *p.TenantID)
	if err != nil {
		return false, err
	}
	return set[code], nil
}

// List: el catálogo con cuántos tenants tienen cada funcionalidad.
func (s *FeatureService) List(ctx context.Context, p *domain.Principal) ([]json.RawMessage, error) {
	return s.Repo.List(ctx, meta(p, ""))
}

func (s *FeatureService) Tenants(ctx context.Context, p *domain.Principal, code string) ([]json.RawMessage, error) {
	if !featureCodeRe.MatchString(code) {
		return nil, domain.ErrNotFound
	}
	return s.Repo.Tenants(ctx, meta(p, ""), code)
}

// SetStatus suspende o reactiva una funcionalidad para todos los tenants.
func (s *FeatureService) SetStatus(ctx context.Context, p *domain.Principal, ip, code, status string) error {
	if !featureCodeRe.MatchString(code) || (status != "ACTIVE" && status != "SUSPENDED") {
		return domain.ErrValidation
	}
	if err := s.Repo.SetStatus(ctx, meta(p, ip), code, status); err != nil {
		return err
	}
	s.invalidate(0)
	return nil
}

// SetTenant activa o suspende una funcionalidad en un tenant; si queda recién habilitada, avisa a los usuarios del
// tenant con los roles impactados.
func (s *FeatureService) SetTenant(ctx context.Context, p *domain.Principal, ip, tenantID, code string, enabled bool) error {
	if !validUUID(tenantID) || !featureCodeRe.MatchString(code) {
		return domain.ErrValidation
	}
	ch, err := s.Repo.SetTenant(ctx, meta(p, ip), tenantID, code, enabled)
	if err != nil {
		return err
	}
	s.invalidate(ch.TenantID)
	if ch.NewlyEnabled && ch.Status == "ACTIVE" && len(ch.Roles) > 0 {
		payload, _ := json.Marshal(map[string]any{"title": ch.Name, "description": ch.Description, "module": ch.Module, "code": code})
		s.notify(ctx, p, ch.TenantID, domain.NotifFeatureEnabled, code, payload, ports.AudienceTenantRoles, ch.Roles, func(locale string) pushMessage {
			t, h := ch.Name["es"], "Nueva funcionalidad disponible"
			if len(locale) >= 2 && locale[:2] == "en" {
				t, h = ch.Name["en"], "New feature available"
			}
			return pushMessage{Title: h, Body: t, URL: "/novedades", Tag: "feature-" + code}
		})
	}
	return nil
}

// AnnounceNew avisa al Super Admin de las funcionalidades que llegaron con el despliegue (se llama al iniciar la API).
func (s *FeatureService) AnnounceNew(ctx context.Context) {
	news, err := s.Repo.Unnoticed(ctx)
	if err != nil {
		slog.Warn("no se pudieron revisar las funcionalidades nuevas", "err", err)
		return
	}
	for _, f := range news {
		// con varias instancias, solo la que la marca avisa
		if ok, err := s.Repo.Noticed(ctx, f.Code); err != nil || !ok {
			continue
		}
		payload, _ := json.Marshal(map[string]any{"title": f.Name, "description": f.Description, "module": f.Module, "code": f.Code})
		s.notify(ctx, nil, 0, domain.NotifFeatureNew, f.Code, payload, ports.AudienceSuperAdmins, nil, func(locale string) pushMessage {
			t, h := f.Name["es"], "Funcionalidad nueva por activar"
			if len(locale) >= 2 && locale[:2] == "en" {
				t, h = f.Name["en"], "New feature to activate"
			}
			return pushMessage{Title: h, Body: t, URL: "/admin/funcionalidades", Tag: "feature-new-" + f.Code}
		})
		slog.Info("funcionalidad nueva anunciada al Super Admin", "code", f.Code)
	}
}

func (s *FeatureService) invalidate(tenantID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tenantID == 0 {
		s.cache = nil
		return
	}
	delete(s.cache, tenantID)
}

func (s *FeatureService) notify(ctx context.Context, p *domain.Principal, tid uint64, typ, code string, payload json.RawMessage, audience string, roles []string, push func(locale string) pushMessage) {
	n := s.Notifier
	if n == nil {
		return
	}
	m := ports.Meta{}
	if p != nil {
		m = meta(p, "")
	}
	in := ports.NotifyInput{ID: uuid.NewString(), Type: typ, EntityType: "FEATURE", EntityID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("feature:"+code)).String(), Payload: payload}
	res, err := n.Repo.NotifyUsers(context.WithoutCancel(ctx), m, tid, in, audience, 0, roles)
	if err != nil {
		slog.Warn("no se pudo registrar la notificación", "type", typ, "err", err)
		return
	}
	if len(res.Targets) == 0 || n.Push == nil || !n.Push.Enabled() {
		return
	}
	go n.deliver(res.ID, res.Targets, func(locale string) pushMessage { msg := push(locale); msg.ID = res.ID; return msg })
}
