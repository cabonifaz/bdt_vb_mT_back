package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

const ip = "127.0.0.1"

type user struct {
	p    *domain.Principal
	name string
}

type talentRef struct {
	id, profile, name string
}

// seedTenant crea (o reutiliza) el tenant y genera usuarios, catálogos, clientes, talentos, RQ, candidatos, entrevistas y lista negra.
func (e *env) seedTenant(ctx context.Context, spec TenantSpec, keepAdmin string) ([]string, error) {
	var saID uint64
	if err := e.db.QueryRowContext(ctx, `SELECT u.id FROM app_user u JOIN user_role ur ON ur.user_id = u.id JOIN role r ON r.id = ur.role_id
		WHERE r.code = 'SUPER_ADMIN' AND u.deleted_at IS NULL ORDER BY u.id LIMIT 1`).Scan(&saID); err != nil {
		return nil, fmt.Errorf("super admin: %w", err)
	}
	sa, err := e.auth.Principal(ctx, saID)
	if err != nil {
		return nil, err
	}
	var creds []string
	var tid uint64
	var admin user
	domainMail := spec.Slug + ".demo"

	if spec.Existing {
		if err := e.db.QueryRowContext(ctx, "SELECT id FROM tenant WHERE slug = ?", spec.Slug).Scan(&tid); err != nil {
			return nil, err
		}
		var aid uint64
		if err := e.db.QueryRowContext(ctx, "SELECT id FROM app_user WHERE email_bidx = ? AND tenant_id = ?",
			e.cipher.BlindIndex(strings.ToLower(keepAdmin)), tid).Scan(&aid); err != nil {
			return nil, fmt.Errorf("admin de %s: %w", spec.Slug, err)
		}
		p, err := e.auth.Principal(ctx, aid)
		if err != nil {
			return nil, err
		}
		admin = user{p, p.FullName}
		creds = append(creds, fmt.Sprintf("# %s: el admin existente (%s) conserva su clave", spec.Name, keepAdmin))
	} else {
		email := "admin@" + domainMail
		t, inv, err := e.tenants.Create(ctx, sa, ip, app.NewTenant{Slug: spec.Slug, Name: spec.Name, LegalName: ptr(spec.Legal), TaxID: ptr(spec.RUC),
			Locale: "es", Timezone: "America/Lima", AdminEmail: email})
		if err != nil {
			return nil, fmt.Errorf("crear tenant: %w", err)
		}
		tid = t.ID
		name := e.personName()
		pwd := password()
		p, err := e.accept(ctx, inv, name, pwd)
		if err != nil {
			return nil, fmt.Errorf("admin: %w", err)
		}
		admin = user{p, name}
		creds = append(creds, fmt.Sprintf("\n# %s (https://<app>/%s/login)", spec.Name, spec.Slug), fmt.Sprintf("%s_ADMIN=%s %s", strings.ToUpper(codeOf(spec.Slug)), email, pwd))
		_, err = e.tenants.UpdateBranding(ctx, admin.p, ip, tid, domain.Branding{DisplayName: spec.Name, ColorPrimary: spec.Primary, ColorPrimaryHov: spec.Primary,
			ColorSecondary: spec.Secondary, ColorAccent: "#FAAB34", ColorSidebar: "#FFFFFF", FontFamily: "Inter"})
		e.fails("marca "+spec.Slug, err)
	}

	// analistas de selección (reclutadores) y comerciales
	invite := func(role, prefix string, n int) []user {
		var out []user
		for i := 1; i <= n; i++ {
			email := fmt.Sprintf("%s%d@%s", prefix, i, domainMail)
			inv, err := e.users.Invite(ctx, admin.p, ip, &tid, email, role, spec.Name, "es")
			if e.fails("invitar "+email, err) {
				continue
			}
			name, pwd := e.personName(), password()
			p, err := e.accept(ctx, inv, name, pwd)
			if e.fails("aceptar "+email, err) {
				continue
			}
			out = append(out, user{p, name})
			creds = append(creds, fmt.Sprintf("%s_%s%d=%s %s   # %s", strings.ToUpper(codeOf(spec.Slug)), strings.ToUpper(prefix), i, email, pwd, name))
		}
		return out
	}
	analysts := invite("RECRUITER", "analista", spec.Analysts)
	commercial := invite("COMMERCIAL", "comercial", spec.Commercial)
	if len(analysts) == 0 {
		return creds, fmt.Errorf("no se crearon analistas")
	}
	writers := append([]user{admin}, commercial...) // con rq.write y tarifas

	// catálogos del tenant: skills, perfiles y clientes del rubro
	m, err := e.loadMasters(ctx, tid)
	if err != nil {
		return creds, err
	}
	ind := spec.Industry
	upsert := func(table, code, lbl string, s1, s2 *string) {
		_, err := e.masters.Upsert(ctx, admin.p, ip, ports.MasterUpsert{TenantID: &tid, TableCode: table, Code: code, Label: label(lbl), Str1: s1, Str2: s2, IsActive: true})
		e.fails("maestro "+table+" "+lbl, err)
	}
	seen := map[string]bool{}
	for _, pr := range ind.Profiles {
		if m.id("RQ_PROFILE", pr.Code) == "" {
			upsert("RQ_PROFILE", pr.Code, pr.Name, nil, nil)
		}
		for _, s := range pr.Skills {
			if k := strings.ToLower(s); !seen[k] && m.byLabel["TECH_SKILL"][k] == "" {
				seen[k] = true
				upsert("TECH_SKILL", codeOf(s), s, nil, nil)
			}
		}
	}
	for _, c := range ind.Clients {
		upsert("CLIENT", codeOf(c.Name), c.Name, ptr(c.RUC), ptr(c.Address))
	}
	if m, err = e.loadMasters(ctx, tid); err != nil {
		return creds, err
	}
	skillID := func(lbl string) string { return m.byLabel["TECH_SKILL"][strings.ToLower(lbl)] }
	clientIDs := m.ids("CLIENT")
	fmt.Printf("%-18s usuarios y catálogos listos (%d analistas, %d clientes)\n", spec.Slug, len(analysts), len(clientIDs))

	// contactos de cada cliente
	contacts := map[string][]string{}
	var cmu sync.Mutex
	e.parallel(len(clientIDs), func(i int) {
		for j := 0; j < e.between(1, 3); j++ {
			id := uuid.NewString()
			first, last := e.firstName(), pick(e, surnames)
			err := e.rqs.SaveContact(ctx, admin.p, ip, ports.ContactWrite{ID: id, ClientID: clientIDs[i], FirstName: first, LastName: last,
				SecondLastName: pick(e, surnames), Position: pick(e, []string{"Jefe de Recursos Humanos", "Analista de Selección", "Gerente de Operaciones", "Superintendente", "Jefe de Proyecto", "Coordinador de Contrataciones"}),
				Phone: fmt.Sprintf("+51 9%08d", e.intn(1e8)), Email: strings.ToLower(ascii(first)+"."+ascii(last)) + "@cliente.demo"})
			if !e.fails("contacto", err) {
				cmu.Lock()
				contacts[clientIDs[i]] = append(contacts[clientIDs[i]], id)
				cmu.Unlock()
			}
		}
	})

	// talentos (postulantes) con experiencia, estudios, skills, idiomas y expectativa salarial
	talents := make([]talentRef, spec.Talents)
	weights := profileWeights(ind.Profiles)
	creators := append(append([]user{}, writers...), analysts...)
	var done int64
	var dmu sync.Mutex
	e.parallel(spec.Talents, func(i int) {
		pr := ind.Profiles[weights[e.intn(len(weights))]]
		by := creators[e.intn(len(creators))]
		withRates := by.p.Has("talent.rates.read")
		t := e.talent(ctx, spec, i, pr, by.p, m, skillID, withRates)
		talents[i] = t
		dmu.Lock()
		done++
		if done%500 == 0 {
			fmt.Printf("%-18s %d/%d talentos\n", spec.Slug, done, spec.Talents)
		}
		dmu.Unlock()
	})
	byProfile := map[string][]talentRef{}
	var all []talentRef
	for _, t := range talents {
		if t.id != "" {
			byProfile[t.profile] = append(byProfile[t.profile], t)
			all = append(all, t)
		}
	}
	fmt.Printf("%-18s %d talentos creados\n", spec.Slug, len(all))

	// lista negra: ~1% (global o de un cliente)
	for i := 0; i < len(all)/100; i++ {
		client := ""
		if e.chance(0.6) {
			client = pick(e, clientIDs)
		}
		_, err := e.bl.Create(ctx, admin.p, ip, uuid.NewString(), all[e.intn(len(all))].id, client,
			pick(e, []string{"Abandonó el puesto sin previo aviso durante el periodo de prueba.", "Información falsa en la documentación presentada.",
				"Incumplimiento grave de normas de seguridad en mina.", "El cliente solicitó no volver a presentarlo.", "Conducta inapropiada en entrevista con el cliente."}))
		e.fails("lista negra", err)
	}

	e.genRQs(ctx, spec, spec.RQs, rqDeps{m: m, skillID: skillID, clientIDs: clientIDs, contacts: contacts, analysts: analysts, writers: writers,
		byProfile: byProfile, all: all})
	return creds, nil
}

