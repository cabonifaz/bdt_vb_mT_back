// Package release: versión de la plataforma (VERSION) y sus notas (CHANGELOG.md), embebidas en el binario.
// Para publicar: subir VERSION (vMAYOR.MENOR) y agregar su sección al registro; al desplegarse en cada ambiente la API
// la registra y avisa a todos los usuarios.
package release

import (
	_ "embed"
	"regexp"
	"strings"
)

//go:embed VERSION
var versionFile string

//go:embed CHANGELOG.md
var changelog string

var versionRe = regexp.MustCompile(`^\d{1,3}\.\d{1,4}$`)

// Version: "1.0" (sin la v).
func Version() string { return strings.TrimSpace(versionFile) }

// Valid: el archivo VERSION tiene el formato MAYOR.MENOR.
func Valid() bool { return versionRe.MatchString(Version()) }

// Notes: viñetas de la sección "## v<versión>" del registro.
func Notes(version string) []string {
	return notesFrom(changelog, version)
}

func notesFrom(log, version string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(log, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "## ") {
			if in {
				break
			}
			h := strings.TrimPrefix(l, "## ")
			in = h == "v"+version || strings.HasPrefix(h, "v"+version+" ")
			continue
		}
		if in && strings.HasPrefix(l, "- ") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(l, "- ")))
		}
	}
	return out
}
