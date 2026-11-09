package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// Bandeja de selección de IGDT: con la integración activa, BDT publica en IGDT el último estado de cada RQ (con el
// avance de sus postulantes) y el estado de los candidatos confirmados, que allá se vuelven solicitudes de ingreso.
//   - Cada cambio de RQ o candidato hecho en BDT encola el envío (igdt_outbox) y despierta al proceso de envío.
//   - La conciliación periódica encola lo que cambió por cualquier otra vía; «Sincronizar ahora» y la conexión
//     reenvían todo (RQ abiertos y cerrados recientes).
//   - Los envíos fallidos se reintentan con espera creciente; IGDT los aplica de forma idempotente.

const (
	igdtPushBatch     = 50
	igdtReconcileEach = 5 * time.Minute
)

// RQChanged / CandidateChanged: encolan el envío (no hacen fallar la operación del usuario).
func (s *IGDTService) RQChanged(ctx context.Context, tenantID uint64, rqID string) {
	if !s.Active(ctx, tenantID) {
		return
	}
	if err := s.Repo.OutboxRQ(context.WithoutCancel(ctx), tenantID, rqID); err != nil {
		slog.Warn("igdt: no se pudo encolar el RQ", "err", err)
		return
	}
	s.wake()
}

func (s *IGDTService) CandidateChanged(ctx context.Context, tenantID uint64, candidateID string) {
	if !s.Active(ctx, tenantID) {
		return
	}
	if err := s.Repo.OutboxCandidate(context.WithoutCancel(ctx), tenantID, candidateID); err != nil {
		slog.Warn("igdt: no se pudo encolar el candidato", "err", err)
		return
	}
	s.wake()
}

func (s *IGDTService) wake() {
	s.mu.Lock()
	ch := s.kick
	s.mu.Unlock()
	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Pump envía la cola a IGDT: al despertar por un cambio o cada `every`; concilia cada pocos minutos.
func (s *IGDTService) Pump(ctx context.Context, every time.Duration) {
	if s.Client == nil || !s.Client.Enabled() || s.Pusher == nil {
		return
	}
	s.mu.Lock()
	s.kick = make(chan struct{}, 1)
	ch := s.kick
	s.mu.Unlock()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	var lastReconcile, lastPurge time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-ch:
			// agrupa los cambios seguidos (p. ej. arrastrar varias tarjetas)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
		reconcile := time.Since(lastReconcile) >= igdtReconcileEach
		if reconcile {
			lastReconcile = time.Now()
		}
		targets, err := s.Repo.Targets(ctx, nil)
		if err != nil {
			slog.Warn("igdt: no se pudo leer los tenants conectados", "err", err)
			continue
		}
		for _, t := range targets {
			if reconcile {
				if _, err := s.Repo.OutboxReconcile(ctx, t.TenantID, false); err != nil {
					slog.Warn("igdt: conciliación fallida", "tenant", t.Slug, "err", err)
				}
			}
			if err := s.drain(ctx, t); err != nil && !errors.Is(err, context.Canceled) {
				slog.Warn("igdt: envío fallido", "tenant", t.Slug, "err", domain.AsError(err).Code)
			}
			s.pullHires(ctx, t)
		}
		if time.Since(lastPurge) > 24*time.Hour {
			lastPurge = time.Now()
			if err := s.Repo.OutboxPurge(ctx); err != nil {
				slog.Warn("igdt: no se pudo depurar la cola", "err", err)
			}
		}
	}
}