// accept acepta la invitación (token del enlace) y devuelve el principal del usuario creado.
func (e *env) accept(ctx context.Context, inv *app.InviteResult, name, pwd string) (*domain.Principal, error) {
	if inv == nil || inv.Link == "" {
		return nil, fmt.Errorf("invitación sin enlace")
	}
	token := inv.Link[strings.LastIndex(inv.Link, "/")+1:]
	id, err := e.users.AcceptInvitation(ctx, token, name, pwd, "es", ip)
	if err != nil {
		return nil, err
	}
	return e.auth.Principal(ctx, id)
}

func (e *env) firstName() string {
	if e.chance(0.5) {
		return pick(e, maleNames)
	}
	return pick(e, femaleNames)
}

func (e *env) personName() string {
	return e.firstName() + " " + pick(e, surnames) + " " + pick(e, surnames)
}

// talent crea un postulante completo con el principal indicado.
func (e *env) talent(ctx context.Context, spec TenantSpec, i int, pr Profile, p *domain.Principal, m masterMap, skillID func(string) string, withRates bool) talentRef {
	female := e.chance(0.45)
	if pr.Code == "DRILLER" || pr.Code == "DRILL_HELPER" || pr.Code == "HEAVY_OPERATOR" || pr.Code == "WELDER" || pr.Code == "FORKLIFT" || pr.Code == "DRIVER" {
		female = e.chance(0.08)
	}
	if pr.Code == "NURSE" || pr.Code == "NURSING_TECH" || pr.Code == "CASHIER" {
		female = e.chance(0.8)
	}
	first := pick(e, maleNames)
	if female {
		first = pick(e, femaleNames)
	}
	if e.chance(0.35) {
		first += " " + pick(e, middleNames)
	}
	last, second := pick(e, surnames), pick(e, surnames)
	years := e.between(0, 18)
	if pr.SalaryMin >= 6000 {
		years = e.between(5, 22)
	}
	ind := spec.Industry
	city := pick(e, ind.Cities)
	email := fmt.Sprintf("%s.%s%d@%s", strings.ToLower(ascii(strings.Fields(first)[0])), strings.ToLower(ascii(last)), 10+i, pick(e, emailDomains))
	headline := pick(e, pr.Headlines)
	birth := time.Now().AddDate(-(22 + years + e.between(0, 8)), -e.between(0, 11), -e.between(0, 27))
	in := app.TalentInput{ID: uuid.NewString(), FirstName: first, LastName: last, SecondLastName: ptr(second), Headline: ptr(headline),
		DocTypeID: ptr(m.id("DOC_IDENTITY_TYPE", "DNI")), DocNumber: fmt.Sprintf("%08d", 40000000+spec.docBase()+i), Email: email,
		Phone: fmt.Sprintf("+51 9%08d", e.intn(1e8)), BirthDate: birth.Format("2006-01-02"), CountryID: ptr(m.id("COUNTRY", "PE")), City: ptr(city),
		AvailabilityID: ptr(m.id("TALENT_AVAILABILITY", pick(e, []string{"IMMEDIATE", "IMMEDIATE", "WEEKS_2", "MONTH_1", "TO_AGREE"}))),
		HasEquipment:   ptr(e.chance(0.55)),
		Summary: ptr(fmt.Sprintf("%s con %d años de experiencia en %s. %s Disponibilidad para trabajar en %s.", headline, years,
			strings.ToLower(pr.Name), pick(e, []string{"Orientado a resultados y al trabajo en equipo.", "Experiencia en empresas líderes del sector.", "Comprometido con la seguridad y la mejora continua.", "Capacidad de adaptación y aprendizaje rápido."}),
			pick(e, []string{"Lima y provincias", "régimen atípico", "modalidad presencial", city})))}
	if e.chance(0.3) {
		in.OriginCountryID, in.OriginCity = ptr(m.id("COUNTRY", "PE")), ptr(pick(e, ind.Cities))
	}
	if e.chance(0.5) {
		in.LinkedinURL = ptr(fmt.Sprintf("https://www.linkedin.com/in/%s-%s-%d", strings.ToLower(ascii(strings.Fields(first)[0])), strings.ToLower(ascii(last)), 100+e.intn(900)))
	}
	if ind == itIndustry && e.chance(0.4) {
		in.GithubURL = ptr(fmt.Sprintf("https://github.com/%s%s", strings.ToLower(ascii(strings.Fields(first)[0])), strings.ToLower(ascii(last))))
	}
	if withRates {
		base := pr.SalaryMin + float64(years)*(pr.SalaryMax-pr.SalaryMin)/22
		min := round2(base*(0.95+e.float()*0.1)/50) * 50
		mod := "PAYROLL"
		r := &domain.Rates{ModalityID: ptr(m.id("BILLING_MODALITY", mod)), PayrollMin: ptr(min), PayrollMax: ptr(min * 1.15), PayrollCurrencyID: ptr(m.id("CURRENCY", "PEN"))}
		if ind == itIndustry && e.chance(0.4) {
			r.ModalityID, r.RxhMin, r.RxhMax, r.RxhCurrencyID = ptr(m.id("BILLING_MODALITY", "RXH")), ptr(min*1.2), ptr(min*1.4), ptr(m.id("CURRENCY", "PEN"))
		}
		in.Rates = r
	}
	if _, err := e.talents.Save(ctx, p, ip, in); e.fails("talento", err) {
		return talentRef{}
	}
	tid := in.ID
	for _, s := range pickN(e, pr.Skills, e.between(3, len(pr.Skills))) {
		if id := skillID(s); id != "" {
			y := float64(e.between(1, max(1, years)))
			e.fails("skill", e.talents.SaveTechSkill(ctx, p, tid, app.SkillInput{ID: uuid.NewString(), SkillID: id, Years: y}))
		}
	}
	for _, c := range pickN(e, softSkillCodes, e.between(1, 3)) {
		if id := m.id("SOFT_SKILL", c); id != "" {
			e.fails("habilidad blanda", e.talents.AddSoftSkill(ctx, p, tid, app.SkillInput{ID: uuid.NewString(), SkillID: id}))
		}
	}
	// experiencia: del trabajo actual hacia atrás
	end := time.Now()
	current := e.chance(0.6)
	for k := 0; k < e.between(1, 4) && years > 0; k++ {
		months := e.between(4, 48)
		start := end.AddDate(0, -months, 0)
		ex := domain.TalentExperience{ID: uuid.NewString(), Company: pick(e, ind.Companies), Position: pick(e, pr.Positions),
			StartDate: start.Format("2006-01-02"), IsCurrent: k == 0 && current, Functions: ptr(strings.Join(pickN(e, pr.Functions, e.between(2, len(pr.Functions))), "\n"))}
		if !ex.IsCurrent {
			ex.EndDate = ptr(end.Format("2006-01-02"))
		}
		e.fails("experiencia", e.talents.SaveExperience(ctx, p, tid, ex))
		end = start.AddDate(0, -e.between(0, 6), 0)
	}
	grade := "BACHELOR"
	if pr.TechnicalEd {
		grade = "TECHNICAL"
	} else if e.chance(0.4) {
		grade = "DEGREE"
	}
	gy := time.Now().Year() - years - e.between(0, 2)
	e.fails("educación", e.talents.SaveEducation(ctx, p, tid, domain.TalentEducation{ID: uuid.NewString(), Institution: pick(e, ind.Institutions),
		Career: pick(e, pr.Careers), GradeID: ptr(m.id("ACADEMIC_GRADE", grade)), StartDate: ptr(fmt.Sprintf("%d-03-01", gy-5)),
		EndDate: ptr(fmt.Sprintf("%d-12-15", gy)), DatePrecision: "YEAR"}))
	if e.chance(0.3) {
		e.fails("curso", e.talents.SaveEducation(ctx, p, tid, domain.TalentEducation{ID: uuid.NewString(), Institution: pick(e, ind.Institutions),
			Career: "Curso de " + strings.ToLower(pick(e, pr.Skills)), GradeID: ptr(m.id("ACADEMIC_GRADE", "COURSE")),
			StartDate: ptr(fmt.Sprintf("%d-01-01", time.Now().Year()-e.between(0, 4))), DatePrecision: "YEAR"}))
	}
	e.fails("idioma", e.talents.SaveLanguage(ctx, p, tid, domain.TalentLanguage{ID: uuid.NewString(), LanguageID: m.id("LANGUAGE", "ES"), LevelID: m.id("LANGUAGE_LEVEL", "NATIVE"), Stars: 4}))
	if lv := pick(e, []string{"", "BASIC", "BASIC", "INTERMEDIATE", "INTERMEDIATE", "ADVANCED"}); lv != "" {
		stars := map[string]int{"BASIC": 1, "INTERMEDIATE": 2, "ADVANCED": 3}[lv]
		e.fails("inglés", e.talents.SaveLanguage(ctx, p, tid, domain.TalentLanguage{ID: uuid.NewString(), LanguageID: m.id("LANGUAGE", "EN"), LevelID: m.id("LANGUAGE_LEVEL", lv), Stars: stars}))
	}
	if e.chance(0.2) {
		_, err := e.talents.SaveFeedback(ctx, p, tid, uuid.NewString(), e.between(3, 5), pick(e, []string{
			"Buen desempeño en el proceso anterior; recomendable para proyectos similares.", "Responsable y puntual. Buena actitud con el equipo.",
			"Perfil técnico sólido, reforzar comunicación con el cliente.", "Cliente satisfecho con su trabajo en la asignación previa."}))
		e.fails("feedback", err)
	}
	return talentRef{id: tid, profile: pr.Code, name: first + " " + last}
}

