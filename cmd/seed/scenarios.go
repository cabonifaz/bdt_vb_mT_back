package main

// Escenarios de prueba de documentos de postulación y CUL sobre los tenants ya generados:
//   - más clientes (con contactos) y más RQ por tenant (todas las etapas y vencimientos);
//   - maestro de documentos según el rubro y documentos pedidos en la mayoría de los RQ;
//   - documentos de los postulantes en todos los estados (falta, por revisar, verificado, rechazado, vencido) y el CUL
//     en cada resultado de la validación automática (auténtico, alterado, de otra persona, no existe, no vigente,
//     MTPE sin respuesta, QR ilegible), además de documentos reutilizados de otro RQ;
//   - RQ reabiertos por renuncia (con baja pendiente o efectiva) y por más vacantes.
// Los archivos se suben por la API del ambiente (-api) para que queden en su volumen. El resultado del CUL se registra
// simulado: nunca se consulta al MTPE con los RUC de prueba.

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/domain"
)

type docSpec struct {
	Code, Name string
	Mode       string // REQUIRED | OPTIONAL
	Reusable   bool
	Validity   int // días (0 = no vence)
	Global     bool
}

var (
	culDoc   = docSpec{Code: "CUL", Mode: "REQUIRED", Reusable: true, Validity: 90, Global: true}
	djDoc    = docSpec{Code: "DOC_DECLARACION_JURADA", Name: "Declaración jurada de datos", Mode: "REQUIRED"}
	pnpDoc   = docSpec{Code: "DOC_ANTECEDENTES_POLICIALES", Name: "Certificado de antecedentes policiales", Mode: "OPTIONAL", Reusable: true, Validity: 90}
	titleDoc = docSpec{Code: "DOC_TITULO_PROFESIONAL", Name: "Título profesional o constancia de egresado", Mode: "REQUIRED", Reusable: true}
)

var industryDocs = map[*Industry][]docSpec{
	itIndustry: {culDoc, titleDoc, djDoc, pnpDoc,
		{Code: "CERTIFICATE", Mode: "OPTIONAL", Reusable: true, Global: true}},
	miningIndustry: {culDoc,
		{Code: "DOC_EMO", Name: "Examen médico ocupacional (EMO)", Mode: "REQUIRED", Reusable: true, Validity: 365},
		{Code: "DOC_TRABAJOS_ALTURA", Name: "Certificado de trabajos en altura", Mode: "REQUIRED", Reusable: true, Validity: 365},
		{Code: "DOC_LICENCIA_A3", Name: "Licencia de conducir A-IIIb", Mode: "OPTIONAL", Reusable: true, Validity: 1825},
		djDoc},
	financeIndustry: {culDoc,
		{Code: "DOC_CENTRALES_RIESGO", Name: "Reporte de centrales de riesgo", Mode: "REQUIRED", Validity: 30},
		{Code: "DOC_ANTECEDENTES_PENALES", Name: "Certificado de antecedentes penales", Mode: "REQUIRED", Reusable: true, Validity: 90},
		titleDoc, djDoc},
	retailIndustry: {culDoc,
		{Code: "DOC_CARNE_SANIDAD", Name: "Carné de sanidad", Mode: "REQUIRED", Reusable: true, Validity: 365},
		{Code: "DOC_ANTECEDENTES_POLICIALES", Name: "Certificado de antecedentes policiales", Mode: "REQUIRED", Reusable: true, Validity: 90},
		{Code: "DOC_SECUNDARIA", Name: "Certificado de estudios secundarios", Mode: "OPTIONAL", Reusable: true},
		{Code: "DOC_DECLARACION_JURADA", Name: "Declaración jurada de datos", Mode: "OPTIONAL"}},
	healthIndustry: {culDoc,
		{Code: "DOC_COLEGIATURA", Name: "Colegiatura habilitada", Mode: "REQUIRED", Reusable: true, Validity: 365},
		titleDoc,
		{Code: "DOC_VACUNA_HEPATITIS", Name: "Carné de vacunación (hepatitis B)", Mode: "REQUIRED", Reusable: true},
		{Code: "DOC_SERUMS", Name: "Constancia de SERUMS", Mode: "OPTIONAL", Reusable: true},
		djDoc},
}

