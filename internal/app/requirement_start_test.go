package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

// Fecha tentativa de inicio de labores: AAAA-MM-DD, entre hoy - 30 días y hoy + 1 año (UTC).
func TestValidStartDate(t *testing.T) {
	now := time.Date(2026, 11, 25, 23, 30, 0, 0, time.UTC)
	for in, want := range map[string]bool{
		"2026-11-25":           true,
		"2026-10-26":           true,  // hoy - 30
		"2026-10-25":           false, // hoy - 31
		"2027-11-25":           true,  // hoy + 1 año
		"2027-11-26":           false,
		"2026-12-01":           true,
		"":                     false,
		"25/11/2026":           false,
		"2026-02-30":           false,
		"2026-12-01T00:00:00Z": false,
	} {
		if got := validStartDate(in, now); got != want {
			t.Errorf("%q: %v, se esperaba %v", in, got, want)
		}
	}
}

type startRepo struct {
	ports.RequirementRepository
	moved, set *string
	moveCalls  int
	err        error
}

func (r *startRepo) CandidateMove(_ context.Context, _ ports.Meta, _ uint64, _, _ string, _ *string, _ *int, start *string) (string, error) {
	r.moveCalls++
	r.moved = start
	return "CONFIRMED", r.err
}

func (r *startRepo) CandidateStartDate(_ context.Context, _ ports.Meta, _ uint64, _, start string) error {
	r.set = &start
	return r.err
}

type outboxSpy struct{ candidates []string }

func (o *outboxSpy) RQChanged(context.Context, uint64, string) {}
func (o *outboxSpy) CandidateChanged(_ context.Context, _ uint64, id string) {
	o.candidates = append(o.candidates, id)
}

const testCand = "6f1c2b9e-3d4a-4e5f-8a7b-1c2d3e4f5a6b"
const testStage = "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d"

func rqPrincipal() *domain.Principal {
	tid := uint64(7)
	return &domain.Principal{UserID: 1, TenantID: &tid}
}

// Al mover: una fecha inválida o fuera de rango es 422 VALIDATION sin llegar al SP; una válida llega al SP y el
// cambio se encola para IGDT.
func TestMoveCandidateStartDate(t *testing.T) {
	repo, out := &startRepo{}, &outboxSpy{}
	s := &RequirementService{Repo: repo, IGDT: out}
	ctx := context.Background()
	for _, bad := range []string{"2020-01-01", "2099-01-01", "mañana"} {
		d := bad
		if _, err := s.MoveCandidate(ctx, rqPrincipal(), "", testCand, testStage, nil, nil, &d); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("%s: se esperaba VALIDATION, llegó %v", bad, err)
		}
	}
	if repo.moveCalls != 0 || len(out.candidates) != 0 {
		t.Fatalf("una fecha inválida no debe llegar al SP ni encolarse")
	}
	d := time.Now().UTC().AddDate(0, 0, 10).Format("2006-01-02")
	if _, err := s.MoveCandidate(ctx, rqPrincipal(), "", testCand, testStage, nil, nil, &d); err != nil {
		t.Fatal(err)
	}
	if repo.moved == nil || *repo.moved != d || len(out.candidates) != 1 || out.candidates[0] != testCand {
		t.Fatalf("fecha al SP: %v, encolados: %v", repo.moved, out.candidates)
	}
	empty := ""
	if _, err := s.MoveCandidate(ctx, rqPrincipal(), "", testCand, testStage, nil, nil, &empty); err != nil || repo.moved != nil {
		t.Fatalf("una fecha vacía viaja como null (el SP decide si es obligatoria): %v %v", err, repo.moved)
	}
}

// Corregir la fecha: valida, llama al SP y re-encola la solicitud de ingreso; si el SP la rechaza, no encola.
func TestSetCandidateStartDate(t *testing.T) {
	repo, out := &startRepo{}, &outboxSpy{}
	s := &RequirementService{Repo: repo, IGDT: out}
	ctx := context.Background()
	if err := s.SetCandidateStartDate(ctx, rqPrincipal(), "", testCand, ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("sin fecha: %v", err)
	}
	if err := s.SetCandidateStartDate(ctx, rqPrincipal(), "", "x", "2026-12-01"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("id inválido: %v", err)
	}
	d := time.Now().UTC().Format("2006-01-02")
	if err := s.SetCandidateStartDate(ctx, rqPrincipal(), "", testCand, d); err != nil {
		t.Fatal(err)
	}
	if repo.set == nil || *repo.set != d || len(out.candidates) != 1 {
		t.Fatalf("SP: %v, encolados: %v", repo.set, out.candidates)
	}
	repo.err = domain.NewError("CANDIDATE_HIRED")
	if err := s.SetCandidateStartDate(ctx, rqPrincipal(), "", testCand, d); err == nil || len(out.candidates) != 1 {
		t.Fatalf("con el ingreso registrado no se encola: %v %v", err, out.candidates)
	}
}

// El snapshot de ingreso conserva conditions.startDate (AAAA-MM-DD o null) al descifrar.
func TestDecryptHireDocStartDate(t *testing.T) {
	for in, want := range map[string]any{
		`{"conditions":{"emo":true,"startDate":"2026-12-01"}}`: "2026-12-01",
		`{"conditions":{"emo":true,"startDate":null}}`:         nil,
	} {
		var m map[string]any
		if err := json.Unmarshal(decryptHireDoc(plainCipher{}, json.RawMessage(in)), &m); err != nil {
			t.Fatal(err)
		}
		c := m["conditions"].(map[string]any)
		if v, ok := c["startDate"]; !ok || v != want {
			t.Fatalf("startDate: %v", c)
		}
	}
}
