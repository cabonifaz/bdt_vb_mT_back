package main

import (
	"os"
	"strings"
	"testing"
)

// El PDF de prueba es válido: cabecera, objetos y tabla xref con los desplazamientos correctos.
func TestSimplePDF(t *testing.T) {
	b := simplePDF([]string{"CERTIFICADO ÚNICO LABORAL", "20261234567", "Nombres: PEÑA (prueba)"})
	s := string(b)
	if !strings.HasPrefix(s, "%PDF-1.4") || !strings.HasSuffix(s, "%%EOF\n") || !strings.Contains(s, `\332NICO`) || !strings.Contains(s, `\(prueba\)`) {
		t.Fatalf("pdf inválido:\n%s", s)
	}
	if out := os.Getenv("PDF_OUT"); out != "" {
		_ = os.WriteFile(out, b, 0o644)
	}
}