func (s TenantSpec) docBase() int {
	base := 0
	for _, c := range s.Slug {
		base = (base*31 + int(c)) % 9000
	}
	return base * 1000
}

// profileWeights reparte la probabilidad de cada perfil según cuántas vacantes suele pedir (más ayudantes que supervisores).
func profileWeights(ps []Profile) []int {
	var w []int
	for i, p := range ps {
		for k := 0; k < p.QtyMax+1; k++ {
			w = append(w, i)
		}
	}
	return w
}

func pickStatus(e *env) string {
	r := e.float()
	switch {
	case r < 0.18:
		return "REGISTERED"
	case r < 0.35:
		return "ASSIGNED"
	case r < 0.65:
		return "IN_SELECTION"
	case r < 0.75:
		return "IN_PRODUCTION"
	case r < 0.87:
		return "ATTENDED"
	case r < 0.92:
		return "FINISHED"
	case r < 0.96:
		return "LOST"
	}
	return "CANCELLED"
}

// candidateStage: etapa final de un candidato según el estado del RQ (y si queda cupo para confirmar).
func candidateStage(e *env, rq string, room bool) string {
	r := e.float()
	switch rq {
	case "REGISTERED":
		if r < 0.75 {
			return "OBSERVED"
		}
		return "DATA_COMPLETE"
	case "IN_PRODUCTION", "ATTENDED", "FINISHED":
		if room && r < 0.7 {
			return "CONFIRMED"
		}
	case "LOST", "CANCELLED":
		if r < 0.5 {
			return "OBSERVED"
		}
		return "DATA_COMPLETE"
	}
	switch {
	case r < 0.3:
		return "OBSERVED"
	case r < 0.6:
		return "DATA_COMPLETE"
	case r < 0.88 || !room:
		return "INTERVIEW"
	}
	return "CONFIRMED"
}

