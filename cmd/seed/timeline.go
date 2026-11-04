package main

// Historia realista de los RQ de un tenant ya generado (para los indicadores): los datos se crean con la fecha del
// momento y aquí se reparten en los últimos 9 meses, en días hábiles:
//   - RQ cerrados: registrados hace 1,5 a 9 meses; cada postulante avanza por sus etapas con días entre una y otra;
//     el cierre llega días después del último movimiento (unos a tiempo y otros fuera de plazo).
//   - RQ abiertos: registrados en los últimos 3 meses; el último movimiento queda reciente, en atención o crítico
//     (alertas de RQ sin movimiento) y la fecha de vencimiento deja una parte vencidos.
//   - RQ reabiertos: la reapertura (y la renuncia) es el último movimiento.
//   - Entrevistas: se reemplazan por entrevistas finalizadas ligadas al RQ (filtro, R&S, técnica, cliente) entre las
//     etapas de cada postulante, registradas por un responsable del RQ, con su duración; y algunas agendadas a futuro.
// Las fechas se escriben en UTC (como las guarda la app) en lotes, porque la BD es remota.

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type tlEvent struct {
	id   uint64
	typ  string
	to   string
	when time.Time
}

type tlCand struct {
	id, talent       uint64
	vacancyProfile   string
	stage            string
	resigned, effect bool
	events           []tlEvent
	added, stageAt   time.Time
	resignedAt       time.Time
}

type tlRQ struct {
	id, client       uint64
	code, status     string
	closed           bool
	owners           []uint64
	ownerRows        []uint64 // ids de requirement_owner
	cands            []*tlCand
	reopens          []uint64
	start, due, last time.Time
	closedAt         time.Time
}

var lima = mustLoc("America/Lima")

func mustLoc(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("PET", -5*3600)
	}
	return l
}

// knownPasswords: correo -> clave de las líneas «CLAVE=correo clave» del archivo de credenciales (si existe).
func knownPasswords(path string) map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		_, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		if f := strings.Fields(v); len(f) == 2 && strings.Contains(f[0], "@") {
			out[strings.ToLower(f[0])] = f[1]
		}
	}
	return out
}

