package release

import "testing"

func TestVersionAndNotes(t *testing.T) {
	if !Valid() {
		t.Fatalf("VERSION inválido: %q", Version())
	}
	if len(Notes(Version())) == 0 {
		t.Fatalf("el registro no tiene notas para v%s", Version())
	}
	log := "# x\n## v1.2 — 2026-01-01\n- a\n- b\n## v1.1 — 2025-12-01\n- c\n"
	if got := notesFrom(log, "1.2"); len(got) != 2 || got[1] != "b" {
		t.Errorf("v1.2: %v", got)
	}
	if got := notesFrom(log, "1.1"); len(got) != 1 || got[0] != "c" {
		t.Errorf("v1.1: %v", got)
	}
	if got := notesFrom(log, "1.12"); len(got) != 0 {
		t.Errorf("v1.12 no existe: %v", got)
	}
}
