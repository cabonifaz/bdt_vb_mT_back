package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// Gestor dirigido: al confirmar a un candidato, Selección puede dirigir la solicitud de ingreso a un gestor de IGDT
// concreto. La lista sale de IGDT (GET /integration/v1/managers) con el token de integración del tenant y se guarda
// unos minutos por tenant. Si IGDT no responde, se informa available = false y la confirmación sigue sin gestor
// (gestores del cliente).

const igdtManagersTTL = 5 * time.Minute

type igdtManagersEntry struct {
	data *ports.IGDTManagers
	at   time.Time
}

// ManagersView: lo que ve el front. ClientPrimaryID / ClientSecondaryID: gestores del cliente del RQ ("" = sin gestor).
type ManagersView struct {
	Available         bool                `json:"available"`
	Managers          []ports.IGDTManager `json:"managers"`
	ClientPrimaryID   string              `json:"clientPrimaryId"`
	ClientSecondaryID string              `json:"clientSecondaryId"`
}

// ManagersFor devuelve los gestores de IGDT y los del cliente (clientID = uuid del cliente en BDT, opcional).
func (s *IGDTService) ManagersFor(ctx context.Context, p *domain.Principal, clientID string) (*ManagersView, error) {
	tid, err := tenantOf(p)
	if err != nil {
		return nil, err
	}
	if clientID != "" && !validUUID(clientID) {
		return nil, domain.ErrValidation
	}
	out := &ManagersView{Managers: []ports.IGDTManager{}}
	data := s.managers(ctx, tid)
	if data == nil {
		return out, nil
	}
	out.Available = true
	if data.Managers != nil {
		out.Managers = data.Managers
	}
	if clientID != "" {
		remote, err := s.Repo.ClientRemote(ctx, meta(p, ""), tid, clientID)
		if err != nil {
			return nil, err
		}
		for _, c := range data.Clients {
			if remote != "" && c.ClientID == remote {
				out.ClientPrimaryID, out.ClientSecondaryID = c.PrimaryID, c.SecondaryID
				break
			}
		}
	}
	return out, nil
}

// managers: lista de IGDT del tenant (cache de unos minutos); nil si no hay integración activa o IGDT no responde.
func (s *IGDTService) managers(ctx context.Context, tid uint64) *ports.IGDTManagers {
	s.mu.Lock()
	if e, ok := s.mgrCache[tid]; ok && time.Since(e.at) < igdtManagersTTL {
		s.mu.Unlock()
		return e.data
	}
	s.mu.Unlock()
	if s.Managers == nil || !s.Active(ctx, tid) {
		return nil
	}
	targets, err := s.Repo.Targets(ctx, &tid)
	if err != nil || len(targets) == 0 {
		return nil
	}
	key, err := s.Cipher.Decrypt(targets[0].KeyEnc)
	if err != nil || key == "" {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	data, err := s.Managers.Managers(cctx, key)
	at := time.Now()
	if err != nil {
		// sin respuesta: se recuerda un minuto para no demorar cada confirmación
		slog.Warn("igdt: no se pudo leer la lista de gestores", "err", err)
		data, at = nil, at.Add(time.Minute-igdtManagersTTL)
	}
	s.mu.Lock()
	if s.mgrCache == nil {
		s.mgrCache = map[uint64]igdtManagersEntry{}
	}
	s.mgrCache[tid] = igdtManagersEntry{data, at}
	s.mu.Unlock()
	return data
}
