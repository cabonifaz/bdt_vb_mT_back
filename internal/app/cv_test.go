package app

import (
	"testing"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

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

// El formato adoptado del CV de ejemplo solo admite colores hex, alineación y tipografías conocidas.
func TestValidCVStyle(t *testing.T) {
	ok := &domain.CVStyle{NameAlign: "center", NameColor: "#111827", FooterBg: "#F3F4F6", Font: "Helvetica"}
	if !validCVStyle(nil) || !validCVStyle(ok) {
		t.Fatal("estilo válido rechazado")
	}
	for _, bad := range []domain.CVStyle{
		{NameAlign: "right", Font: "Helvetica"},
		{NameAlign: "left", Font: "Comic"},
		{NameAlign: "left", Font: "Helvetica", HeadingColor: "red;}"},
	} {
		if validCVStyle(&bad) {
			t.Fatalf("estilo inválido aceptado: %+v", bad)
		}
	}
}