// drain envía los pendientes listos del tenant (hasta 20 lotes por vuelta).
func (s *IGDTService) drain(ctx context.Context, t ports.IGDTTarget) error {
	key, err := s.Cipher.Decrypt(t.KeyEnc)
	if err != nil || key == "" {
		return domain.NewError("IGDT_KEY_INVALID")
	}
	for round := 0; round < 20; round++ {
		items, err := s.Repo.OutboxNext(ctx, t.TenantID, igdtPushBatch)
		if err != nil || len(items) == 0 {
			return err
		}
		var rqs, hires []ports.IGDTOutboxItem
		for _, it := range items {
			if it.Kind == "RQ" {
				rqs = append(rqs, it)
			} else {
				hires = append(hires, it)
			}
		}
		if err := s.pushKind(ctx, t.TenantID, key, rqs, s.rqDocs, s.Pusher.PushRequirements); err != nil {
			return err
		}
		if err := s.pushKind(ctx, t.TenantID, key, hires, s.hireDocs, s.Pusher.PushHires); err != nil {
			return err
		}
		if len(items) < igdtPushBatch {
			return nil
		}
	}
	return nil
}

type igdtPushFn func(ctx context.Context, key string, items []json.RawMessage) ([]ports.IGDTPushResult, error)

// pushKind arma los snapshots del lote (una sola consulta), los envía y registra los resultados (una sola escritura).
func (s *IGDTService) pushKind(ctx context.Context, tid uint64, key string, items []ports.IGDTOutboxItem,
	docs func(context.Context, uint64, []string) (map[string]json.RawMessage, error), push igdtPushFn) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	byID := map[string][]uint64{}
	for _, it := range items {
		if _, ok := byID[it.EntityID]; !ok {
			ids = append(ids, it.EntityID)
		}
		byID[it.EntityID] = append(byID[it.EntityID], it.ID)
	}
	results := []ports.IGDTOutboxResult{}
	mark := func(eid string, ok bool, msg string) {
		for _, id := range byID[eid] {
			r := ports.IGDTOutboxResult{ID: id, Error: msg}
			if ok {
				r.OK = 1
			}
			results = append(results, r)
		}
	}
	defer func() {
		if err := s.Repo.OutboxResults(context.WithoutCancel(ctx), results); err != nil {
			slog.Warn("igdt: no se pudo registrar el resultado de los envíos", "err", err)
		}
	}()
	snap, err := docs(ctx, tid, ids)
	if err != nil {
		for _, eid := range ids {
			mark(eid, false, "SNAPSHOT: "+domain.AsError(err).Code)
		}
		return err
	}
	payload := make([]json.RawMessage, 0, len(snap))
	sent := []string{}
	for _, eid := range ids {
		if d, ok := snap[eid]; ok {
			payload = append(payload, d)
			sent = append(sent, eid)
		} else {
			mark(eid, true, "") // ya no existe en BDT: nada que enviar
		}
	}
	if len(payload) == 0 {
		return nil
	}
	res, err := push(ctx, key, payload)
	if err != nil {
		for _, eid := range sent {
			mark(eid, false, domain.AsError(err).Code)
		}
		return err
	}
	seen := map[string]bool{}
	for _, r := range res {
		if _, ok := byID[r.ID]; ok && !seen[r.ID] {
			seen[r.ID] = true
			mark(r.ID, r.OK, r.Error)
		}
	}
	for _, eid := range sent {
		if !seen[eid] {
			mark(eid, false, "NO_RESULT")
		}
	}
	return nil
}

func docsByID(list []json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(list))
	for _, d := range list {
		var h struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(d, &h) == nil && h.ID != "" {
			out[h.ID] = d
		}
	}
	return out
}

// rqDocs: snapshots de los RQ con los responsables como texto.
func (s *IGDTService) rqDocs(ctx context.Context, tid uint64, ids []string) (map[string]json.RawMessage, error) {
	list, err := s.Repo.RQSnapshots(ctx, tid, ids)
	if err != nil {
		return nil, err
	}
	for i, raw := range list {
		var m map[string]any
		if err := json.Unmarshal(decryptNames(s.Cipher, raw), &m); err != nil {
			return nil, err
		}
		names := []string{}
		if owners, ok := m["owners"].([]any); ok {
			for _, o := range owners {
				if om, ok := o.(map[string]any); ok {
					if n, _ := om["name"].(string); n != "" {
						names = append(names, n)
					}
				}
			}
		}
		m["owner"] = strings.Join(names, ", ")
		delete(m, "owners")
		list[i], _ = json.Marshal(m)
	}
	return docsByID(list), nil
}

