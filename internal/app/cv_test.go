package app

import (
	"testing"
	"time"

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

// Lo que devuelve el modelo se acota a lo que acepta el alta: fechas, catálogos, contactos y duplicados.
func TestSanitizeCVRead(t *testing.T) {
	out := sanitizeCVRead(CVRead{
		FirstName: "  Ana ", Email: "no es correo", PhoneCode: "+51", PhoneNumber: "987 654 321", CountryCode: "pe", Linkedin: "linkedin.com/in/ana",
		TechSkills:  []CVReadSkill{{Name: "Java", Years: 4.3}, {Name: "java", Years: 2}, {Name: "Go", Years: 99}},
		Experiences: []CVReadExp{{Company: "ACME", StartDate: "2020-01", EndDate: "2021-05-01", Current: true}, {Company: " "}},
		Educations:  []CVReadEdu{{Institution: "UNI", Grade: "phd"}},
		Languages:   []CVReadLang{{Code: "en", Level: "advanced"}, {Code: "ENG", Level: "x"}},
	}, time.Now())
	if out.FirstName != "Ana" || out.Email != "" || out.PhoneCode != "51" || out.PhoneNumber != "987654321" || out.CountryCode != "PE" ||
		out.Linkedin != "https://linkedin.com/in/ana" {
		t.Fatalf("datos: %+v", out)
	}
	if len(out.TechSkills) != 2 || out.TechSkills[0].Years != 4.5 || out.TechSkills[1].Years != 1 || out.TechSkills[1].YearsKnown {
		t.Fatalf("skills: %+v", out.TechSkills)
	}
	if len(out.Experiences) != 1 || out.Experiences[0].StartDate != "" || out.Experiences[0].EndDate != "" {
		t.Fatalf("experiencias: %+v", out.Experiences)
	}
	if out.Educations[0].Grade != "" || out.Languages[0].Code != "EN" || out.Languages[0].Level != "ADVANCED" || len(out.Languages) != 1 {
		t.Fatalf("catálogos: %+v %+v", out.Educations, out.Languages)
	}
}

// Años por tecnología: los explícitos del CV mandan; si no, la suma de las experiencias donde se usó, uniendo solapes.
func TestSkillYearsFromExperiences(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	out := sanitizeCVRead(CVRead{
		Experiences: []CVReadExp{
			{Company: "A", StartDate: "2024-10-01", Current: true},         // 2 años hasta hoy
			{Company: "B", StartDate: "2020-01-01", EndDate: "2023-01-01"}, // 3 años
			{Company: "C", StartDate: "2022-01-01", EndDate: "2023-07-01"}, // se solapa con B: +0,5
			{Company: "D", StartDate: "2019-01-01", EndDate: "2019-02-15"}, // 1,5 meses
			{Company: "E", StartDate: "", EndDate: "2018-01-01"},           // sin inicio: no se mide
		},
		TechSkills: []CVReadSkill{
			{Name: "Java", UsedIn: []int{0, 1, 2}},
			{Name: "Go", Years: 7, UsedIn: []int{0}},
			{Name: "Cobol", UsedIn: []int{3}},
			{Name: "Delphi", UsedIn: []int{4, 9, -1}},
		},
	}, now)
	want := map[string]float64{"Java": 5.5, "Go": 7, "Cobol": 1, "Delphi": 1}
	for _, s := range out.TechSkills {
		if s.Years != want[s.Name] {
			t.Fatalf("%s: %v años, esperado %v", s.Name, s.Years, want[s.Name])
		}
		if s.YearsKnown != (s.Name != "Delphi") {
			t.Fatalf("%s: yearsKnown=%v", s.Name, s.YearsKnown)
		}
		if s.UsedIn != nil {
			t.Fatalf("usedIn no se devuelve al front: %+v", s)
		}
	}
}
