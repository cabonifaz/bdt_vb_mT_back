package app

import "testing"

// El modelo no puede agregar ni quitar elementos: se conservan los originales (por id) y se descartan los inventados.
func TestKeepOnlyKnownItems(t *testing.T) {
	src := []CVTextItem{{ID: "a", Title: "Desarrollador", Functions: "Programar"}, {ID: "b", Title: "Analista"}}
	got := []CVTextItem{{ID: "a", Title: "Developer", Functions: "Coding"}, {ID: "x", Title: "Invented"}, {ID: "b", Title: "Analyst", Functions: "added"}}
	out := keep(src, got)
	if len(out) != 2 || out[0].Title != "Developer" || out[0].Functions != "Coding" || out[1].Title != "Analyst" || out[1].Functions != "" {
		t.Fatalf("got %+v", out)
	}
	if out := keep(src, nil); out[0].Title != "Desarrollador" {
		t.Fatalf("sin traducción se conserva el original: %+v", out)
	}
}
