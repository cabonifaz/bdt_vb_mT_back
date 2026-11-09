package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// IGDTService: conexión del tenant con IGDT, que gobierna los catálogos compartidos (clientes y contactos, perfiles,
// tarifario, modalidades, monedas, plazos, áreas y motivos).
//   - Sin integración, el admin administra esos maestros en BDT.
//   - Al conectar: se verifica el token (tenant y alcances), se envían a IGDT los catálogos que BDT ya tenía (carga
//     inicial, conservando ids), se guarda el token cifrado y se sincroniza. Desde ahí esos catálogos son de solo
//     lectura en BDT y se actualizan cada pocos minutos (y con «Sincronizar ahora»).
type IGDTService struct {
	Repo   ports.IGDTRepository
	Client ports.IGDTClient
	Pusher ports.IGDTPusher
	Cipher ports.Cipher

	mu    sync.Mutex
	cache map[uint64]igdtActive
	kick  chan struct{}
}

type igdtActive struct {
	on bool
	at time.Time
}

const igdtActiveTTL = 30 * time.Second

var igdtKeyRe = regexp.MustCompile(`^igdt_[A-Za-z0-9]{12}_[A-Za-z0-9_-]{16,128}$`)

// Active: ¿el tenant tiene la integración activa? (cacheado; lo usa /auth/me para bloquear la edición en la app).
func (s *IGDTService) Active(ctx context.Context, tenantID uint64) bool {
	s.mu.Lock()
	if c, ok := s.cache[tenantID]; ok && time.Since(c.at) < igdtActiveTTL {
		s.mu.Unlock()
		return c.on
	}
	s.mu.Unlock()
	on, err := s.Repo.Active(ctx, tenantID)
	if err != nil {
		return false
	}
	s.mu.Lock()
	if s.cache == nil {
		s.cache = map[uint64]igdtActive{}
	}
	s.cache[tenantID] = igdtActive{on, time.Now()}
	s.mu.Unlock()
	return on
}

func (s *IGDTService) forget(tenantID uint64) {
	s.mu.Lock()
	delete(s.cache, tenantID)
	s.mu.Unlock()
}

func (s *IGDTService) Status(ctx context.Context, p *domain.Principal) (json.RawMessage, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	doc, err := s.Repo.Status(ctx, meta(p, ""), tid)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if json.Unmarshal(doc, &m) == nil {
		m["available"] = s.Client != nil && s.Client.Enabled()
		if ob, err := s.Repo.OutboxStatus(ctx, tid); err == nil && ob != nil {
			m["selection"] = ob
		}
		doc, _ = json.Marshal(m)
	}
	return doc, nil
}

// Connect verifica el token, hace la carga inicial y deja la integración activa y sincronizada.
func (s *IGDTService) Connect(ctx context.Context, p *domain.Principal, ip, key string) (json.RawMessage, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !igdtKeyRe.MatchString(key) {
		return nil, domain.NewError("IGDT_KEY_INVALID")
	}
	if s.Client == nil || !s.Client.Enabled() {
		return nil, domain.NewError("IGDT_NOT_CONFIGURED")
	}
	who, err := s.Client.WhoAmI(ctx, key)
	if err != nil {
		return nil, err
	}
	// el token debe ser de la misma empresa en IGDT (evita cruzar catálogos entre empresas)
	if who.Tenant != p.TenantSlug {
		return nil, domain.NewError("IGDT_TENANT_MISMATCH")
	}
	if !slices.Contains(who.Scopes, "masters.read") || !slices.Contains(who.Scopes, "catalog.import") {
		return nil, domain.NewError("IGDT_SCOPE_MISSING")
	}
	export, err := s.Repo.Export(ctx, tid)
	if err != nil {
		return nil, err
	}
	mapping, err := s.Client.Import(ctx, key, export)
	if err != nil {
		return nil, err
	}
	enc, err := s.Cipher.Encrypt(key)
	if err != nil {
		return nil, err
	}
	if err := s.Repo.Save(ctx, meta(p, ip), tid, enc, who.Tenant); err != nil {
		return nil, err
	}
	s.forget(tid)
	if err := s.Repo.ApplyMap(ctx, tid, mapping); err != nil {
		return nil, err
	}
	if err := s.syncTenant(ctx, tid); err != nil {
		slog.Warn("igdt: la primera sincronización falló", "tenant", tid, "err", err)
	}
	s.resendInBackground(tid)
	return s.Status(ctx, p)
}

