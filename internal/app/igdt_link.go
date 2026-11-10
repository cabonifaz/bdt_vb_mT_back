package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/tokens"
)

// Llave BDT automática: con la integración activa, IGDT necesita leer de BDT la ficha, los CV y las postulaciones de los
// candidatos. Al conectar (y en «Sincronizar ahora» o la sincronización periódica mientras falte) BDT crea una API key
// del tenant con alcances talent.read y talent.search (búsqueda en todo el banco, v1.9), ya habilitada, y se la entrega a IGDT
// (PUT /integration/v1/bdt-link).
//   - Si la entrega falla, la key se descarta (revocada) y se reintenta más tarde; el error queda en la integración.
//   - Un IGDT sin el endpoint (404) no es un error: se reintenta en silencio cada cierto tiempo.
//   - Al desconectar se retira de IGDT (sin esperar a que responda bien) y la key queda revocada.

const (
	igdtLinkKeyName   = "IGDT (automática)"
	igdtLinkRetry     = 5 * time.Minute  // tras un fallo de IGDT
	igdtLinkRetryGone = 30 * time.Minute // IGDT aún sin el endpoint
)

// IGDTNotifier avisa de los ingresos revocados en IGDT (a los responsables del RQ y a quienes pueden reabrirlo).
type IGDTNotifier interface {
	HireRevoked(ctx context.Context, tenantID uint64, tenantSlug string, h ports.IGDTRevokedHire)
}

// ensureLink entrega a IGDT la llave BDT del tenant si aún no la tiene. force ignora la espera entre reintentos
// («Sincronizar ahora» y la conexión). Nunca hace fallar la sincronización.
func (s *IGDTService) ensureLink(ctx context.Context, t ports.IGDTTarget, key string, force bool) {
	if len(s.Pepper) == 0 || s.Client == nil {
		return
	}
	s.mu.Lock()
	if s.linking == nil {
		s.linking = map[uint64]bool{}
		s.linkWait = map[uint64]time.Time{}
	}
	if s.linking[t.TenantID] || (!force && time.Now().Before(s.linkWait[t.TenantID])) {
		s.mu.Unlock()
		return
	}
	s.linking[t.TenantID] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.linking, t.TenantID)
		s.mu.Unlock()
	}()
	retry := func(d time.Duration) {
		s.mu.Lock()
		s.linkWait[t.TenantID] = time.Now().Add(d)
		s.mu.Unlock()
	}

	prefix, secret := newApiKeySecret()
	id := uuid.NewString()
	created, err := s.Repo.LinkKeyCreate(ctx, t.TenantID, id, igdtLinkKeyName, prefix, tokens.Keyed(s.Pepper, secret))
	if err != nil {
		slog.Warn("igdt: no se pudo crear la llave BDT", "tenant", t.Slug, "err", err)
		retry(igdtLinkRetry)
		return
	}
	if !created { // otra instancia ya la creó, o la integración ya no está activa
		return
	}
	err = s.Client.LinkBDT(ctx, key, secret)
	if err == nil {
		slog.Info("igdt: llave BDT entregada a IGDT", "tenant", t.Slug, "prefix", prefix)
		return
	}
	code := domain.AsError(err).Code
	bg := context.WithoutCancel(ctx)
	if derr := s.Repo.LinkKeyDrop(bg, t.TenantID, id, "IGDT_LINK_FAILED"); derr != nil {
		slog.Warn("igdt: no se pudo descartar la llave BDT no entregada", "tenant", t.Slug, "err", derr)
	}
	if code == "IGDT_LINK_UNSUPPORTED" {
		retry(igdtLinkRetryGone)
		return
	}
	retry(igdtLinkRetry)
	_ = s.Repo.SyncError(bg, t.TenantID, "BDT_LINK: "+code)
	slog.Warn("igdt: no se pudo entregar la llave BDT", "tenant", t.Slug, "err", code)
}

// unlink retira de IGDT la llave BDT antes de desconectar (mejor esfuerzo: la key se revoca igual en BDT).
func (s *IGDTService) unlink(ctx context.Context, tid uint64) {
	if s.Client == nil || !s.Client.Enabled() {
		return
	}
	targets, err := s.Repo.Targets(ctx, &tid)
	if err != nil || len(targets) == 0 {
		return
	}
	key, err := s.Cipher.Decrypt(targets[0].KeyEnc)
	if err != nil || key == "" {
		return
	}
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := s.Client.UnlinkBDT(c, key); err != nil && domain.AsError(err).Code != "IGDT_LINK_UNSUPPORTED" {
		slog.Warn("igdt: no se pudo retirar la llave BDT de IGDT", "tenant", targets[0].Slug, "err", domain.AsError(err).Code)
	}
	s.mu.Lock()
	delete(s.linkWait, tid)
	s.mu.Unlock()
}