// hireDocs: snapshots de los candidatos con sus datos personales descifrados (solo viajan a IGDT por TLS).
func (s *IGDTService) hireDocs(ctx context.Context, tid uint64, ids []string) (map[string]json.RawMessage, error) {
	list, err := s.Repo.HireSnapshots(ctx, tid, ids)
	if err != nil {
		return nil, err
	}
	for i, raw := range list {
		list[i] = decryptKeys(s.Cipher, decryptNames(s.Cipher, raw), "docNumberEnc", "emailEnc", "phoneEnc")
	}
	return docsByID(list), nil
}

// decryptKeys reemplaza las claves indicadas ("<x>Enc", base64) por "<x>" descifrado, a cualquier profundidad.
func decryptKeys(c ports.Cipher, doc json.RawMessage, keys ...string) json.RawMessage {
	var v any
	if json.Unmarshal(doc, &v) != nil {
		return doc
	}
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			for k, val := range t {
				if contains(keys, k) {
					plain := ""
					if enc, ok := val.(string); ok && enc != "" {
						if b, err := base64.StdEncoding.DecodeString(enc); err == nil {
							plain, _ = c.Decrypt(b)
						}
					}
					delete(t, k)
					t[strings.TrimSuffix(k, "Enc")] = plain
					continue
				}
				walk(val)
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
	out, err := json.Marshal(v)
	if err != nil {
		return doc
	}
	return out
}

// resendAll: concilia todo el tenant (RQ abiertos y cerrados recientes) y lo envía en el momento.
func (s *IGDTService) resendAll(ctx context.Context, tid uint64) error {
	if s.Pusher == nil {
		return nil
	}
	if _, err := s.Repo.OutboxReconcile(ctx, tid, true); err != nil {
		return err
	}
	targets, err := s.Repo.Targets(ctx, &tid)
	if err != nil || len(targets) == 0 {
		return err
	}
	return s.drain(ctx, targets[0])
}

// pullHires lee los ingresos registrados en IGDT y los aplica (candidato ingresado, RQ cerrado si cubrió sus
// vacantes). Sin el alcance hires.read en el token se reintenta cada 15 minutos para no saturar.
func (s *IGDTService) pullHires(ctx context.Context, t ports.IGDTTarget) {
	if s.Hires == nil {
		return
	}
	s.mu.Lock()
	wait := s.hiresWait[t.TenantID]
	s.mu.Unlock()
	if time.Now().Before(wait) {
		return
	}
	key, err := s.Cipher.Decrypt(t.KeyEnc)
	if err != nil || key == "" {
		return
	}
	cursor := t.HiresCursor
	for page := 0; page < 10; page++ {
		res, err := s.Hires.HireEvents(ctx, key, cursor, 200)
		if err != nil {
			code := domain.AsError(err).Code
			if code == "IGDT_SCOPE_MISSING" {
				s.mu.Lock()
				if s.hiresWait == nil {
					s.hiresWait = map[uint64]time.Time{}
				}
				s.hiresWait[t.TenantID] = time.Now().Add(15 * time.Minute)
				s.mu.Unlock()
			}
			slog.Warn("igdt: no se pudieron leer los ingresos", "tenant", t.Slug, "err", code)
			return
		}
		if res.Next == cursor {
			return
		}
		closed, err := s.Repo.ApplyHires(ctx, t.TenantID, res.Events, res.Next)
		if err != nil {
			slog.Warn("igdt: no se pudieron aplicar los ingresos", "tenant", t.Slug, "err", err)
			return
		}
		if len(closed) > 0 {
			slog.Info("igdt: RQ cerrados por ingresos registrados", "tenant", t.Slug, "rqs", closed)
		}
		cursor = res.Next
		if !res.More {
			return
		}
	}
}