// bizDay lleva t al siguiente día hábil (lunes a viernes) si cae en fin de semana.
func bizDay(t time.Time) time.Time {
	for t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// addBiz suma n días hábiles (n puede ser fraccionario: la fracción se vuelve horas de la jornada).
func addBiz(t time.Time, n float64) time.Time {
	whole := int(math.Floor(n))
	for i := 0; i < whole; i++ {
		t = bizDay(t.AddDate(0, 0, 1))
	}
	return t.Add(time.Duration((n - float64(whole)) * 8 * float64(time.Hour)))
}

// subBiz resta n días hábiles completos.
func subBiz(t time.Time, n int) time.Time {
	for i := 0; i < n; {
		t = t.AddDate(0, 0, -1)
		if t.Weekday() != time.Saturday && t.Weekday() != time.Sunday {
			i++
		}
	}
	return t
}

// at: el día d a una hora de oficina (8:30 a 18:00, hora de Lima).
func (e *env) at(d time.Time) time.Time {
	d = d.In(lima)
	return time.Date(d.Year(), d.Month(), d.Day(), 8+e.intn(10), e.intn(60), e.intn(60), 0, lima)
}

func (e *env) timeline(ctx context.Context, spec TenantSpec) error {
	var tid uint64
	if err := e.db.QueryRowContext(ctx, "SELECT id FROM tenant WHERE slug = ? AND deleted_at IS NULL", spec.Slug).Scan(&tid); err != nil {
		return fmt.Errorf("tenant: %w", err)
	}
	rqs, err := e.tlLoad(ctx, tid)
	if err != nil {
		return err
	}
	now := time.Now().In(lima)
	var nIv int
	var ivRows []string
	m, err := e.loadMasters(ctx, tid)
	if err != nil {
		return err
	}
	durations := map[string]int{"PHONE_SCREEN": 30, "RS_TEAM": 45, "TECH_LEAD": 60, "TECH_CLIENT": 60}
	durID := map[int]uint64{}
	if err := e.tlDurations(ctx, durID); err != nil {
		return err
	}
	ids := func(table, code string) (uint64, error) {
		var id uint64
		err := e.db.QueryRowContext(ctx, "SELECT id FROM master_item WHERE uuid = ?", m.id(table, code)).Scan(&id)
		return id, err
	}
	stageIDs, typeIDs := map[string]uint64{}, map[string]uint64{}
	for _, c := range []string{"PHONE_SCREEN", "RS_TEAM", "TECH_LEAD", "TECH_CLIENT"} {
		if stageIDs[c], err = ids("INTERVIEW_STAGE", c); err != nil {
			return fmt.Errorf("etapa de entrevista %s: %w", c, err)
		}
	}
	for _, c := range []string{"VIRTUAL", "IN_PERSON", "PHONE"} {
		if typeIDs[c], err = ids("INTERVIEW_TYPE", c); err != nil {
			return fmt.Errorf("tipo de entrevista %s: %w", c, err)
		}
	}
	finished, err := ids("INTERVIEW_STATUS", "FINISHED")
	if err != nil {
		return err
	}
	registered, err := ids("INTERVIEW_STATUS", "REGISTERED")
	if err != nil {
		return err
	}

	for _, r := range rqs {
		e.tlPlan(r, now)
		// entrevistas de cada postulante entre sus etapas
		for _, c := range r.cands {
			if len(r.owners) == 0 {
				break
			}
			var plan []string
			reached := c.stage
			for _, ev := range c.events {
				if ev.to == "CONFIRMED" || ev.to == "INTERVIEW" {
					reached = ev.to
				}
			}
			switch reached {
			case "CONFIRMED":
				plan = []string{"RS_TEAM", "TECH_LEAD", "TECH_CLIENT"}[:2+e.intn(2)]
				if e.chance(0.4) {
					plan = append([]string{"PHONE_SCREEN"}, plan...)
				}
			case "INTERVIEW":
				plan = []string{"RS_TEAM", "TECH_LEAD"}[:1+e.intn(2)]
			case "DATA_COMPLETE":
				if e.chance(0.3) {
					plan = []string{"PHONE_SCREEN"}
				}
			}
			if len(plan) == 0 {
				continue
			}
			// ventana: desde el alta hasta el último movimiento (o hasta hoy si sigue en entrevista)
			from, to := c.added, c.stageAt
			if reached == "INTERVIEW" && !r.closed {
				to = now
			}
			if !to.After(from) {
				to = from.Add(4 * time.Hour)
			}
			span := to.Sub(from)
			for k, st := range plan {
				when := e.at(bizDay(from.Add(time.Duration(float64(span) * (float64(k) + 0.5) / float64(len(plan))))))
				status := finished
				future := false
				if when.After(now) {
					status, future = registered, true
				}
				// algunas entrevistas de quienes siguen en entrevista quedan agendadas para los próximos días
				if !r.closed && reached == "INTERVIEW" && k == len(plan)-1 && e.chance(0.35) {
					when, status, future = e.at(addBiz(now, float64(1+e.intn(5)))), registered, true
				}
				dur := durations[st]
				if st == "TECH_CLIENT" && e.chance(0.4) {
					dur = 90
				}
				typ := typeIDs["VIRTUAL"]
				if st == "PHONE_SCREEN" {
					typ = typeIDs["PHONE"]
				} else if e.chance(0.2) {
					typ = typeIDs["IN_PERSON"]
				}
				owner := r.owners[e.intn(len(r.owners))]
				created := subBiz(when, 1+e.intn(3))
				rating := func() int {
					if future {
						return 0
					}
					return 3 + e.intn(3)
				}
				ivRows = append(ivRows, fmt.Sprintf("(%s, %d, %d, %d, %d, %d, %d, %d, %s, %d, %s, %s, %s, %s, %d, %d, %d, %d, %d, %s, %s, %d)",
					q(uuid.NewString()), tid, c.talent, typ, stageIDs[st], status, durID[dur], r.client, q(r.code), r.id, q(c.vacancyProfile),
					q(when.Format("2006-01-02")), q(when.Format("15:04:00")),
					q("https://teams.microsoft.com/l/meetup-join/19%3ameeting_"+strings.ReplaceAll(uuid.NewString(), "-", "")[:24]),
					rating(), rating(), rating(), rating(), rating(),
					q(utc(created)), q(utc(created)), owner))
				nIv++
			}
		}
	}

	// escritura en lotes
	tx := func(stmts ...string) error {
		for _, s := range stmts {
			if _, err := e.db.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("%.120s: %w", s, err)
			}
		}
		return nil
	}
	var rqRows, ownRows, candRows, evRows, reoRows []string
	for _, r := range rqs {
		closed := "NULL"
		if r.closed {
			closed = q(utc(r.closedAt))
		}
		rqRows = append(rqRows, fmt.Sprintf("ROW(%d, %s, %s, %s, %s, %s)", r.id, q(r.start.Format("2006-01-02")), q(r.due.Format("2006-01-02")),
			q(utc(r.start)), q(utc(r.last)), closed))
		for _, o := range r.ownerRows {
			ownRows = append(ownRows, fmt.Sprintf("ROW(%d, %s)", o, q(utc(r.start))))
		}
		for _, c := range r.cands {
			res := "NULL"
			off := "NULL"
			if c.resigned {
				res = q(utc(c.resignedAt))
				if c.effect {
					off = q(utc(addBiz(c.resignedAt, 3)))
				}
			}
			candRows = append(candRows, fmt.Sprintf("ROW(%d, %s, %s, %s, %s)", c.id, q(utc(c.added)), q(utc(c.stageAt)), res, off))
			for _, ev := range c.events {
				evRows = append(evRows, fmt.Sprintf("ROW(%d, %s)", ev.id, q(utc(ev.when))))
			}
		}
		for _, ro := range r.reopens {
			reoRows = append(reoRows, fmt.Sprintf("ROW(%d, %s)", ro, q(utc(r.last))))
		}
	}
	err = batch(rqRows, func(v string) error {
		return tx(`UPDATE requirement r JOIN (VALUES ` + v + `) AS x(id, req, due, created, upd, closed) ON x.id = r.id
			SET r.request_date = x.req, r.due_date = x.due, r.created_at = x.created, r.updated_at = x.upd, r.closed_at = x.closed, r.stale_level = 0`)
	})
	if err == nil {
		err = batch(ownRows, func(v string) error {
			return tx(`UPDATE requirement_owner o JOIN (VALUES ` + v + `) AS x(id, at) ON x.id = o.id
				SET o.assigned_at = x.at, o.unassigned_at = IF(o.unassigned_at IS NULL, NULL, x.at + INTERVAL 1 HOUR)`)
		})
	}
	if err == nil {
		err = batch(candRows, func(v string) error {
			return tx(`UPDATE requirement_candidate c JOIN (VALUES ` + v + `) AS x(id, added, st, res, off) ON x.id = c.id
				SET c.created_at = x.added, c.stage_at = x.st, c.updated_at = x.st, c.resigned_at = IF(c.resigned_at IS NULL, NULL, x.res),
				    c.offboard_at = IF(c.offboard_at IS NULL, NULL, x.off)`)
		})
	}
	if err == nil {
		err = batch(evRows, func(v string) error {
			return tx(`UPDATE requirement_candidate_event ev JOIN (VALUES ` + v + `) AS x(id, at) ON x.id = ev.id SET ev.created_at = x.at`)
		})
	}
	if err == nil {
		err = batch(reoRows, func(v string) error {
			return tx(`UPDATE requirement_reopen ro JOIN (VALUES ` + v + `) AS x(id, at) ON x.id = ro.id SET ro.created_at = x.at`)
		})
	}
	if err != nil {
		return err
	}
	// entrevistas: se reemplazan las anteriores del tenant por las ligadas a los RQ
	if err := tx(
		fmt.Sprintf("DELETE a FROM interview_answer a JOIN interview i ON i.id = a.interview_id WHERE i.tenant_id = %d", tid),
		fmt.Sprintf("DELETE a FROM interview_interviewer a JOIN interview i ON i.id = a.interview_id WHERE i.tenant_id = %d", tid),
		fmt.Sprintf("DELETE a FROM interview_recording a JOIN interview i ON i.id = a.interview_id WHERE i.tenant_id = %d", tid),
		fmt.Sprintf("DELETE FROM interview WHERE tenant_id = %d", tid),
	); err != nil {
		return err
	}
	if err := batch(ivRows, func(v string) error {
		return tx(`INSERT INTO interview (uuid, tenant_id, talent_id, type_item_id, stage_item_id, status_item_id, duration_item_id, client_item_id,
			requirement_ref, requirement_id, profile, scheduled_date, scheduled_time, meeting_link,
			rating_general, rating_personal, rating_experience, rating_languages, rating_education, created_at, updated_at, created_by) VALUES ` + v)
	}); err != nil {
		return err
	}
	closed, open := 0, 0
	for _, r := range rqs {
		if r.closed {
			closed++
		} else {
			open++
		}
	}
	fmt.Printf("%-18s %d RQ (%d cerrados, %d abiertos), %d postulantes, %d eventos, %d entrevistas\n", spec.Slug, len(rqs), closed, open, len(candRows), len(evRows), nIv)
	return nil
}