func profileByID(ps []Profile, m masterMap, id string) Profile {
	for _, p := range ps {
		if m.id("RQ_PROFILE", p.Code) == id {
			return p
		}
	}
	return ps[0]
}

func rqTitle(e *env, pr Profile, vacs []app.RQVacancyInput, ind *Industry, client string, m masterMap) string {
	total := 0
	for _, v := range vacs {
		total += v.Quantity
	}
	name := pr.Name
	if total > 1 {
		name = plural(pr.Name)
	}
	if ind == miningIndustry {
		return fmt.Sprintf("%d %s – %s", total, name, pick(e, []string{"campaña de exploración", "proyecto brownfield", "sondajes de relleno", "ampliación de mina", "guardia 14x7", "guardia 20x10"}))
	}
	return fmt.Sprintf("%s %s", name, pick(e, []string{"para proyecto de transformación", "para nueva sede", "por reemplazo", "para campaña", "para expansión", "– contrato por servicio", "para área de operaciones"}))
}

func rqDescription(e *env, pr Profile) string {
	return fmt.Sprintf("Se requiere %s con experiencia comprobada. Funciones principales: %s. %s", strings.ToLower(pr.Name),
		strings.ToLower(strings.Join(pickN(e, pr.Functions, 2), "; ")),
		pick(e, []string{"Incorporación inmediata.", "Contrato inicial con posibilidad de renovación.", "Se valorará experiencia en el mismo cliente.", "Indispensable disponibilidad para viajar."}))
}