// Clientes adicionales (ficticios) por rubro.
var extraClients = map[*Industry][]Client{
	itIndustry: {{"Financiera Andina Digital S.A.", "20611110011", "Av. Javier Prado Este 4200, Surco"}, {"Seguros Horizonte S.A.", "20611110022", "Av. Canaval y Moreyra 380, San Isidro"},
		{"Retail Sur Tech S.A.C.", "20611110033", "Calle Las Begonias 441, San Isidro"}, {"Cooperativa Vida Plena", "20611110044", "Jr. Huancavelica 320, Cercado de Lima"},
		{"Logística Pacífico Cloud S.A.C.", "20611110055", "Av. Elmer Faucett 2850, Callao"}, {"EduNet Perú S.A.C.", "20611110066", "Av. Arequipa 3520, San Isidro"},
		{"Telecom Altiplano S.A.", "20611110077", "Av. Ejército 710, Arequipa"}, {"Gobierno Regional Digital (proyecto)", "20611110088", "Av. Abancay 251, Cercado de Lima"}},
	miningIndustry: {{"Compañía Minera Cerro Azul S.A.", "20622220011", "Av. Víctor Andrés Belaúnde 147, San Isidro"}, {"Minera Pucará del Sur S.A.C.", "20622220022", "Av. Ejército 1020, Arequipa"},
		{"Exploraciones Andinas S.A.", "20622220033", "Calle Amador Merino Reyna 267, San Isidro"}, {"Sociedad Minera Huaylas S.A.", "20622220044", "Jr. San Martín 410, Huaraz"},
		{"Minera Collahuasi Perú S.A.C.", "20622220055", "Av. Mariscal Castilla 980, Moquegua"}, {"Cobre del Norte S.A.", "20622220066", "Av. España 1580, Trujillo"},
		{"Proyecto Aurífero Ollachea", "20622220077", "Jr. Lima 255, Puno"}, {"Minera Santa Rosa de Huánuco S.A.C.", "20622220088", "Jr. Dos de Mayo 1120, Huánuco"}},
	financeIndustry: {{"Caja Municipal del Centro", "20633330011", "Jr. Ancash 340, Huancayo"}, {"Banco Comunal Progreso", "20633330022", "Av. Larco 1150, Miraflores"},
		{"Financiera Qhapaq S.A.", "20633330033", "Av. Benavides 1555, Miraflores"}, {"Seguros La Previsora S.A.", "20633330044", "Av. Pardo y Aliaga 699, San Isidro"},
		{"Edpyme Crecer Perú", "20633330055", "Av. Brasil 1820, Jesús María"}, {"Fondo de Pensiones Futuro", "20633330066", "Av. Rivera Navarrete 501, San Isidro"}},
	retailIndustry: {{"Supermercados El Ahorro S.A.C.", "20644440011", "Av. Túpac Amaru 1550, Independencia"}, {"Tiendas Mi Hogar S.A.", "20644440022", "Av. La Marina 2355, San Miguel"},
		{"Distribuidora del Norte S.A.C.", "20644440033", "Av. Grau 980, Piura"}, {"Boticas Bienestar S.A.C.", "20644440044", "Av. Angamos Este 1890, Surquillo"},
		{"Moda Andina Retail S.A.", "20644440055", "Jr. Gamarra 650, La Victoria"}, {"Centro Logístico Lurín S.A.C.", "20644440066", "Antigua Panamericana Sur km 33, Lurín"}},
	healthIndustry: {{"Clínica Santa Lucía S.A.", "20655550011", "Av. Salaverry 2400, San Isidro"}, {"Hospital Privado del Sur S.A.C.", "20655550022", "Av. Dolores 320, Arequipa"},
		{"Red de Policlínicos Vida S.A.C.", "20655550033", "Av. Brasil 2560, Pueblo Libre"}, {"Laboratorio Clínico Andino S.A.", "20655550044", "Av. Petit Thouars 4255, Miraflores"},
		{"Centro Oncológico del Norte S.A.C.", "20655550055", "Av. América Sur 1250, Trujillo"}, {"Clínica Materno Infantil Esperanza", "20655550066", "Av. Túpac Amaru 3500, Comas"}},
}

// ---------------- Cliente HTTP del ambiente (subida de archivos) ----------------

type apiSeed struct {
	base  string
	http  *http.Client
	tick  <-chan time.Time
	mu    sync.Mutex
	token map[string]string // email -> token
	pwd   map[string]string // email -> clave
	slugs map[string]string // email -> tenant
}

func newAPISeed(base string, perSecond int) *apiSeed {
	return &apiSeed{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 60 * time.Second},
		tick: time.Tick(time.Second / time.Duration(perSecond)), token: map[string]string{}, pwd: map[string]string{}, slugs: map[string]string{}}
}