// tlPlan reparte en el tiempo el RQ, sus postulantes y eventos.
func (e *env) tlPlan(r *tlRQ, now time.Time) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, lima)
	reopened := len(r.reopens) > 0
	// fin de la actividad: cerrados, antes del cierre; abiertos, hace 0 a 25 días hábiles (reciente, atención o crítico)
	var end time.Time
	switch {
	case r.closed:
		r.start = e.at(bizDay(today.AddDate(0, 0, -(45 + e.intn(225)))))
	default:
		r.start = e.at(bizDay(today.AddDate(0, 0, -(5 + e.intn(85)))))
		if r.status == "REGISTERED" {
			r.start = e.at(bizDay(today.AddDate(0, 0, -e.intn(20))))
		}
	}
	if r.start.After(now) {
		r.start = now.Add(-2 * time.Hour)
	}
	// días hábiles de actividad del RQ (alta del primero al último movimiento)
	active := float64(8 + e.intn(30))
	if r.closed {
		end = addBiz(r.start, active)
		limit := subBiz(today, 2)
		if end.After(limit) {
			end = limit
		}
	} else {
		idle := 0
		switch p := e.float(); {
		case p < 0.45:
			idle = e.intn(4) // reciente
		case p < 0.8:
			idle = 5 + e.intn(5) // atención
		default:
			idle = 10 + e.intn(15) // crítico
		}
		end = e.at(subBiz(today, idle))
		if end.After(now) {
			end = now.Add(-time.Hour)
		}
	}
	if !end.After(r.start) {
		end = r.start.Add(6 * time.Hour)
	}
	// con reapertura, la actividad previa termina antes y la reapertura es el último movimiento
	actEnd := end
	if reopened {
		actEnd = r.start.Add(time.Duration(float64(end.Sub(r.start)) * 0.7))
	}
	span := actEnd.Sub(r.start)
	r.last = r.start
	for _, c := range r.cands {
		// alta repartida en la primera mitad; cada etapa ocupa una parte del tiempo restante
		addFrac := e.float() * 0.5
		if len(r.cands) > 0 && c == r.cands[0] {
			addFrac = e.float() * 0.08 // el primer postulante llega pronto
		}
		c.added = r.start.Add(time.Duration(float64(span) * addFrac))
		t := c.added
		rest := actEnd.Sub(c.added)
		steps := len(c.events)
		for k := range c.events {
			if k == 0 {
				c.events[k].when = c.added
				continue
			}
			frac := (float64(k) + e.float()*0.6) / float64(steps)
			when := c.added.Add(time.Duration(float64(rest) * frac))
			if !when.After(t) {
				when = t.Add(time.Duration(30+e.intn(240)) * time.Minute)
			}
			c.events[k].when = when
			t = when
		}
		c.stageAt = c.added
		for _, ev := range c.events {
			if (ev.typ == "ADDED" || ev.typ == "STAGE") && ev.when.After(c.stageAt) {
				c.stageAt = ev.when
			}
		}
		if t.After(r.last) {
			r.last = t
		}
	}
	// el último movimiento de un RQ abierto con postulantes cae en la fecha elegida (estado de la alerta)
	if !r.closed && !reopened && len(r.cands) > 0 {
		shift := end.Sub(r.last)
		if shift > 0 {
			for _, c := range r.cands {
				for k := range c.events {
					if k > 0 {
						c.events[k].when = c.events[k].when.Add(shift)
					}
				}
				c.stageAt = c.added
				for _, ev := range c.events {
					if ev.when.After(c.stageAt) {
						c.stageAt = ev.when
					}
				}
			}
			r.last = end
		}
	}
	if reopened {
		r.last = end
		for _, c := range r.cands {
			if c.resigned {
				c.resignedAt = end
				for k := range c.events {
					if c.events[k].typ == "RESIGNED" {
						c.events[k].when = end
					}
				}
			}
		}
	}
	if r.closed {
		r.closedAt = e.at(addBiz(r.last, float64(1+e.intn(8))))
		if r.closedAt.After(now) {
			r.closedAt = now.Add(-time.Hour)
		}
		closeDay := time.Date(r.closedAt.Year(), r.closedAt.Month(), r.closedAt.Day(), 0, 0, 0, 0, lima)
		if e.chance(0.6) {
			r.due = closeDay.AddDate(0, 0, e.intn(12)) // a tiempo
		} else {
			r.due = closeDay.AddDate(0, 0, -(1 + e.intn(20))) // fuera de plazo
		}
		if r.due.Before(r.start) {
			r.due = r.start.AddDate(0, 0, 7)
		}
		if r.closedAt.After(r.last) {
			r.last = r.closedAt
		}
	} else {
		r.due = today.AddDate(0, 0, -25+e.intn(70))
		if min := r.start.AddDate(0, 0, 10); r.due.Before(min) {
			r.due = min
		}
	}
}