func plural(s string) string {
	w := strings.Fields(s)
	if len(w) == 0 {
		return s
	}
	switch {
	case strings.HasSuffix(w[0], "r") || strings.HasSuffix(w[0], "l") || strings.HasSuffix(w[0], "n"):
		w[0] += "es"
	case strings.HasSuffix(w[0], "a") || strings.HasSuffix(w[0], "o") || strings.HasSuffix(w[0], "e"):
		w[0] += "s"
	}
	return strings.Join(w, " ")
}

func pickN[T any](e *env, xs []T, n int) []T {
	idx := make([]int, len(xs))
	for i := range idx {
		idx[i] = i
	}
	for i := len(idx) - 1; i > 0; i-- {
		j := e.intn(i + 1)
		idx[i], idx[j] = idx[j], idx[i]
	}
	if n > len(xs) {
		n = len(xs)
	}
	out := make([]T, 0, n)
	for _, i := range idx[:n] {
		out = append(out, xs[i])
	}
	return out
}

func firstN(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

func ascii(s string) string {
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", "Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u", "Ñ", "n", " ", "")
	return r.Replace(s)
}

// rqDeps: lo que necesita la generación de RQ (usuarios, catálogos, clientes, contactos y postulantes del tenant).
type rqDeps struct {
	m         masterMap
	skillID   func(string) string
	clientIDs []string
	contacts  map[string][]string
	analysts  []user
	writers   []user
	byProfile map[string][]talentRef
	all       []talentRef
}

// genRQs crea n requerimientos con vacantes, candidatos en distintas etapas y algunas entrevistas.
func (e *env) genRQs(ctx context.Context, spec TenantSpec, n int, d rqDeps) {
	m, skillID, clientIDs, contacts, analysts, writers, byProfile, all := d.m, d.skillID, d.clientIDs, d.contacts, d.analysts, d.writers, d.byProfile, d.all
	ind := spec.Industry
	weights := profileWeights(ind.Profiles)
	// requerimientos con vacantes y candidatos (pipeline), repartidos entre los analistas
	stat := func(code string) string { return m.id("RQ_STATUS", code) }
	stage := func(code string) string { return m.id("CANDIDATE_STAGE", code) }
	type ivTask struct {
		by                       *domain.Principal
		talent, client, rq, prof string
	}
	var ivTasks []ivTask
	var imu sync.Mutex
	e.parallel(n, func(i int) {
		owner := analysts[i%len(analysts)]
		writer := writers[e.intn(len(writers))]
		final := pickStatus(e)
		closed := final == "ATTENDED" || final == "FINISHED" || final == "LOST" || final == "CANCELLED"
		today := time.Now()
		ageDays := e.between(0, 70)
		if closed {
			ageDays = e.between(30, 120)
		}
		req := today.AddDate(0, 0, -ageDays)
		due := req.AddDate(0, 0, e.between(10, 45))
		client := pick(e, clientIDs)
		// vacantes: 1 a 3 perfiles distintos
		nv := e.between(1, 3)
		var vacs []app.RQVacancyInput
		used := map[int]bool{}
		for len(vacs) < nv {
			k := weights[e.intn(len(weights))]
			if used[k] {
				if len(used) >= len(ind.Profiles) {
					break
				}
				continue
			}
			used[k] = true
			pr := ind.Profiles[k]
			v := app.RQVacancyInput{ID: uuid.NewString(), ProfileID: m.id("RQ_PROFILE", pr.Code), Quantity: e.between(1, pr.QtyMax)}
			rate := pr.SalaryMin * (1.35 + e.float()*0.3) / 160 // tarifa por hora referencial
			v.RateInitial, v.RateFinal = ptr(round2(rate)), ptr(round2(rate*1.2))
			for _, s := range pickN(e, pr.Skills, e.between(2, 4)) {
				if id := skillID(s); id != "" {
					v.Skills = append(v.Skills, app.RQSkillInput{SkillID: id, Years: ptr(float64(e.between(1, 5))), Optional: e.chance(0.25)})
				}
			}
			if e.chance(0.5) {
				v.Careers = append(v.Careers, app.RQCareerInput{Career: pick(e, pr.Careers), GradeID: ptr(m.id("ACADEMIC_GRADE", map[bool]string{true: "TECHNICAL", false: "BACHELOR"}[pr.TechnicalEd])), Optional: e.chance(0.3)})
			}
			vacs = append(vacs, v)
		}
		first := ind.Profiles[weights[0]]
		for k := range used {
			first = ind.Profiles[k]
			break
		}
		title := rqTitle(e, first, vacs, ind, client, m)
		in := app.RQInput{ClientID: client, Title: title, Description: rqDescription(e, first), StatusID: stat("REGISTERED"),
			RequestDate: req.Format("2006-01-02"), DueDate: due.Format("2006-01-02"),
			ContractValue: ptr(float64(e.between(3, 12))), ContractUnitID: ptr(m.id("TIME_UNIT", "MONTHS")),
			WorkModalityID: ptr(m.id("WORK_MODALITY", pick(e, []string{"ON_SITE", "ON_SITE", "HYBRID", "REMOTE"}))),
			OwnerID:        ptr(owner.p.UserUUID), NeedsEmo: e.chance(0.5), NeedsSctr: e.chance(0.6), NeedsLaptop: e.chance(0.3),
			Contacts: firstN(contacts[client], 2), Vacancies: vacs}
		if e.chance(0.7) {
			in.Billing = []app.RQBillingInput{{ContractID: m.id("CONTRACT_MODALITY", "PAYROLL_FULL"), CurrencyID: m.id("CURRENCY", "PEN"),
				BaseMin: ptr(first.SalaryMin), BaseMax: ptr(first.SalaryMax), MobilityMin: ptr(200.0), MobilityMax: ptr(400.0)}}
		}
		if ind == miningIndustry {
			in.NeedsEmo, in.NeedsSctr = true, true
		}
		id := uuid.NewString()
		c, err := e.rqs.Save(ctx, writer.p, ip, id, in)
		if e.fails("RQ", err) {
			return
		}
		// candidatos según el avance del RQ
		n := e.between(spec.CandMin, spec.CandMax)
		if final == "REGISTERED" {
			n = n / 3
		}
		capacity := map[string]int{}
		confirmed := map[string]int{}
		for _, v := range vacs {
			capacity[v.ID] = v.Quantity
		}
		chosen := map[string]bool{}
		for j := 0; j < n; j++ {
			v := vacs[e.intn(len(vacs))]
			prof := profileByID(ind.Profiles, m, v.ProfileID)
			pool := byProfile[prof.Code]
			if len(pool) == 0 || e.chance(0.15) {
				pool = all
			}
			t := pool[e.intn(len(pool))]
			if chosen[t.id] {
				continue
			}
			chosen[t.id] = true
			cid := uuid.NewString()
			if _, err := e.rqs.AddCandidate(ctx, owner.p, ip, id, cid, t.id, v.ID); e.fails("candidato", err) {
				continue
			}
			target := candidateStage(e, final, confirmed[v.ID] < capacity[v.ID])
			move := func(code string) bool {
				_, err := e.rqs.MoveCandidate(ctx, owner.p, ip, cid, stage(code), nil, nil)
				return !e.fails("mover candidato a "+code, err)
			}
			switch target {
			case "DATA_COMPLETE":
				move("DATA_COMPLETE")
			case "INTERVIEW":
				if move("INTERVIEW") && !closed && e.chance(0.35) {
					imu.Lock()
					ivTasks = append(ivTasks, ivTask{owner.p, t.id, client, c.Code, prof.Name})
					imu.Unlock()
				}
			case "CONFIRMED":
				if move("DATA_COMPLETE") && move("CONFIRMED") {
					confirmed[v.ID]++
				}
			}
		}
		if final != "REGISTERED" {
			_, err := e.rqs.SetStatus(ctx, writer.p, ip, id, stat(final), nil)
			e.fails("estado RQ", err)
		}
	})
	fmt.Printf("%-18s %d RQ generados\n", spec.Slug, spec.RQs)

	// entrevistas: agendadas (hoy y próximos días) y realizadas (últimas dos semanas)
	if len(ivTasks) > 60 {
		ivTasks = ivTasks[:60]
	}
	e.parallel(len(ivTasks), func(i int) {
		t := ivTasks[i]
		day := e.between(-14, 7)
		date := time.Now().AddDate(0, 0, day)
		status := "REGISTERED"
		in := app.InterviewInput{ID: uuid.NewString(), TalentID: t.talent, TypeID: m.id("INTERVIEW_TYPE", "VIRTUAL"), StageID: m.id("INTERVIEW_STAGE", "RS_TEAM"),
			DurationID: ptr(m.id("INTERVIEW_DURATION", "MIN45")), ClientID: ptr(t.client), RequirementRef: ptr(t.rq), Profile: t.prof,
			Date: date.Format("2006-01-02"), Time: fmt.Sprintf("%02d:%s", e.between(8, 17), pick(e, []string{"00", "15", "30", "45"})),
			MeetingLink: ptr("https://teams.microsoft.com/l/meetup-join/19%3ameeting_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24]), AcknowledgeBlacklist: true}
		if day < 0 {
			status = "FINISHED"
			in.RatingGeneral, in.RatingPersonal, in.RatingExperience, in.RatingLanguages, in.RatingEducation = e.between(3, 5), e.between(3, 5), e.between(2, 5), e.between(2, 5), e.between(3, 5)
			in.NotesExperience = ptr("Experiencia acorde al perfil; buena comunicación y disponibilidad inmediata.")
		}
		in.StatusID = m.id("INTERVIEW_STATUS", status)
		_, err := e.ivs.Save(ctx, t.by, ip, in)
		e.fails("entrevista", err)
	})
	fmt.Printf("%-18s %d entrevistas\n", spec.Slug, len(ivTasks))
}

// topUpRQs completa los RQ de un tenant ya generado hasta spec.RQs (p. ej. si algunos fallaron), reutilizando sus
// usuarios, clientes, contactos y postulantes.
func (e *env) topUpRQs(ctx context.Context, spec TenantSpec) error {
	var tid uint64
	if err := e.db.QueryRowContext(ctx, "SELECT id FROM tenant WHERE slug = ? AND deleted_at IS NULL", spec.Slug).Scan(&tid); err != nil {
		return fmt.Errorf("tenant %s: %w", spec.Slug, err)
	}
	var have int
	if err := e.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM requirement WHERE tenant_id = ? AND deleted_at IS NULL", tid).Scan(&have); err != nil {
		return err
	}
	n := spec.RQs - have
	fmt.Printf("%-18s %d RQ, faltan %d\n", spec.Slug, have, n)
	if n <= 0 {
		return nil
	}
	rows, err := e.db.QueryContext(ctx, `SELECT u.id, r.code FROM app_user u JOIN user_role ur ON ur.user_id = u.id JOIN role r ON r.id = ur.role_id
		WHERE u.tenant_id = ? AND u.deleted_at IS NULL AND u.status = 'ACTIVE' ORDER BY u.id`, tid)
	if err != nil {
		return err
	}
	var d rqDeps
	for rows.Next() {
		var id uint64
		var role string
		if err := rows.Scan(&id, &role); err != nil {
			rows.Close()
			return err
		}
		p, err := e.auth.Principal(ctx, id)
		if err != nil {
			rows.Close()
			return err
		}
		if role == "RECRUITER" {
			d.analysts = append(d.analysts, user{p, p.FullName})
		} else if role == "TENANT_ADMIN" || role == "COMMERCIAL" {
			d.writers = append(d.writers, user{p, p.FullName})
		}
	}
	rows.Close()
	if len(d.analysts) == 0 || len(d.writers) == 0 {
		return fmt.Errorf("%s: faltan analistas o administradores", spec.Slug)
	}
	if d.m, err = e.loadMasters(ctx, tid); err != nil {
		return err
	}
	d.skillID = func(lbl string) string { return d.m.byLabel["TECH_SKILL"][strings.ToLower(lbl)] }
	d.clientIDs = d.m.ids("CLIENT")
	d.contacts = map[string][]string{}
	for _, c := range d.clientIDs {
		docs, err := e.rqs.Contacts(ctx, d.writers[0].p, c)
		if err != nil {
			return err
		}
		for _, doc := range docs {
			var x struct{ ID string }
			if json.Unmarshal(doc, &x) == nil {
				d.contacts[c] = append(d.contacts[c], x.ID)
			}
		}
	}
	trows, err := e.db.QueryContext(ctx, "SELECT uuid, IFNULL(headline, ''), first_name, last_name FROM talent WHERE tenant_id = ? AND deleted_at IS NULL", tid)
	if err != nil {
		return err
	}
	byHeadline := map[string]string{}
	for _, pr := range spec.Industry.Profiles {
		for _, h := range pr.Headlines {
			byHeadline[h] = pr.Code
		}
	}
	d.byProfile = map[string][]talentRef{}
	for trows.Next() {
		var t talentRef
		var headline, first, last string
		if err := trows.Scan(&t.id, &headline, &first, &last); err != nil {
			trows.Close()
			return err
		}
		t.profile, t.name = byHeadline[headline], first+" "+last
		d.all = append(d.all, t)
		if t.profile != "" {
			d.byProfile[t.profile] = append(d.byProfile[t.profile], t)
		}
	}
	trows.Close()
	e.genRQs(ctx, spec, n, d)
	return nil
}
