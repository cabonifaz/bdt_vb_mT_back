-- =====================================================================
-- Indicadores de selección (jefe de selección y administrador). Un solo documento JSON por período.
-- Definiciones:
--   ingreso (alta)  = postulante confirmado (cambio de etapa a Confirmado) dentro del período; cuenta aunque luego renuncie.
--   éxito           = RQ cerrado como Atendido.
--   analista        = responsable del RQ al momento del ingreso (historial de responsables); el jefe de selección cuenta
--                     como analista. Un RQ con 2 responsables cuenta completo para cada uno.
--   reunión         = entrevista finalizada registrada por el analista (fecha de la entrevista dentro del período).
--   horas por ingreso = duración de las entrevistas finalizadas de los RQ con ingresos / ingresos de esos RQ.
-- Fechas en la hora local del tenant; p_grain WEEK (semanas de lunes) o MONTH. p_client_uuid filtra por cliente.
-- =====================================================================
DELIMITER $$

DROP PROCEDURE IF EXISTS sp_dash_selection $$
CREATE PROCEDURE sp_dash_selection(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_from DATE, IN p_to DATE,
  IN p_grain VARCHAR(5), IN p_client_uuid CHAR(36), IN p_today DATE)
BEGIN
  DECLARE v_tz VARCHAR(60);
  DECLARE v_client BIGINT UNSIGNED;
  DECLARE v_from_utc, v_to_utc DATETIME(3);
  DECLARE v_warn, v_crit INT;
  DECLARE v_notify TINYINT(1);
  DECLARE v_overdue, v_closed, v_hires, v_per_hire, v_closing, v_reasons, v_analysts, v_series, v_funnel, v_multi,
          v_multi_top, v_clients, v_profiles, v_quality, v_speed JSON;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'dashboard.selection');
  IF p_from IS NULL OR p_to IS NULL OR p_to < p_from OR DATEDIFF(p_to, p_from) > 731 OR p_grain NOT IN ('WEEK', 'MONTH') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION';
  END IF;
  SELECT timezone INTO v_tz FROM tenant WHERE id = p_tenant_id;
  SET v_from_utc = IFNULL(CONVERT_TZ(TIMESTAMP(p_from), v_tz, '+00:00'), TIMESTAMP(p_from));
  SET v_to_utc = IFNULL(CONVERT_TZ(TIMESTAMP(p_to + INTERVAL 1 DAY), v_tz, '+00:00'), TIMESTAMP(p_to + INTERVAL 1 DAY));
  IF p_client_uuid IS NOT NULL THEN
    CALL sp_master_resolve(p_tenant_id, 'CLIENT', p_client_uuid, v_client);
    IF v_client IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  END IF;
  CALL sp_rq_alert_days(p_tenant_id, v_warn, v_crit, v_notify);

  -- ingresos del período (último paso a Confirmado de cada postulante)
  DROP TEMPORARY TABLE IF EXISTS tmp_dash_hire;
  CREATE TEMPORARY TABLE tmp_dash_hire (candidate_id BIGINT UNSIGNED PRIMARY KEY, rq_id BIGINT UNSIGNED, talent_id BIGINT UNSIGNED,
    vacancy_id BIGINT UNSIGNED, hired_at DATETIME(3), hired_day DATE, days_to_hire INT, KEY (rq_id)) ENGINE=MEMORY;
  INSERT INTO tmp_dash_hire
  SELECT rc.id, rc.requirement_id, rc.talent_id, rc.vacancy_id, MAX(e.created_at), DATE(IFNULL(CONVERT_TZ(MAX(e.created_at), '+00:00', v_tz), MAX(e.created_at))),
         DATEDIFF(DATE(IFNULL(CONVERT_TZ(MAX(e.created_at), '+00:00', v_tz), MAX(e.created_at))), DATE(IFNULL(CONVERT_TZ(rc.created_at, '+00:00', v_tz), rc.created_at)))
    FROM requirement_candidate_event e
    JOIN requirement_candidate rc ON rc.id = e.candidate_id AND rc.deleted_at IS NULL
    JOIN requirement r ON r.id = rc.requirement_id AND r.tenant_id = p_tenant_id AND r.deleted_at IS NULL
   WHERE e.type = 'STAGE' AND JSON_UNQUOTE(JSON_EXTRACT(e.data, '$.to')) = 'CONFIRMED'
     AND e.created_at >= v_from_utc AND e.created_at < v_to_utc AND (v_client IS NULL OR r.client_item_id = v_client)
   GROUP BY rc.id, rc.requirement_id, rc.talent_id, rc.vacancy_id, rc.created_at;

  DROP TEMPORARY TABLE IF EXISTS tmp_dash_hire_rq;
  CREATE TEMPORARY TABLE tmp_dash_hire_rq (rq_id BIGINT UNSIGNED PRIMARY KEY, hires INT) ENGINE=MEMORY;
  INSERT INTO tmp_dash_hire_rq SELECT rq_id, COUNT(*) FROM tmp_dash_hire GROUP BY rq_id;

  -- por ingreso: entrevistas, horas y postulantes de los RQ con ingresos
  SELECT JSON_OBJECT('hires', IFNULL(SUM(h.hires), 0),
           'candidates', IFNULL(SUM((SELECT COUNT(*) FROM requirement_candidate rc WHERE rc.requirement_id = h.rq_id AND rc.deleted_at IS NULL)), 0),
           'interviews', IFNULL(SUM((SELECT COUNT(*) FROM interview iv JOIN master_item st ON st.id = iv.status_item_id AND st.code = 'FINISHED'
                                     WHERE iv.requirement_id = h.rq_id AND iv.deleted_at IS NULL)), 0),
           'interviewMinutes', IFNULL(SUM((SELECT SUM(IFNULL(du.num1, 45)) FROM interview iv JOIN master_item st ON st.id = iv.status_item_id AND st.code = 'FINISHED'
                                          LEFT JOIN master_item du ON du.id = iv.duration_item_id
                                          WHERE iv.requirement_id = h.rq_id AND iv.deleted_at IS NULL)), 0),
           'rqs', COUNT(*))
    INTO v_per_hire FROM tmp_dash_hire_rq h;
  SELECT JSON_OBJECT('hires', COUNT(*), 'avgDaysToHire', ROUND(AVG(days_to_hire), 1),
           'talents', COUNT(DISTINCT talent_id))
    INTO v_hires FROM tmp_dash_hire;

  -- vencidos: abiertos hoy con la fecha pasada, sin y con postulantes; cerrados en el período tarde o a tiempo
  SELECT JSON_OBJECT(
           'openNoCandidates', IFNULL(SUM(x.cands = 0), 0), 'openWithCandidates', IFNULL(SUM(x.cands > 0), 0),
           'openWithCandidatesNoHire', IFNULL(SUM(x.cands > 0 AND x.confirmed = 0), 0), 'openTotal', COUNT(*))
    INTO v_overdue
    FROM (SELECT r.id,
                 (SELECT COUNT(*) FROM requirement_candidate rc WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL) AS cands,
                 (SELECT COUNT(*) FROM requirement_candidate rc JOIN master_item cs ON cs.id = rc.stage_item_id
                   WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL AND rc.resigned_at IS NULL AND cs.code = 'CONFIRMED') AS confirmed
            FROM requirement r JOIN master_item s ON s.id = r.status_item_id AND s.str1 = 'OPEN'
           WHERE r.tenant_id = p_tenant_id AND r.deleted_at IS NULL AND r.due_date < p_today AND (v_client IS NULL OR r.client_item_id = v_client)) x;

  -- cerrados en el período
  DROP TEMPORARY TABLE IF EXISTS tmp_dash_closed;
  CREATE TEMPORARY TABLE tmp_dash_closed (rq_id BIGINT UNSIGNED PRIMARY KEY, reason_id BIGINT UNSIGNED, days INT, late TINYINT,
    cands INT, confirmed INT, vacancies INT) ENGINE=MEMORY;
  INSERT INTO tmp_dash_closed
  SELECT r.id, r.status_item_id, DATEDIFF(DATE(IFNULL(CONVERT_TZ(r.closed_at, '+00:00', v_tz), r.closed_at)), r.request_date), DATE(IFNULL(CONVERT_TZ(r.closed_at, '+00:00', v_tz), r.closed_at)) > r.due_date,
         (SELECT COUNT(*) FROM requirement_candidate rc WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL),
         (SELECT COUNT(*) FROM requirement_candidate rc JOIN master_item cs ON cs.id = rc.stage_item_id
           WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL AND rc.resigned_at IS NULL AND cs.code = 'CONFIRMED'),
         (SELECT IFNULL(SUM(v.quantity), 0) FROM requirement_vacancy v WHERE v.requirement_id = r.id AND v.deleted_at IS NULL)
    FROM requirement r JOIN master_item s ON s.id = r.status_item_id AND s.str1 = 'CLOSED'
   WHERE r.tenant_id = p_tenant_id AND r.deleted_at IS NULL AND r.closed_at IS NOT NULL AND r.closed_at >= v_from_utc AND r.closed_at < v_to_utc AND (v_client IS NULL OR r.client_item_id = v_client);
  SELECT JSON_OBJECT('closed', COUNT(*), 'avgDaysToClose', ROUND(AVG(c.days), 1), 'late', IFNULL(SUM(c.late), 0),
           'onTime', IFNULL(SUM(c.late = 0), 0), 'vacancies', IFNULL(SUM(c.vacancies), 0), 'covered', IFNULL(SUM(LEAST(c.confirmed, c.vacancies)), 0),
           'success', IFNULL(SUM(s.code = 'ATTENDED'), 0),
           'successAvgCandidates', ROUND(AVG(IF(s.code = 'ATTENDED', c.cands, NULL)), 1),
           'successAvgDays', ROUND(AVG(IF(s.code = 'ATTENDED', c.days, NULL)), 1))
    INTO v_closing FROM tmp_dash_closed c JOIN master_item s ON s.id = c.reason_id;
  SELECT IFNULL(JSON_ARRAYAGG(JSON_OBJECT('code', x.code, 'label', x.label_i18n, 'count', x.n, 'avgDays', x.d, 'avgCandidates', x.ac)), JSON_ARRAY())
    INTO v_reasons
    FROM (SELECT s.code, s.label_i18n, s.sort_order, COUNT(*) n, ROUND(AVG(c.days), 1) d, ROUND(AVG(c.cands), 1) ac
            FROM tmp_dash_closed c JOIN master_item s ON s.id = c.reason_id GROUP BY s.id, s.code, s.label_i18n, s.sort_order
           ORDER BY s.sort_order) x;

  -- velocidad: días hasta el primer postulante de los RQ registrados en el período
  SELECT JSON_OBJECT('created', COUNT(*), 'withCandidates', IFNULL(SUM(x.first_at IS NOT NULL), 0),
           'avgDaysToFirstCandidate', ROUND(AVG(DATEDIFF(DATE(IFNULL(CONVERT_TZ(x.first_at, '+00:00', v_tz), x.first_at)), DATE(IFNULL(CONVERT_TZ(x.created_at, '+00:00', v_tz), x.created_at)))), 1))
    INTO v_speed
    FROM (SELECT r.created_at, (SELECT MIN(rc.created_at) FROM requirement_candidate rc WHERE rc.requirement_id = r.id) AS first_at
            FROM requirement r
           WHERE r.tenant_id = p_tenant_id AND r.deleted_at IS NULL AND r.created_at >= v_from_utc AND r.created_at < v_to_utc AND (v_client IS NULL OR r.client_item_id = v_client)) x;

  -- analistas (incluye al jefe de selección): ingresos, reuniones y carga actual
  DROP TEMPORARY TABLE IF EXISTS tmp_dash_an;
  CREATE TEMPORARY TABLE tmp_dash_an (user_id BIGINT UNSIGNED, bucket DATE, hires INT, meetings INT, minutes INT) ENGINE=MEMORY;
  INSERT INTO tmp_dash_an
  SELECT ro.user_id, IF(p_grain = 'WEEK', h.hired_day - INTERVAL WEEKDAY(h.hired_day) DAY, DATE(DATE_FORMAT(h.hired_day, '%Y-%m-01'))), 1, 0, 0
    FROM tmp_dash_hire h
    JOIN requirement_owner ro ON ro.requirement_id = h.rq_id AND ro.assigned_at <= h.hired_at AND (ro.unassigned_at IS NULL OR ro.unassigned_at > h.hired_at);
  INSERT INTO tmp_dash_an
  SELECT iv.created_by, IF(p_grain = 'WEEK', iv.scheduled_date - INTERVAL WEEKDAY(iv.scheduled_date) DAY, DATE(DATE_FORMAT(iv.scheduled_date, '%Y-%m-01'))), 0, 1, IFNULL(du.num1, 45)
    FROM interview iv
    JOIN master_item st ON st.id = iv.status_item_id AND st.code = 'FINISHED'
    LEFT JOIN master_item du ON du.id = iv.duration_item_id
    LEFT JOIN requirement r ON r.id = iv.requirement_id
   WHERE iv.tenant_id = p_tenant_id AND iv.deleted_at IS NULL AND iv.created_by IS NOT NULL
     AND iv.scheduled_date BETWEEN p_from AND p_to AND (v_client IS NULL OR iv.client_item_id = v_client OR r.client_item_id = v_client);

  SELECT IFNULL(JSON_ARRAYAGG(JSON_OBJECT('id', u.uuid, 'nameEnc', TO_BASE64(u.full_name_enc), 'lead', u.is_lead = 1,
           'hires', IFNULL(a.hires, 0), 'meetings', IFNULL(a.meetings, 0), 'meetingMinutes', IFNULL(a.minutes, 0),
           'series', IFNULL(a.series, JSON_ARRAY()),
           'openRqs', (SELECT COUNT(*) FROM requirement_owner ro JOIN requirement r ON r.id = ro.requirement_id AND r.deleted_at IS NULL
                         JOIN master_item s ON s.id = r.status_item_id AND s.str1 = 'OPEN'
                        WHERE ro.user_id = u.id AND ro.unassigned_at IS NULL AND (v_client IS NULL OR r.client_item_id = v_client)),
           'overdueRqs', (SELECT COUNT(*) FROM requirement_owner ro JOIN requirement r ON r.id = ro.requirement_id AND r.deleted_at IS NULL
                         JOIN master_item s ON s.id = r.status_item_id AND s.str1 = 'OPEN'
                        WHERE ro.user_id = u.id AND ro.unassigned_at IS NULL AND r.due_date < p_today AND (v_client IS NULL OR r.client_item_id = v_client)),
           'staleRqs', (SELECT COUNT(*) FROM requirement_owner ro JOIN requirement r ON r.id = ro.requirement_id AND r.deleted_at IS NULL
                         JOIN master_item s ON s.id = r.status_item_id AND s.str1 = 'OPEN'
                         JOIN v_rq_activity act ON act.requirement_id = r.id AND act.in_process > 0
                        WHERE ro.user_id = u.id AND ro.unassigned_at IS NULL AND (v_client IS NULL OR r.client_item_id = v_client)
                          AND (5 * (DATEDIFF(p_today, '2000-12-31') DIV 7) + LEAST(DATEDIFF(p_today, '2000-12-31') MOD 7, 5)) - (5 * (DATEDIFF(act.last_move_day, '2000-12-31') DIV 7) + LEAST(DATEDIFF(act.last_move_day, '2000-12-31') MOD 7, 5)) >= v_warn),
           'inProcess', (SELECT COUNT(*) FROM requirement_owner ro JOIN requirement r ON r.id = ro.requirement_id AND r.deleted_at IS NULL
                         JOIN master_item s ON s.id = r.status_item_id AND s.str1 = 'OPEN'
                         JOIN requirement_candidate rc ON rc.requirement_id = r.id AND rc.deleted_at IS NULL AND rc.resigned_at IS NULL
                         JOIN master_item cs ON cs.id = rc.stage_item_id AND cs.code <> 'CONFIRMED'
                        WHERE ro.user_id = u.id AND ro.unassigned_at IS NULL AND (v_client IS NULL OR r.client_item_id = v_client))
         )), JSON_ARRAY())
    INTO v_analysts
    FROM (SELECT DISTINCT u.id, u.uuid, u.full_name_enc, ro.code = 'SELECTION_LEAD' AS is_lead
            FROM app_user u JOIN user_role ur ON ur.user_id = u.id JOIN role ro ON ro.id = ur.role_id
           WHERE u.tenant_id = p_tenant_id AND u.deleted_at IS NULL AND u.status = 'ACTIVE' AND ro.code IN ('RECRUITER', 'SELECTION_LEAD')) u
    LEFT JOIN (SELECT t.user_id, SUM(t.hires) hires, SUM(t.meetings) meetings, SUM(t.minutes) minutes,
                      JSON_ARRAYAGG(JSON_OBJECT('period', t.bucket, 'hires', t.hires, 'meetings', t.meetings)) series
                 FROM (SELECT user_id, bucket, SUM(hires) hires, SUM(meetings) meetings, SUM(minutes) minutes
                         FROM tmp_dash_an GROUP BY user_id, bucket) t
                GROUP BY t.user_id) a ON a.user_id = u.id;

  -- serie del período: ingresos, reuniones, RQ registrados y cerrados
  SELECT IFNULL(JSON_ARRAYAGG(JSON_OBJECT('period', x.bucket, 'hires', x.hires, 'meetings', x.meetings, 'created', x.created, 'closed', x.closed)), JSON_ARRAY())
    INTO v_series
    FROM (SELECT b.bucket, SUM(b.hires) hires, SUM(b.meetings) meetings, SUM(b.created) created, SUM(b.closed) closed
            FROM (SELECT IF(p_grain = 'WEEK', h.hired_day - INTERVAL WEEKDAY(h.hired_day) DAY, DATE(DATE_FORMAT(h.hired_day, '%Y-%m-01'))) bucket, 1 hires, 0 meetings, 0 created, 0 closed FROM tmp_dash_hire h
                  UNION ALL
                  SELECT IF(p_grain = 'WEEK', iv.scheduled_date - INTERVAL WEEKDAY(iv.scheduled_date) DAY, DATE(DATE_FORMAT(iv.scheduled_date, '%Y-%m-01'))), 0, 1, 0, 0
                    FROM interview iv JOIN master_item st ON st.id = iv.status_item_id AND st.code = 'FINISHED'
                    LEFT JOIN requirement r ON r.id = iv.requirement_id
                   WHERE iv.tenant_id = p_tenant_id AND iv.deleted_at IS NULL AND iv.scheduled_date BETWEEN p_from AND p_to
                     AND (v_client IS NULL OR iv.client_item_id = v_client OR r.client_item_id = v_client)
                  UNION ALL
                  SELECT IF(p_grain = 'WEEK', DATE(IFNULL(CONVERT_TZ(r.created_at, '+00:00', v_tz), r.created_at)) - INTERVAL WEEKDAY(DATE(IFNULL(CONVERT_TZ(r.created_at, '+00:00', v_tz), r.created_at))) DAY, DATE(DATE_FORMAT(DATE(IFNULL(CONVERT_TZ(r.created_at, '+00:00', v_tz), r.created_at)), '%Y-%m-01'))), 0, 0, 1, 0 FROM requirement r
                   WHERE r.tenant_id = p_tenant_id AND r.deleted_at IS NULL AND r.created_at >= v_from_utc AND r.created_at < v_to_utc AND (v_client IS NULL OR r.client_item_id = v_client)
                  UNION ALL
                  SELECT IF(p_grain = 'WEEK', DATE(IFNULL(CONVERT_TZ(r.closed_at, '+00:00', v_tz), r.closed_at)) - INTERVAL WEEKDAY(DATE(IFNULL(CONVERT_TZ(r.closed_at, '+00:00', v_tz), r.closed_at))) DAY, DATE(DATE_FORMAT(DATE(IFNULL(CONVERT_TZ(r.closed_at, '+00:00', v_tz), r.closed_at)), '%Y-%m-01'))), 0, 0, 0, 1 FROM requirement r
                   WHERE r.tenant_id = p_tenant_id AND r.deleted_at IS NULL AND r.closed_at IS NOT NULL AND r.closed_at >= v_from_utc AND r.closed_at < v_to_utc AND (v_client IS NULL OR r.client_item_id = v_client)) b
           GROUP BY b.bucket) x;

  -- embudo: postulantes agregados en el período y la etapa más avanzada a la que llegaron
  SELECT JSON_OBJECT('added', COUNT(*), 'dataComplete', IFNULL(SUM(x.reached >= 2), 0), 'interview', IFNULL(SUM(x.reached >= 3), 0),
           'confirmed', IFNULL(SUM(x.reached >= 4), 0), 'resigned', IFNULL(SUM(x.resigned), 0), 'removed', IFNULL(SUM(x.removed), 0))
    INTO v_funnel
    FROM (SELECT rc.id, rc.resigned_at IS NOT NULL AS resigned, rc.deleted_at IS NOT NULL AS removed,
                 GREATEST(cs.sort_order, IFNULL((SELECT MAX(es.sort_order) FROM requirement_candidate_event e
                                                    JOIN master_item es ON es.code = JSON_UNQUOTE(JSON_EXTRACT(e.data, '$.to')) AND es.master_table_id = cs.master_table_id
                                                   WHERE e.candidate_id = rc.id AND e.type = 'STAGE'), 0)) AS reached
            FROM requirement_candidate rc
            JOIN requirement r ON r.id = rc.requirement_id AND r.tenant_id = p_tenant_id AND r.deleted_at IS NULL
            JOIN master_item cs ON cs.id = rc.stage_item_id
           WHERE rc.created_at >= v_from_utc AND rc.created_at < v_to_utc AND (v_client IS NULL OR r.client_item_id = v_client)) x;

  -- talentos en varios procesos: de los que postularon en el período, cuántos estuvieron en 2 o más RQ y a cuántos ingresaron
  DROP TEMPORARY TABLE IF EXISTS tmp_dash_talent;
  CREATE TEMPORARY TABLE tmp_dash_talent (talent_id BIGINT UNSIGNED PRIMARY KEY, processes INT, active INT, hired TINYINT) ENGINE=MEMORY;
  INSERT INTO tmp_dash_talent
  SELECT rc.talent_id, COUNT(DISTINCT rc.requirement_id),
         COUNT(DISTINCT IF(s.str1 = 'OPEN' AND rc.resigned_at IS NULL AND cs.code <> 'CONFIRMED', rc.requirement_id, NULL)),
         MAX(EXISTS (SELECT 1 FROM requirement_candidate_event e WHERE e.candidate_id = rc.id AND e.type = 'STAGE'
                      AND JSON_UNQUOTE(JSON_EXTRACT(e.data, '$.to')) = 'CONFIRMED'))
    FROM requirement_candidate rc
    JOIN requirement r ON r.id = rc.requirement_id AND r.tenant_id = p_tenant_id AND r.deleted_at IS NULL
    JOIN master_item s ON s.id = r.status_item_id
    JOIN master_item cs ON cs.id = rc.stage_item_id
   WHERE rc.deleted_at IS NULL
     AND rc.talent_id IN (SELECT rc2.talent_id FROM requirement_candidate rc2 JOIN requirement r2 ON r2.id = rc2.requirement_id
                           WHERE r2.tenant_id = p_tenant_id AND rc2.created_at >= v_from_utc AND rc2.created_at < v_to_utc AND (v_client IS NULL OR r2.client_item_id = v_client))
   GROUP BY rc.talent_id;
  SELECT JSON_OBJECT('talents', COUNT(*), 'multi', IFNULL(SUM(processes >= 2), 0), 'multiHired', IFNULL(SUM(processes >= 2 AND hired = 1), 0),
           'single', IFNULL(SUM(processes = 1), 0), 'singleHired', IFNULL(SUM(processes = 1 AND hired = 1), 0),
           'concurrent', IFNULL(SUM(active >= 2), 0), 'avgProcesses', ROUND(AVG(processes), 2))
    INTO v_multi FROM tmp_dash_talent;
  SELECT IFNULL(JSON_ARRAYAGG(JSON_OBJECT('id', x.uuid, 'firstName', x.first_name, 'lastName', x.last_name, 'secondLastName', x.second_last_name,
           'processes', x.processes, 'active', x.active, 'hired', x.hired = 1)), JSON_ARRAY())
    INTO v_multi_top
    FROM (SELECT t.uuid, t.first_name, t.last_name, t.second_last_name, d.processes, d.active, d.hired
            FROM tmp_dash_talent d JOIN talent t ON t.id = d.talent_id
           WHERE d.processes >= 2 ORDER BY d.processes DESC, d.active DESC LIMIT 10) x;

  -- clientes: RQ registrados en el período, cierres, ingresos y tiempos
  SELECT IFNULL(JSON_ARRAYAGG(JSON_OBJECT('id', x.uuid, 'label', x.label_i18n, 'rqs', x.rqs, 'open', x.open_n, 'attended', x.attended,
           'lostOrCancelled', x.lost, 'overdue', x.overdue, 'vacancies', x.vacancies, 'covered', x.covered, 'avgDaysToClose', x.days)), JSON_ARRAY())
    INTO v_clients
    FROM (SELECT c.uuid, c.label_i18n, COUNT(*) rqs, SUM(s.str1 = 'OPEN') open_n, SUM(s.code = 'ATTENDED') attended,
                 SUM(s.code IN ('LOST', 'CANCELLED')) lost, SUM(s.str1 = 'OPEN' AND r.due_date < p_today) overdue,
                 SUM((SELECT IFNULL(SUM(v.quantity), 0) FROM requirement_vacancy v WHERE v.requirement_id = r.id AND v.deleted_at IS NULL)) vacancies,
                 SUM((SELECT COUNT(*) FROM requirement_candidate rc JOIN master_item cs ON cs.id = rc.stage_item_id
                       WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL AND rc.resigned_at IS NULL AND cs.code = 'CONFIRMED')) covered,
                 ROUND(AVG(IF(r.closed_at IS NULL, NULL, DATEDIFF(DATE(IFNULL(CONVERT_TZ(r.closed_at, '+00:00', v_tz), r.closed_at)), r.request_date))), 1) days
            FROM requirement r
            JOIN master_item s ON s.id = r.status_item_id
            JOIN master_item c ON c.id = r.client_item_id
           WHERE r.tenant_id = p_tenant_id AND r.deleted_at IS NULL AND r.created_at >= v_from_utc AND r.created_at < v_to_utc AND (v_client IS NULL OR r.client_item_id = v_client)
           GROUP BY c.id, c.uuid, c.label_i18n
           ORDER BY rqs DESC LIMIT 12) x;

  -- perfiles: ingresos del período, días hasta el ingreso y postulantes por ingreso (de las vacantes con ingresos)
  SELECT IFNULL(JSON_ARRAYAGG(JSON_OBJECT('id', x.uuid, 'label', x.label_i18n, 'hires', x.hires, 'avgDaysToHire', x.days,
           'candidatesPerHire', ROUND(x.cands / x.hires, 1))), JSON_ARRAY())
    INTO v_profiles
    FROM (SELECT p.uuid, p.label_i18n, COUNT(*) hires, ROUND(AVG(h.days_to_hire), 1) days,
                 SUM((SELECT COUNT(*) FROM requirement_candidate rc WHERE rc.vacancy_id = v.id AND rc.deleted_at IS NULL)
                     / (SELECT COUNT(*) FROM tmp_dash_hire_rq hr WHERE hr.rq_id = h.rq_id)) cands
            FROM tmp_dash_hire h
            JOIN requirement_vacancy v ON v.id = h.vacancy_id
            JOIN master_item p ON p.id = v.profile_item_id
           GROUP BY p.id, p.uuid, p.label_i18n
           ORDER BY hires DESC LIMIT 12) x;

  -- calidad: renuncias y reaperturas del período
  SELECT JSON_OBJECT(
           'resignations', (SELECT COUNT(*) FROM requirement_candidate rc JOIN requirement r ON r.id = rc.requirement_id
                             WHERE r.tenant_id = p_tenant_id AND rc.resigned_at IS NOT NULL AND rc.resigned_at >= v_from_utc AND rc.resigned_at < v_to_utc AND (v_client IS NULL OR r.client_item_id = v_client)),
           'reopens', (SELECT COUNT(*) FROM requirement_reopen ro JOIN requirement r ON r.id = ro.requirement_id
                        WHERE r.tenant_id = p_tenant_id AND ro.created_at >= v_from_utc AND ro.created_at < v_to_utc AND (v_client IS NULL OR r.client_item_id = v_client)),
           'reopensMoreVacancies', (SELECT COUNT(*) FROM requirement_reopen ro JOIN requirement r ON r.id = ro.requirement_id
                        WHERE r.tenant_id = p_tenant_id AND ro.reason = 'MORE_VACANCIES' AND ro.created_at >= v_from_utc AND ro.created_at < v_to_utc AND (v_client IS NULL OR r.client_item_id = v_client)),
           'staleNow', (SELECT COUNT(*) FROM requirement r JOIN master_item s ON s.id = r.status_item_id AND s.str1 = 'OPEN'
                         JOIN v_rq_activity act ON act.requirement_id = r.id AND act.in_process > 0
                        WHERE r.tenant_id = p_tenant_id AND r.deleted_at IS NULL AND (v_client IS NULL OR r.client_item_id = v_client)
                          AND (5 * (DATEDIFF(p_today, '2000-12-31') DIV 7) + LEAST(DATEDIFF(p_today, '2000-12-31') MOD 7, 5)) - (5 * (DATEDIFF(act.last_move_day, '2000-12-31') DIV 7) + LEAST(DATEDIFF(act.last_move_day, '2000-12-31') MOD 7, 5)) >= v_warn))
    INTO v_quality;

  SELECT JSON_OBJECT('from', p_from, 'to', p_to, 'grain', p_grain, 'staleDays', v_warn,
           'hires', v_hires, 'perHire', v_per_hire, 'overdue', v_overdue, 'closing', v_closing, 'closeReasons', v_reasons,
           'speed', v_speed, 'analysts', v_analysts, 'series', v_series, 'funnel', v_funnel, 'multiProcess', v_multi,
           'multiTop', v_multi_top, 'clients', v_clients, 'profiles', v_profiles, 'quality', v_quality) AS doc;

  DROP TEMPORARY TABLE IF EXISTS tmp_dash_hire, tmp_dash_hire_rq, tmp_dash_closed, tmp_dash_an, tmp_dash_talent;
END $$

DELIMITER ;