func (e *env) tlLoad(ctx context.Context, tid uint64) ([]*tlRQ, error) {
	byID := map[uint64]*tlRQ{}
	var list []*tlRQ
	rows, err := e.db.QueryContext(ctx, `SELECT r.id, r.code, r.client_item_id, s.code, s.str1 = 'CLOSED'
		FROM requirement r JOIN master_item s ON s.id = r.status_item_id WHERE r.tenant_id = ? AND r.deleted_at IS NULL ORDER BY r.id`, tid)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		r := &tlRQ{}
		if err := rows.Scan(&r.id, &r.code, &r.client, &r.status, &r.closed); err != nil {
			rows.Close()
			return nil, err
		}
		byID[r.id] = r
		list = append(list, r)
	}
	rows.Close()
	if err := e.tlEach(ctx, `SELECT o.id, o.requirement_id, o.user_id, o.unassigned_at IS NULL FROM requirement_owner o
		JOIN requirement r ON r.id = o.requirement_id WHERE r.tenant_id = ?`, tid, func(rs *sql.Rows) error {
		var id, rq, user uint64
		var active bool
		if err := rs.Scan(&id, &rq, &user, &active); err != nil {
			return err
		}
		if r := byID[rq]; r != nil {
			r.ownerRows = append(r.ownerRows, id)
			if active {
				r.owners = append(r.owners, user)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	cands := map[uint64]*tlCand{}
	if err := e.tlEach(ctx, `SELECT c.id, c.requirement_id, c.talent_id, cs.code, c.resigned_at IS NOT NULL, IFNULL(c.offboard_status, '') = 'EFFECTIVE',
		JSON_UNQUOTE(JSON_EXTRACT(p.label_i18n, '$.es'))
		FROM requirement_candidate c JOIN requirement r ON r.id = c.requirement_id JOIN master_item cs ON cs.id = c.stage_item_id
		JOIN requirement_vacancy v ON v.id = c.vacancy_id JOIN master_item p ON p.id = v.profile_item_id
		WHERE r.tenant_id = ? AND c.deleted_at IS NULL ORDER BY c.id`, tid, func(rs *sql.Rows) error {
		c := &tlCand{}
		var rq uint64
		if err := rs.Scan(&c.id, &rq, &c.talent, &c.stage, &c.resigned, &c.effect, &c.vacancyProfile); err != nil {
			return err
		}
		if r := byID[rq]; r != nil {
			r.cands = append(r.cands, c)
			cands[c.id] = c
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := e.tlEach(ctx, `SELECT ev.id, ev.candidate_id, ev.type, IFNULL(JSON_UNQUOTE(JSON_EXTRACT(ev.data, '$.to')), '')
		FROM requirement_candidate_event ev JOIN requirement_candidate c ON c.id = ev.candidate_id JOIN requirement r ON r.id = c.requirement_id
		WHERE r.tenant_id = ? ORDER BY ev.id`, tid, func(rs *sql.Rows) error {
		var ev tlEvent
		var cid uint64
		if err := rs.Scan(&ev.id, &cid, &ev.typ, &ev.to); err != nil {
			return err
		}
		if c := cands[cid]; c != nil {
			c.events = append(c.events, ev)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := e.tlEach(ctx, `SELECT ro.id, ro.requirement_id FROM requirement_reopen ro JOIN requirement r ON r.id = ro.requirement_id
		WHERE r.tenant_id = ?`, tid, func(rs *sql.Rows) error {
		var id, rq uint64
		if err := rs.Scan(&id, &rq); err != nil {
			return err
		}
		if r := byID[rq]; r != nil {
			r.reopens = append(r.reopens, id)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// el orden de los eventos de cada postulante es el de su id (alta, etapas, renuncia)
	for _, c := range cands {
		sort.Slice(c.events, func(i, j int) bool { return c.events[i].id < c.events[j].id })
	}
	return list, nil
}

func (e *env) tlEach(ctx context.Context, query string, tid uint64, fn func(*sql.Rows) error) error {
	rs, err := e.db.QueryContext(ctx, query, tid)
	if err != nil {
		return err
	}
	defer rs.Close()
	for rs.Next() {
		if err := fn(rs); err != nil {
			return err
		}
	}
	return rs.Err()
}

func (e *env) tlDurations(ctx context.Context, out map[int]uint64) error {
	rs, err := e.db.QueryContext(ctx, `SELECT i.id, CAST(i.num1 AS UNSIGNED) FROM master_item i JOIN master_table t ON t.id = i.master_table_id
		WHERE t.code = 'INTERVIEW_DURATION' AND i.tenant_id IS NULL AND i.deleted_at IS NULL`)
	if err != nil {
		return err
	}
	defer rs.Close()
	for rs.Next() {
		var id uint64
		var mins int
		if err := rs.Scan(&id, &mins); err != nil {
			return err
		}
		out[mins] = id
	}
	for _, need := range []int{30, 45, 60, 90} {
		if out[need] == 0 {
			return fmt.Errorf("falta la duración de entrevista de %d minutos", need)
		}
	}
	return rs.Err()
}

// batch ejecuta fn con las filas unidas de a 800 (una consulta por lote).
func batch(rows []string, fn func(string) error) error {
	for i := 0; i < len(rows); i += 800 {
		j := min(i+800, len(rows))
		if err := fn(strings.Join(rows[i:j], ", ")); err != nil {
			return err
		}
	}
	return nil
}

func utc(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05.000") }

// q: literal SQL de texto (solo datos generados aquí: uuid, fechas, códigos y perfiles del catálogo).
func q(s string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", "''") + "'"
}