func (s *IGDTService) Disconnect(ctx context.Context, p *domain.Principal, ip string) error {
	tid, err := tenantOf(p)
	if err != nil {
		return err
	}
	err = s.Repo.Disable(ctx, meta(p, ip), tid)
	s.forget(tid)
	return err
}

// SyncNow: sincroniza el tenant del usuario en el momento.
func (s *IGDTService) SyncNow(ctx context.Context, p *domain.Principal) (json.RawMessage, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if _, err := s.Repo.Status(ctx, meta(p, ""), tid); err != nil { // exige el permiso
		return nil, err
	}
	if err := s.syncTenant(ctx, tid); err != nil {
		return nil, err
	}
	s.resendInBackground(tid)
	return s.Status(ctx, p)
}

func (s *IGDTService) syncTenant(ctx context.Context, tid uint64) error {
	targets, err := s.Repo.Targets(ctx, &tid)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return domain.NewError("IGDT_NOT_CONNECTED")
	}
	return s.sync(ctx, targets[0])
}

// sync trae las páginas de cambios desde el cursor y las aplica (cada página avanza el cursor).
func (s *IGDTService) sync(ctx context.Context, t ports.IGDTTarget) error {
	key, err := s.Cipher.Decrypt(t.KeyEnc)
	if err != nil || key == "" {
		return domain.NewError("IGDT_KEY_INVALID")
	}
	cursor := t.Cursor
	for page := 0; page < 50; page++ {
		res, err := s.Client.Changes(ctx, key, cursor, 500)
		if err != nil {
			_ = s.Repo.SyncError(context.WithoutCancel(ctx), t.TenantID, domain.AsError(err).Code)
			return err
		}
		if _, err := s.Repo.ApplyChanges(ctx, t.TenantID, res.Changes, res.Next); err != nil {
			_ = s.Repo.SyncError(context.WithoutCancel(ctx), t.TenantID, "APPLY_FAILED")
			return err
		}
		cursor = res.Next
		if !res.More {
			return nil
		}
	}
	return nil
}

// Watch sincroniza todos los tenants conectados cada `every` (y una vez al minuto de iniciar).
func (s *IGDTService) Watch(ctx context.Context, every time.Duration) {
	if s.Client == nil || !s.Client.Enabled() {
		return
	}
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		targets, err := s.Repo.Targets(ctx, nil)
		if err != nil {
			slog.Warn("igdt: no se pudo leer los tenants conectados", "err", err)
		}
		for _, t := range targets {
			if err := s.sync(ctx, t); err != nil && !errors.Is(err, context.Canceled) {
				slog.Warn("igdt: sincronización fallida", "tenant", t.Slug, "err", domain.AsError(err).Code)
			}
		}
		timer.Reset(every)
	}
}

// TariffLookup: tarifa sugerida (del tarifario de IGDT) para un perfil y cliente en una fecha (AAAA-MM-DD).
func (s *IGDTService) TariffLookup(ctx context.Context, p *domain.Principal, profileID, clientID, date string) (json.RawMessage, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if !validUUID(profileID) || (clientID != "" && !validUUID(clientID)) {
		return nil, domain.ErrValidation
	}
	if date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return nil, domain.ErrValidation
		}
	}
	var c *string
	if clientID != "" {
		c = &clientID
	}
	return s.Repo.TariffLookup(ctx, meta(p, ""), tid, profileID, c, date)
}

// resendInBackground reenvía a IGDT todos los RQ y candidatos confirmados del tenant (bandeja de selección).
func (s *IGDTService) resendInBackground(tid uint64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := s.resendAll(ctx, tid); err != nil {
			slog.Warn("igdt: reenvío de la bandeja de selección fallido", "tenant", tid, "err", domain.AsError(err).Code)
		}
	}()
}
