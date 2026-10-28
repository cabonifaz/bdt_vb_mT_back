package app

import (
	"testing"
	"time"
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
