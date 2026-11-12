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
	const s1, s2 = "6f1c2a3b-0000-4000-8000-000000000001", "6F1C2A3B-0000-4000-8000-000000000002"
	q = url.Values{"skills": {" " + s1 + " ," + s2 + "," + s1}, "gradeId": {" " + s2 + " "}, "status": {"archived"}}
	f, err = ParseIntegrationTalentQuery(q.Get)
	if err != nil || len(f.Skills) != 2 || f.Skills[0] != s1 || f.Skills[1] != strings.ToLower(s2) ||
		f.GradeID != strings.ToLower(s2) || f.Status != "ARCHIVED" {
		t.Fatalf("skills/gradeId/status: %+v %v", f, err)
	}
	many := make([]string, 31)
	for i := range many {
		many[i] = s1
	}
	long := strings.Repeat("a", 151)
	for _, bad := range []url.Values{
		{"skills": {strings.Join(many, ",")}}, {"skills": {s1 + ",x"}}, {"skills": {s1 + ","}},
		{"skills": {"{" + s1 + "}"}}, {"gradeId": {"abc"}}, {"status": {"ACTIVE"}}, {"status": {"DELETED"}},
		{"q": {long}}, {"position": {long}}, {"education": {long}}, {"skill": {long}},
		{"english": {"B2!"}}, {"minYears": {"x"}}, {"minYears": {"-1"}}, {"minYears": {"NaN"}}, {"minYears": {"101"}},
		{"page": {"0"}}, {"page": {"1.5"}}, {"pageSize": {"0"}}, {"pageSize": {"51"}}, {"pageSize": {"abc"}},
	} {
		if _, err := ParseIntegrationTalentQuery(bad.Get); err == nil || err.Error() != "VALIDATION" {
			t.Fatalf("aceptado %v: %v", bad, err)
		}
	}
}
