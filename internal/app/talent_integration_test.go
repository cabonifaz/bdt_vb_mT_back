package app

import (
	"net/url"
	"strings"
	"testing"
)

// Búsqueda de integración: valores por defecto, normalización y rechazo (VALIDATION) de parámetros fuera de rango.
func TestParseIntegrationTalentQuery(t *testing.T) {
	f, err := ParseIntegrationTalentQuery(url.Values{}.Get)
	if err != nil || f.Page != 1 || f.PageSize != 20 || f.MinYears != nil || f.Search != "" || f.EnglishLevel != "" {
		t.Fatalf("por defecto: %+v %v", f, err)
	}
	q := url.Values{"q": {"  Ana  "}, "skill": {" java "}, "english": {"intermediate"}, "minYears": {"2.5"},
		"page": {"3"}, "pageSize": {"50"}, "position": {"dev"}, "education": {"UNI"}}
	f, err = ParseIntegrationTalentQuery(q.Get)
	if err != nil || f.Search != "Ana" || f.Skill != "java" || f.EnglishLevel != "INTERMEDIATE" || f.MinYears == nil ||
		*f.MinYears != 2.5 || f.Page != 3 || f.PageSize != 50 || f.Position != "dev" || f.Education != "UNI" {
		t.Fatalf("normalizado: %+v %v", f, err)
	}
	long := strings.Repeat("a", 151)
	for _, bad := range []url.Values{
		{"q": {long}}, {"position": {long}}, {"education": {long}}, {"skill": {long}},
		{"english": {"B2!"}}, {"minYears": {"x"}}, {"minYears": {"-1"}}, {"minYears": {"NaN"}}, {"minYears": {"101"}},
		{"page": {"0"}}, {"page": {"1.5"}}, {"pageSize": {"0"}}, {"pageSize": {"51"}}, {"pageSize": {"abc"}},
	} {
		if _, err := ParseIntegrationTalentQuery(bad.Get); err == nil || err.Error() != "VALIDATION" {
			t.Fatalf("aceptado %v: %v", bad, err)
		}
	}
}
