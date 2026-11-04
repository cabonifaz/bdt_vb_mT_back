package app

import (
	"context"
	"testing"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// La fecha del navegador se acepta si está a un día de la del servidor (cualquier zona horaria); si no, se usa UTC.
func TestBoardToday(t *testing.T) {
	now := time.Date(2026, 10, 28, 3, 0, 0, 0, time.UTC) // 27/10 a las 22:00 en Lima
	for in, want := range map[string]string{
		"2026-10-27": "2026-10-27",
		"2026-10-28": "2026-10-28",
		"2026-10-29": "2026-10-29",
		"2026-10-25": "2026-10-28",
		"2026-11-30": "2026-10-28",
		"":           "2026-10-28",
		"27/10/2026": "2026-10-28",
	} {
		if got := boardToday(in, now); got != want {
			t.Errorf("%q: %s, se esperaba %s", in, got, want)
		}
	}
}

type staleRepo struct {
	ports.RequirementRepository
	list  []ports.StaleRQ
	today string
}

func (r *staleRepo) StaleScan(_ context.Context, today string) ([]ports.StaleRQ, error) {
	r.today = today
	return r.list, nil
}

type staleNotifier struct {
	single  []string
	summary [][]string
}

func (n *staleNotifier) RQChanged(context.Context, *domain.Principal, *ports.RQChange) {}
func (n *staleNotifier) RQStale(_ context.Context, r ports.StaleRQ) {
	n.single = append(n.single, r.Code)
}
func (n *staleNotifier) RQStaleSummary(_ context.Context, list []ports.StaleRQ) {
	codes := []string{}
	for _, r := range list {
		codes = append(codes, r.Code)
	}
	n.summary = append(n.summary, codes)
}

// Hasta 3 RQ por empresa se avisan uno por uno; si en el mismo barrido son más, un solo resumen por empresa.
func TestScanStaleGroupsByTenant(t *testing.T) {
	list := []ports.StaleRQ{{TenantID: 1, Code: "A1"}, {TenantID: 1, Code: "A2"}}
	for i := range 5 {
		list = append(list, ports.StaleRQ{TenantID: 2, Code: "B" + string(rune('1'+i))})
	}
	repo, n := &staleRepo{list: list}, &staleNotifier{}
	s := &RequirementService{Repo: repo, Notifier: n}
	got, err := s.ScanStale(context.Background(), time.Date(2026, 11, 4, 23, 0, 0, 0, time.UTC))
	if err != nil || got != 7 {
		t.Fatalf("got %d, %v", got, err)
	}
	if repo.today != "2026-11-04" {
		t.Fatalf("today = %s", repo.today)
	}
	if len(n.single) != 2 || n.single[0] != "A1" || len(n.summary) != 1 || len(n.summary[0]) != 5 {
		t.Fatalf("single=%v summary=%v", n.single, n.summary)
	}
}
