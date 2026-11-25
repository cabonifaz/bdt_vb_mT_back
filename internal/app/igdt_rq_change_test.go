package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

type hiresRepo struct {
	ports.IGDTRepository
	events json.RawMessage
	cursor uint64
	moved  []ports.IGDTMovedCandidate
}

func (r *hiresRepo) ApplyHires(_ context.Context, _ uint64, events json.RawMessage, cursor uint64) (*ports.IGDTHiresApplied, error) {
	r.events, r.cursor = events, cursor
	return &ports.IGDTHiresApplied{Moved: r.moved}, nil
}
func (r *hiresRepo) OutboxNext(context.Context, uint64, int) ([]ports.IGDTOutboxItem, error) {
	return nil, nil
}

type hiresReader struct{ page *ports.IGDTHireEvents }

func (h hiresReader) HireEvents(context.Context, string, uint64, int) (*ports.IGDTHireEvents, error) {
	return h.page, nil
}

type notifySpy struct{ moved []ports.IGDTMovedCandidate }

func (n *notifySpy) HireRevoked(context.Context, uint64, string, ports.IGDTRevokedHire) {}
func (n *notifySpy) CandidateFromIGDT(_ context.Context, _ uint64, _ string, m ports.IGDTMovedCandidate) {
	n.moved = append(n.moved, m)
}

// Evento RQ_CHANGE_REQUESTED de IGDT 0.14 (varios RQ destino): la página llega tal cual al SP y por cada candidato que
// BDT agregó se avisa a los responsables del RQ destino.
func TestPullHiresRQChange(t *testing.T) {
	ev := `[{"type":"RQ_CHANGE_REQUESTED","operationId":"op-1","candidateId":"c-origen","talentId":"t-1","hireId":"h-1",` +
		`"fromRequirementId":"rq-origen","requirementId":"rq-a","date":"2026-12-01","at":"2026-11-25T18:00:00Z",` +
		`"requirements":[{"id":"rq-a","code":"RQ-2026-0001","title":"Java","client":"ACME"},{"id":"rq-b","code":"RQ-2026-0002","title":"QA","client":"ACME"}]},` +
		`{"type":"TERMINATED","candidateId":"c-origen","terminationDate":"2026-11-30","reason":{"id":"r1","code":"END","label":"Fin"},` +
		`"recoverEquipment":true,"equipmentReturnDate":"2026-12-02"},{"type":"OTRO"}]`
	repo := &hiresRepo{moved: []ports.IGDTMovedCandidate{
		{RQID: "rq-a", RQCode: "RQ-2026-0001", CandidateID: "n-1", TalentName: "Ana Pérez", FromCode: "RQ-2026-0000"},
		{RQID: "rq-b", RQCode: "RQ-2026-0002", CandidateID: "n-2", TalentName: "Ana Pérez", FromCode: "RQ-2026-0000"},
	}}
	spy := &notifySpy{}
	s := &IGDTService{Repo: repo, Hires: hiresReader{&ports.IGDTHireEvents{Events: json.RawMessage(ev), Next: 9}}, Cipher: plainCipher{}, Notifier: spy}
	s.pullHires(context.Background(), ports.IGDTTarget{TenantID: 7, Slug: "fractal", KeyEnc: []byte("k"), HiresCursor: 3})
	if string(repo.events) != ev || repo.cursor != 9 {
		t.Fatalf("al SP: cursor %d, eventos %s", repo.cursor, repo.events)
	}
	if len(spy.moved) != 2 || spy.moved[1].RQCode != "RQ-2026-0002" {
		t.Fatalf("avisos: %+v", spy.moved)
	}
}

func TestIGDTRQChangePush(t *testing.T) {
	m := ports.IGDTMovedCandidate{RQCode: "RQ-2026-0002", RQTitle: "QA", TalentName: "Ana", FromCode: "RQ-2026-0001", CandidateID: "c"}
	p := igdtRQChangePush("es", m, "/fractal/requerimientos/x", "n")
	if !strings.Contains(p.Body, "desde RQ-2026-0001") || p.URL != "/fractal/requerimientos/x" {
		t.Fatalf("push: %+v", p)
	}
	m.FromCode = ""
	if p = igdtRQChangePush("en", m, "l", "n"); strings.Contains(p.Body, "from") || !strings.HasPrefix(p.Title, "Candidate") {
		t.Fatalf("push en: %+v", p)
	}
}