func (a *apiSeed) login(email string) (string, error) {
	body, _ := json.Marshal(map[string]string{"tenant": a.slugs[email], "email": email, "password": a.pwd[email]})
	// el login tiene un límite propio más bajo: ante 429 se espera y se reintenta
	var res *http.Response
	for attempt := 0; ; attempt++ {
		<-a.tick
		r, err := a.http.Post(a.base+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		if r.StatusCode != http.StatusTooManyRequests || attempt >= 6 {
			res = r
			break
		}
		r.Body.Close()
		time.Sleep(time.Duration(attempt+1) * 10 * time.Second)
	}
	defer res.Body.Close()
	var out struct {
		AccessToken string `json:"accessToken"`
	}
	if res.StatusCode != 200 || json.NewDecoder(res.Body).Decode(&out) != nil {
		return "", fmt.Errorf("login %s: %d", email, res.StatusCode)
	}
	a.mu.Lock()
	a.token[email] = out.AccessToken
	a.mu.Unlock()
	return out.AccessToken, nil
}

// upload sube un documento del postulante (como lo haría el analista desde el modal del postulante).
func (a *apiSeed) upload(email, candidateID, typeID, name string, pdf []byte, culCode, culDocNum string) error {
	for attempt := 0; attempt < 4; attempt++ {
		a.mu.Lock()
		tok := a.token[email]
		a.mu.Unlock()
		if tok == "" {
			var err error
			if tok, err = a.login(email); err != nil {
				return err
			}
		}
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		fw, _ := w.CreateFormFile("file", name)
		_, _ = fw.Write(pdf)
		_ = w.WriteField("typeId", typeID)
		_ = w.WriteField("today", time.Now().Format("2006-01-02"))
		if culCode != "" {
			_ = w.WriteField("culCode", culCode)
			_ = w.WriteField("culDoc", culDocNum)
		}
		_ = w.Close()
		req, _ := http.NewRequest(http.MethodPost, a.base+"/api/v1/tenant/requirements/candidates/"+candidateID+"/documents", &buf)
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+tok)
		<-a.tick
		res, err := a.http.Do(req)
		if err != nil {
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			continue
		}
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		res.Body.Close()
		switch {
		case res.StatusCode == http.StatusCreated:
			return nil
		case res.StatusCode == http.StatusUnauthorized:
			a.mu.Lock()
			a.token[email] = ""
			a.mu.Unlock()
		case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
			time.Sleep(time.Duration(attempt+1) * 3 * time.Second)
		default:
			return fmt.Errorf("subida %d: %s", res.StatusCode, msg)
		}
	}
	return fmt.Errorf("subida: sin éxito tras reintentos")
}

// loadCreds lee las claves de los usuarios generados (seed_users.env): analistas por tenant.
func loadCreds(path string) (map[string][]string, map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	analysts, pwd := map[string][]string{}, map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || !strings.Contains(k, "_ANALISTA") {
			continue
		}
		parts := strings.Fields(v)
		if len(parts) != 2 {
			continue
		}
		email := parts[0]
		slug := email[strings.Index(email, "@")+1:]
		slug = strings.TrimSuffix(slug, ".demo")
		analysts[slug] = append(analysts[slug], email)
		pwd[email] = parts[1]
	}
	return analysts, pwd, sc.Err()
}

// ---------------- PDF mínimo de prueba ----------------

