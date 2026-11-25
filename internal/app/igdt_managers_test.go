package app

import (
	"context"
	"errors"
	"testing"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

type mgrRepo struct {
	ports.IGDTRepository
	remote string
}

func (r mgrRepo) Active(context.Context, uint64) (bool, error) { return true, nil }
func (r mgrRepo) Targets(_ context.Context, tid *uint64) ([]ports.IGDTTarget, error) {
	return []ports.IGDTTarget{{TenantID: *tid, KeyEnc: []byte("igdt_key")}}, nil
}
func (r mgrRepo) ClientRemote(context.Context, ports.Meta, uint64, string) (string, error) {
	return r.remote, nil
}

type mgrReader struct {
	calls int
	err   error
	key   string
}

func (m *mgrReader) Managers(_ context.Context, key string) (*ports.IGDTManagers, error) {
	m.calls++
	m.key = key
	if m.err != nil {
		return nil, m.err
	}
	return &ports.IGDTManagers{
		Managers: []ports.IGDTManager{{ID: "m1", Name: "Ana"}, {ID: "m2", Name: "Beto"}, {ID: "m3", Name: "Caro"}},
		Clients:  []ports.IGDTClientManager{{ClientID: "remote-client", PrimaryID: "m2", SecondaryID: ""}},
	}, nil
}

// Gestores: con el token del tenant, los del cliente según su id en IGDT y en cache (una sola llamada a IGDT).
func TestManagersFor(t *testing.T) {
	rd := &mgrReader{}
	s := &IGDTService{Repo: mgrRepo{remote: "remote-client"}, Managers: rd, Cipher: plainCipher{}}
	v, err := s.ManagersFor(context.Background(), rqPrincipal(), testCand)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Available || len(v.Managers) != 3 || v.ClientPrimaryID != "m2" || v.ClientSecondaryID != "" || rd.key != "igdt_key" {
		t.Fatalf("vista: %+v (llave %q)", v, rd.key)
	}
	if v, _ = s.ManagersFor(context.Background(), rqPrincipal(), ""); !v.Available || v.ClientPrimaryID != "" || rd.calls != 1 {
		t.Fatalf("cache / sin cliente: %+v llamadas %d", v, rd.calls)
	}
	if _, err := s.ManagersFor(context.Background(), rqPrincipal(), "x"); err == nil {
		t.Fatal("clientId inválido debe fallar")
	}
}

// Si IGDT no responde: available = false, lista vacía, sin error (no bloquea la confirmación).
func TestManagersForUnavailable(t *testing.T) {
	rd := &mgrReader{err: errors.New("IGDT_UNAVAILABLE")}
	s := &IGDTService{Repo: mgrRepo{}, Managers: rd, Cipher: plainCipher{}}
	v, err := s.ManagersFor(context.Background(), rqPrincipal(), testCand)
	if err != nil || v.Available || v.Managers == nil || len(v.Managers) != 0 {
		t.Fatalf("sin IGDT: %+v %v", v, err)
	}
	_, _ = s.ManagersFor(context.Background(), rqPrincipal(), testCand)
	if rd.calls != 1 {
		t.Fatalf("el fallo se recuerda un minuto: %d llamadas", rd.calls)
	}
}