// simplePDF arma un PDF de una página con líneas de texto (Helvetica), válido para verlo en el navegador.
func simplePDF(lines []string) []byte {
	var content strings.Builder
	content.WriteString("BT /F1 12 Tf 60 780 Td 16 TL\n")
	for _, l := range lines {
		content.WriteString("(" + pdfEscape(l) + ") Tj T*\n")
	}
	content.WriteString("ET")
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", content.Len(), content.String()),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}

// pdfEscape: texto en WinAnsi (tildes y ñ) con paréntesis y barras escapados.
func pdfEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '(' || r == ')' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 128:
			b.WriteRune(r)
		case r < 256:
			fmt.Fprintf(&b, "\\%03o", r)
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

// ---------------- Escenarios ----------------

type candRow struct {
	id, talent, first, last, second, stage string
	docEnc                                 []byte
	resigned                               bool
}

// scenario: estado final de un documento de un postulante.
//
//	MISSING, PENDING, VERIFIED, REJECTED, EXPIRED; CUL: CUL_OK, CUL_ALTERED, CUL_OTHER, CUL_NOTFOUND, CUL_EXPIRED,
//	CUL_UNREACHABLE, CUL_NOCODE.
func (e *env) docScenario(stage string, isCUL, hasValidity bool) string {
	// cuanto más avanzado el postulante, más completa su carpeta
	present := map[string]float64{"OBSERVED": 0.35, "DATA_COMPLETE": 0.75, "INTERVIEW": 0.85, "CONFIRMED": 0.97}[stage]
	if !e.chance(present) {
		return "MISSING"
	}
	if stage == "CONFIRMED" && e.chance(0.85) {
		if isCUL {
			return "CUL_OK"
		}
		return "VERIFIED"
	}
	x := e.float()
	if isCUL {
		switch {
		case x < 0.42:
			return "CUL_OK"
		case x < 0.54:
			return "CUL_ALTERED"
		case x < 0.61:
			return "CUL_OTHER"
		case x < 0.68:
			return "CUL_NOTFOUND"
		case x < 0.78:
			return "CUL_EXPIRED"
		case x < 0.90:
			return "CUL_UNREACHABLE"
		default:
			return "CUL_NOCODE"
		}
	}
	switch {
	case x < 0.48:
		return "VERIFIED"
	case x < 0.78:
		return "PENDING"
	case x < 0.92 || !hasValidity:
		return "REJECTED"
	default:
		return "EXPIRED"
	}
}

var rejectReasons = []string{"Documento ilegible, volver a escanear", "No corresponde al titular", "Documento vencido según su fecha de emisión",
	"Falta la firma o el sello de la entidad", "Está incompleto: faltan páginas", "No es el documento solicitado"}

// scenarios aplica los escenarios de documentos y CUL a un tenant generado.
func (e *env) scenarios(ctx context.Context, spec TenantSpec, api *apiSeed, analystEmails []string, extraRQs int) error {
	var tid uint64
	if err := e.db.QueryRowContext(ctx, "SELECT id FROM tenant WHERE slug = ? AND deleted_at IS NULL", spec.Slug).Scan(&tid); err != nil {
		return fmt.Errorf("tenant %s: %w", spec.Slug, err)
	}
	admin, lead, err := e.principalsByRole(ctx, tid)
	if err != nil {
		return err
	}
	ind := spec.Industry

	// 1) más clientes (con contactos) y más RQ; los que ya existen se dejan como están
	before, err := e.loadMasters(ctx, tid)
	if err != nil {
		return err
	}
	for _, c := range extraClients[ind] {
		if before.id("CLIENT", codeOf(c.Name)) != "" {
			continue
		}
		_, err := e.masters.Upsert(ctx, admin, ip, ports.MasterUpsert{TenantID: &tid, TableCode: "CLIENT", Code: codeOf(c.Name), Label: label(c.Name),
			Str1: ptr(c.RUC), Str2: ptr(c.Address), IsActive: true})
		e.fails("cliente "+c.Name, err)
	}
	m, err := e.loadMasters(ctx, tid)
	if err != nil {
		return err
	}
	for _, c := range extraClients[ind] {
		cid := m.id("CLIENT", codeOf(c.Name))
		if cid == "" {
			continue
		}
		var have int
		_ = e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM client_contact cc JOIN master_item i ON i.id = cc.client_item_id WHERE i.uuid = ? AND cc.deleted_at IS NULL`, cid).Scan(&have)
		for j := have; j < 2; j++ {
			first, last := e.firstName(), pick(e, surnames)
			err := e.rqs.SaveContact(ctx, admin, ip, ports.ContactWrite{ID: uuid.NewString(), ClientID: cid, FirstName: first, LastName: last,
				SecondLastName: pick(e, surnames), Position: pick(e, []string{"Jefe de Recursos Humanos", "Analista de Selección", "Gerente de Operaciones", "Coordinador de Contrataciones"}),
				Phone: fmt.Sprintf("+51 9%08d", e.intn(1e8)), Email: strings.ToLower(ascii(first)+"."+ascii(last)) + "@cliente.demo"})
			e.fails("contacto", err)
		}
	}
	if extraRQs > 0 {
		d, err := e.loadDeps(ctx, spec, tid)
		if err != nil {
			return err
		}
		e.genRQs(ctx, spec, extraRQs, d)
	}

	// 2) maestro de documentos del rubro (el CUL queda manual mientras se cargan los archivos: así no se consulta al MTPE)
	docs := industryDocs[ind]
	typeUUID := map[string]string{}
	for i, ds := range docs {
		typeID := ""
		if ds.Global {
			typeID = m.id("DOCUMENT_TYPE", ds.Code)
		} else {
			typeID, err = e.docType(ctx, admin, tid, ds)
			if err != nil {
				return err
			}
		}
		if typeID == "" {
			return fmt.Errorf("%s: tipo de documento %s no encontrado", spec.Slug, ds.Code)
		}
		typeUUID[ds.Code] = typeID
		var validity *int
		if ds.Validity > 0 {
			validity = ptr(ds.Validity)
		}
		err := e.docs.SaveRule(ctx, admin, ip, ports.DocRule{TypeID: typeID, DefaultMode: ds.Mode, Reusable: ds.Reusable, ValidityDays: validity,
			Verification: "MANUAL", Active: true, SortOrder: i + 1})
		if e.fails("regla "+ds.Code, err) {
			return err
		}
	}

	// 3) documentos pedidos: 70% de los RQ (algunos con el CUL opcional u omitiendo opcionales)
	rqRows, err := e.db.QueryContext(ctx, `SELECT r.id FROM requirement r WHERE r.tenant_id = ? AND r.deleted_at IS NULL
		AND NOT EXISTS (SELECT 1 FROM requirement_document rd WHERE rd.requirement_id = r.id)`, tid)
	if err != nil {
		return err
	}
	var rqIDs []uint64
	for rqRows.Next() {
		var id uint64
		if rqRows.Scan(&id) == nil {
			rqIDs = append(rqIDs, id)
		}
	}
	rqRows.Close()
	typeIDs := map[string]uint64{}
	for _, ds := range docs {
		var id uint64
		if err := e.db.QueryRowContext(ctx, "SELECT id FROM master_item WHERE uuid = ?", typeUUID[ds.Code]).Scan(&id); err != nil {
			return err
		}
		typeIDs[ds.Code] = id
	}
	withDocs := 0
	for _, rq := range rqIDs {
		if !e.chance(0.7) {
			continue
		}
		withDocs++
		for _, ds := range docs {
			mode := ds.Mode
			if ds.Mode == "OPTIONAL" && e.chance(0.4) {
				continue // este RQ no pide el opcional
			}
			if ds.Code == "CUL" && e.chance(0.1) {
				mode = "OPTIONAL"
			}
			if _, err := e.db.ExecContext(ctx, "INSERT IGNORE INTO requirement_document (requirement_id, document_type_item_id, mode) VALUES (?, ?, ?)",
				rq, typeIDs[ds.Code], mode); err != nil {
				return err
			}
		}
	}
	fmt.Printf("%-18s documentos pedidos en %d de %d RQ\n", spec.Slug, withDocs, len(rqIDs))

	// 4) documentos de los postulantes (hasta 7 por RQ; los reutilizables se cargan una vez por talento)
	cands, err := e.scenarioCandidates(ctx, tid)
	if err != nil {
		return err
	}
	uids := map[string]uint64{}
	for _, em := range analystEmails {
		var id uint64
		if e.db.QueryRowContext(ctx, "SELECT id FROM app_user WHERE email_bidx = ? AND tenant_id = ?", e.cipher.BlindIndex(em), tid).Scan(&id) == nil {
			uids[em] = id
		}
	}
	if len(uids) == 0 {
		return fmt.Errorf("%s: sin analistas con clave en el archivo de credenciales", spec.Slug)
	}
	type job struct {
		c    candRow
		ds   docSpec
		scen string
		rq   uint64
		mode string
	}
	var jobs []job
	reusedDone := map[string]bool{} // talento+tipo reutilizable ya cargado
	for rq, list := range cands {
		var rdocs []struct {
			code, mode string
		}
		rows, err := e.db.QueryContext(ctx, `SELECT i.code, rd.mode FROM requirement_document rd JOIN master_item i ON i.id = rd.document_type_item_id WHERE rd.requirement_id = ?`, rq)
		if err != nil {
			return err
		}
		for rows.Next() {
			var x struct{ code, mode string }
			if rows.Scan(&x.code, &x.mode) == nil {
				rdocs = append(rdocs, x)
			}
		}
		rows.Close()
		if len(rdocs) == 0 {
			continue
		}
		for _, c := range pickN(e, list, 7) {
			for _, rd := range rdocs {
				ds := docByCode(docs, rd.code)
				if ds.Reusable && reusedDone[c.talent+rd.code] {
					continue // ya presentado en otro RQ: se reutiliza
				}
				scen := e.docScenario(c.stage, ds.Code == "CUL", ds.Validity > 0)
				if scen == "MISSING" {
					continue
				}
				if ds.Reusable {
					reusedDone[c.talent+rd.code] = true
				}
				jobs = append(jobs, job{c, ds, scen, rq, rd.mode})
			}
		}
	}
	fmt.Printf("%-18s %d documentos a cargar\n", spec.Slug, len(jobs))
	// una sesión por analista antes de subir (evita logins simultáneos)
	for em := range uids {
		if _, err := api.login(em); err != nil {
			return err
		}
	}
	var done, failed int
	var dmu sync.Mutex
	emails := make([]string, 0, len(uids))
	for em := range uids {
		emails = append(emails, em)
	}
	e.parallel(len(jobs), func(i int) {
		j := jobs[i]
		em := emails[i%len(emails)]
		name := strings.TrimSpace(j.c.first + " " + j.c.last + " " + j.c.second)
		docNum := ""
		if len(j.c.docEnc) > 0 {
			docNum, _ = e.cipher.Decrypt(j.c.docEnc)
		}
		if docNum == "" {
			docNum = fmt.Sprintf("4%07d", e.intn(1e7))
		}
		code := fmt.Sprintf("2026%07d", e.intn(1e7))
		label := docLabel(j.ds)
		lines := []string{label, "", "Titular: " + strings.ToUpper(name), "Documento de identidad: " + docNum,
			"Emitido: " + time.Now().AddDate(0, 0, -e.between(1, 60)).Format("02/01/2006"), "", "Documento de prueba generado para staging."}
		culCode, culDocNum := "", ""
		if j.ds.Code == "CUL" {
			lines = []string{"CERTIFICADO ÚNICO LABORAL", code, "", "Nombres y apellidos: " + strings.ToUpper(name), "N° de documento: " + docNum,
				"Vigencia: 3 meses", "", "Documento de prueba generado para staging (no es un CUL real)."}
			if j.scen != "CUL_NOCODE" {
				culCode, culDocNum = code, docNum
			}
		}
		err := api.upload(em, j.c.id, typeUUID[j.ds.Code], strings.ReplaceAll(ascii(label), " ", "_")+".pdf", simplePDF(lines), culCode, culDocNum)
		if err != nil {
			dmu.Lock()
			failed++
			dmu.Unlock()
			e.fails("subida "+j.ds.Code, err)
			return
		}
		var docID string
		if err := e.db.QueryRowContext(ctx, `SELECT td.uuid FROM talent_document td JOIN requirement_candidate rc ON rc.id = td.candidate_id
			WHERE rc.uuid = ? AND td.document_type_item_id = ? ORDER BY td.id DESC LIMIT 1`, j.c.id, typeIDs[j.ds.Code]).Scan(&docID); err != nil {
			e.fails("documento cargado", err)
			return
		}
		v := e.verdict(j.scen, j.ds, code, docNum, strings.ToUpper(name))
		if v != nil {
			e.fails("validación", e.docRepo.Verify(ctx, ports.Meta{ActorID: uids[em], IP: ip}, tid, docID, j.c.id, *v))
		}
		dmu.Lock()
		done++
		if done%200 == 0 {
			fmt.Printf("%-18s %d/%d documentos\n", spec.Slug, done, len(jobs))
		}
		dmu.Unlock()
	})
	fmt.Printf("%-18s %d documentos cargados (%d con error)\n", spec.Slug, done, failed)

	// 5) el CUL pasa a validación automática (lo nuevo que se cargue en staging se valida contra el MTPE si el tenant tiene RUC)
	for i, ds := range docs {
		if ds.Code != "CUL" {
			continue
		}
		err := e.docs.SaveRule(ctx, admin, ip, ports.DocRule{TypeID: typeUUID[ds.Code], DefaultMode: ds.Mode, Reusable: ds.Reusable, ValidityDays: ptr(ds.Validity),
			Verification: "AUTO_CUL", Active: true, SortOrder: i + 1})
		e.fails("regla CUL automática", err)
	}

	// 6) RQ reabiertos: por renuncia (baja pendiente y efectiva) y por más vacantes
	return e.reopenScenarios(ctx, tid, spec, lead)
}

func docByCode(docs []docSpec, code string) docSpec {
	for _, d := range docs {
		if d.Code == code {
			return d
		}
	}
	return docSpec{Code: code, Mode: "OPTIONAL", Reusable: true}
}

func docLabel(d docSpec) string {
	switch d.Code {
	case "CUL":
		return "Certificado Único Laboral"
	case "CERTIFICATE":
		return "Certificado o diploma"
	}
	if d.Name != "" {
		return d.Name
	}
	return d.Code
}

// verdict: resultado a registrar según el escenario (nil = queda por revisar tal como se cargó).
func (e *env) verdict(scen string, ds docSpec, code, docNum, names string) *ports.DocVerdict {
	now := time.Now()
	day := func(t time.Time) *time.Time {
		d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		return &d
	}
	culResult := func(found, vigente, match bool, extra map[string]any) json.RawMessage {
		r := map[string]any{"code": code, "docNumber": docNum, "checkedAt": now.UTC().Format(time.RFC3339)}
		if found {
			r["names"], r["vigente"], r["match"] = names, vigente, match
		}
		for k, v := range extra {
			r[k] = v
		}
		b, _ := json.Marshal(r)
		return b
	}
	switch scen {
	case "PENDING":
		return nil
	case "VERIFIED":
		return &ports.DocVerdict{Status: "VERIFIED", Method: "MANUAL"}
	case "REJECTED":
		return &ports.DocVerdict{Status: "REJECTED", Method: "MANUAL", Reason: pick(e, rejectReasons)}
	case "EXPIRED":
		issued := now.AddDate(0, 0, -(ds.Validity + e.between(5, 40)))
		return &ports.DocVerdict{Status: "VERIFIED", Method: "MANUAL", IssuedAt: day(issued), ExpiresAt: day(issued.AddDate(0, 0, ds.Validity))}
	case "CUL_OK", "CUL_EXPIRED", "CUL_ALTERED":
		age := e.between(1, 80)
		if scen == "CUL_EXPIRED" {
			age = e.between(95, 160)
		}
		issued := now.AddDate(0, 0, -age)
		status, reason := "VERIFIED", ""
		if scen == "CUL_ALTERED" {
			status, reason = "REJECTED", "CUL_FILE_ALTERED"
		}
		return &ports.DocVerdict{Status: status, Method: "AUTO", Reason: reason, IssuedAt: day(issued), ExpiresAt: day(issued.AddDate(0, 0, 90)),
			Result: culResult(true, scen != "CUL_EXPIRED", scen != "CUL_ALTERED", map[string]any{"issuedAt": issued.UTC().Format(time.RFC3339)})}
	case "CUL_OTHER":
		other := fmt.Sprintf("7%07d", e.intn(1e7))
		b, _ := json.Marshal(map[string]any{"code": code, "docNumber": other, "checkedAt": now.UTC().Format(time.RFC3339)})
		return &ports.DocVerdict{Status: "REJECTED", Method: "AUTO", Reason: "CUL_OTHER_PERSON", Result: b}
	case "CUL_NOTFOUND":
		return &ports.DocVerdict{Status: "REJECTED", Method: "AUTO", Reason: "CUL_NOT_FOUND",
			Result: culResult(false, false, false, map[string]any{"message": "No se encontró información del certificado consultado."})}
	case "CUL_UNREACHABLE":
		return &ports.DocVerdict{Status: "PENDING", Method: "AUTO", Reason: "CUL_UNREACHABLE", Result: culResult(false, false, false, nil)}
	case "CUL_NOCODE":
		b, _ := json.Marshal(map[string]any{"code": "", "docNumber": docNum, "checkedAt": now.UTC().Format(time.RFC3339)})
		return &ports.DocVerdict{Status: "PENDING", Method: "AUTO", Reason: "CUL_CODE_MISSING", Result: b}
	}
	return nil
}

// docType crea (o reutiliza) un tipo de documento propio del tenant.
func (e *env) docType(ctx context.Context, admin *domain.Principal, tid uint64, ds docSpec) (string, error) {
	_, err := e.masters.Upsert(ctx, admin, ip, ports.MasterUpsert{TenantID: &tid, TableCode: "DOCUMENT_TYPE", Code: ds.Code, Label: label(ds.Name), IsActive: true})
	if err != nil && !strings.Contains(err.Error(), "CODE_TAKEN") {
		return "", err
	}
	var id string
	err = e.db.QueryRowContext(ctx, `SELECT i.uuid FROM master_item i JOIN master_table t ON t.id = i.master_table_id
		WHERE t.code = 'DOCUMENT_TYPE' AND i.code = ? AND i.tenant_id = ? AND i.deleted_at IS NULL`, ds.Code, tid).Scan(&id)
	return id, err
}

// scenarioCandidates: postulantes vigentes (no renunciados) de los RQ que piden documentos, agrupados por RQ.
func (e *env) scenarioCandidates(ctx context.Context, tid uint64) (map[uint64][]candRow, error) {
	rows, err := e.db.QueryContext(ctx, `SELECT rc.requirement_id, rc.uuid, t.uuid, t.first_name, t.last_name, IFNULL(t.second_last_name, ''), s.code, t.doc_number_enc
		FROM requirement_candidate rc
		JOIN requirement r ON r.id = rc.requirement_id AND r.tenant_id = ? AND r.deleted_at IS NULL
		JOIN talent t ON t.id = rc.talent_id
		JOIN master_item s ON s.id = rc.stage_item_id
		WHERE rc.deleted_at IS NULL AND rc.resigned_at IS NULL
		  AND EXISTS (SELECT 1 FROM requirement_document rd WHERE rd.requirement_id = r.id)
		  AND NOT EXISTS (SELECT 1 FROM talent_document td WHERE td.candidate_id = rc.id)`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uint64][]candRow{}
	for rows.Next() {
		var rq uint64
		var c candRow
		var enc []byte
		if err := rows.Scan(&rq, &c.id, &c.talent, &c.first, &c.last, &c.second, &c.stage, &enc); err != nil {
			return nil, err
		}
		c.docEnc = enc
		out[rq] = append(out[rq], c)
	}
	return out, rows.Err()
}

func (e *env) principalsByRole(ctx context.Context, tid uint64) (admin, lead *domain.Principal, err error) {
	q := `SELECT u.id FROM app_user u JOIN user_role ur ON ur.user_id = u.id JOIN role r ON r.id = ur.role_id
		WHERE u.tenant_id = ? AND r.code = ? AND u.status = 'ACTIVE' AND u.deleted_at IS NULL ORDER BY u.id LIMIT 1`
	var aid, lid uint64
	if err = e.db.QueryRowContext(ctx, q, tid, "TENANT_ADMIN").Scan(&aid); err != nil {
		return nil, nil, fmt.Errorf("admin: %w", err)
	}
	if admin, err = e.auth.Principal(ctx, aid); err != nil {
		return nil, nil, err
	}
	if err = e.db.QueryRowContext(ctx, q, tid, "SELECTION_LEAD").Scan(&lid); err == sql.ErrNoRows {
		return admin, nil, nil // sin jefe de selección no se reabren RQ
	} else if err != nil {
		return nil, nil, err
	}
	lead, err = e.auth.Principal(ctx, lid)
	return admin, lead, err
}

// reopenScenarios reabre RQ atendidos: dos por renuncia (uno con la baja ya efectiva) y uno por más vacantes.
func (e *env) reopenScenarios(ctx context.Context, tid uint64, spec TenantSpec, lead *domain.Principal) error {
	if lead == nil {
		return nil
	}
	rows, err := e.db.QueryContext(ctx, `SELECT r.uuid, MIN(rc.uuid) FROM requirement r
		JOIN master_item s ON s.id = r.status_item_id AND s.code = 'ATTENDED'
		JOIN requirement_candidate rc ON rc.requirement_id = r.id AND rc.deleted_at IS NULL AND rc.resigned_at IS NULL
		JOIN master_item cs ON cs.id = rc.stage_item_id AND cs.code = 'CONFIRMED'
		WHERE r.tenant_id = ? AND r.deleted_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM requirement_reopen ro WHERE ro.requirement_id = r.id)
		GROUP BY r.id LIMIT 3`, tid)
	if err != nil {
		return err
	}
	type pair struct{ rq, cand string }
	var list []pair
	for rows.Next() {
		var p pair
		if rows.Scan(&p.rq, &p.cand) == nil {
			list = append(list, p)
		}
	}
	rows.Close()
	notes := []string{"El talento confirmado renunció antes de su ingreso; se reabre para reemplazarlo.",
		"Renuncia a la semana de ingresar; el cliente pide un reemplazo.", "El cliente amplía el pedido en dos vacantes."}
	for i, p := range list {
		reason, cands := "RESIGNATION", []string{p.cand}
		if i == 2 {
			reason, cands = "MORE_VACANCIES", nil
		}
		_, err := e.rqs.Reopen(ctx, lead, ip, p.rq, reason, notes[i], cands, "")
		if e.fails("reabrir RQ", err) {
			continue
		}
		if i == 1 { // la baja ya la confirmó el FMI
			_, err = e.db.ExecContext(ctx, "UPDATE requirement_candidate SET offboard_status = 'EFFECTIVE', offboard_at = NOW(3) WHERE uuid = ?", p.cand)
			e.fails("baja efectiva", err)
		}
	}
	fmt.Printf("%-18s %d RQ reabiertos\n", spec.Slug, len(list))
	return nil
}
