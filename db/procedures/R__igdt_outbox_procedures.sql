-- =====================================================================
-- Envíos a IGDT (bandeja de selección): cola igdt_outbox y snapshots de RQ y candidatos confirmados.
-- Solo los tenants con la integración activa encolan. Los snapshots no requieren actor (los usa el proceso de envío);
-- los datos personales salen cifrados (base64) y la API los descifra solo para enviarlos a IGDT por TLS.
-- =====================================================================
DELIMITER $$

-- Encola una entidad (si ya hay un envío pendiente, lo adelanta).
DROP PROCEDURE IF EXISTS sp_igdt_outbox_add $$
CREATE PROCEDURE sp_igdt_outbox_add(IN p_tenant_id BIGINT UNSIGNED, IN p_kind VARCHAR(10), IN p_uuid CHAR(36))
BEGIN
  IF p_uuid IS NOT NULL AND EXISTS (SELECT 1 FROM tenant_integration
                                     WHERE tenant_id = p_tenant_id AND provider = 'IGDT' AND status = 'ACTIVE') THEN
    INSERT INTO igdt_outbox (tenant_id, kind, entity_uuid) VALUES (p_tenant_id, p_kind, p_uuid)
    ON DUPLICATE KEY UPDATE next_at = NOW(3), attempts = 0;
  END IF;
END $$

-- Cambió un RQ: se envía el RQ y el estado de sus candidatos confirmados (o de todos, si el RQ se eliminó).
DROP PROCEDURE IF EXISTS sp_igdt_outbox_rq $$
CREATE PROCEDURE sp_igdt_outbox_rq(IN p_tenant_id BIGINT UNSIGNED, IN p_rq_uuid CHAR(36))
BEGIN
  IF EXISTS (SELECT 1 FROM tenant_integration WHERE tenant_id = p_tenant_id AND provider = 'IGDT' AND status = 'ACTIVE') THEN
    CALL sp_igdt_outbox_add(p_tenant_id, 'RQ', p_rq_uuid);
    INSERT INTO igdt_outbox (tenant_id, kind, entity_uuid)
    SELECT p_tenant_id, 'HIRE', rc.uuid
      FROM requirement r
      JOIN requirement_candidate rc ON rc.requirement_id = r.id
      JOIN master_item s ON s.id = rc.stage_item_id
     WHERE r.tenant_id = p_tenant_id AND r.uuid = p_rq_uuid AND (s.code = 'CONFIRMED' OR r.deleted_at IS NOT NULL)
    ON DUPLICATE KEY UPDATE next_at = NOW(3), attempts = 0;
  END IF;
END $$

-- Cambió un candidato: se envía su estado (solicitud de ingreso) y el RQ (tablero de postulantes).
DROP PROCEDURE IF EXISTS sp_igdt_outbox_candidate $$
CREATE PROCEDURE sp_igdt_outbox_candidate(IN p_tenant_id BIGINT UNSIGNED, IN p_candidate_uuid CHAR(36))
BEGIN
  DECLARE v_rq CHAR(36);
  IF EXISTS (SELECT 1 FROM tenant_integration WHERE tenant_id = p_tenant_id AND provider = 'IGDT' AND status = 'ACTIVE') THEN
    SET v_rq = (SELECT r.uuid FROM requirement_candidate rc JOIN requirement r ON r.id = rc.requirement_id
                 WHERE rc.uuid = p_candidate_uuid AND r.tenant_id = p_tenant_id);
    IF v_rq IS NOT NULL THEN
      CALL sp_igdt_outbox_add(p_tenant_id, 'HIRE', p_candidate_uuid);
      CALL sp_igdt_outbox_add(p_tenant_id, 'RQ', v_rq);
    END IF;
  END IF;
END $$

-- Conciliación: encola los RQ que cambiaron (ellos o sus candidatos) desde la última revisión del tenant. En la primera
-- (o con p_full = 1) encola los RQ abiertos y los cerrados de los últimos 180 días. Devuelve cuántos RQ encoló.
DROP PROCEDURE IF EXISTS sp_igdt_outbox_reconcile $$
CREATE PROCEDURE sp_igdt_outbox_reconcile(IN p_tenant_id BIGINT UNSIGNED, IN p_full TINYINT)
BEGIN
  DECLARE v_since DATETIME(3);
  DECLARE v_now DATETIME(3) DEFAULT NOW(3);
  IF NOT EXISTS (SELECT 1 FROM tenant_integration WHERE tenant_id = p_tenant_id AND provider = 'IGDT' AND status = 'ACTIVE') THEN
    SELECT 0 AS queued;
  ELSE
    SET v_since = IF(p_full = 1, NULL, (SELECT rq_synced_at FROM tenant_integration WHERE tenant_id = p_tenant_id AND provider = 'IGDT'));
    DROP TEMPORARY TABLE IF EXISTS tmp_igdt_rq;
    CREATE TEMPORARY TABLE tmp_igdt_rq (id BIGINT UNSIGNED PRIMARY KEY, uuid CHAR(36), deleted TINYINT) ENGINE=MEMORY;
    IF v_since IS NULL THEN
      INSERT INTO tmp_igdt_rq
      SELECT r.id, r.uuid, r.deleted_at IS NOT NULL FROM requirement r JOIN master_item s ON s.id = r.status_item_id
       WHERE r.tenant_id = p_tenant_id AND r.deleted_at IS NULL
         AND (s.str1 = 'OPEN' OR IFNULL(r.closed_at, IFNULL(r.updated_at, r.created_at)) >= v_now - INTERVAL 180 DAY);
    ELSE
      INSERT INTO tmp_igdt_rq
      SELECT r.id, r.uuid, r.deleted_at IS NOT NULL FROM requirement r
       WHERE r.tenant_id = p_tenant_id
         AND (IFNULL(r.updated_at, r.created_at) >= v_since OR r.deleted_at >= v_since
              OR EXISTS (SELECT 1 FROM requirement_candidate rc WHERE rc.requirement_id = r.id
                          AND (IFNULL(rc.updated_at, rc.created_at) >= v_since OR rc.deleted_at >= v_since)));
    END IF;
    INSERT INTO igdt_outbox (tenant_id, kind, entity_uuid)
    SELECT p_tenant_id, 'RQ', t.uuid FROM tmp_igdt_rq t
    ON DUPLICATE KEY UPDATE next_at = NOW(3);
    -- candidatos: los confirmados y los que cambiaron (si dejaron de estar confirmados, IGDT cancela la solicitud)
    INSERT INTO igdt_outbox (tenant_id, kind, entity_uuid)
    SELECT p_tenant_id, 'HIRE', rc.uuid
      FROM tmp_igdt_rq t
      JOIN requirement_candidate rc ON rc.requirement_id = t.id
      JOIN master_item s ON s.id = rc.stage_item_id
     WHERE s.code = 'CONFIRMED' OR t.deleted = 1
        OR (v_since IS NOT NULL AND (IFNULL(rc.updated_at, rc.created_at) >= v_since OR rc.deleted_at >= v_since))
    ON DUPLICATE KEY UPDATE next_at = NOW(3);
    DROP TEMPORARY TABLE IF EXISTS tmp_igdt_rq;
    UPDATE tenant_integration SET rq_synced_at = v_now WHERE tenant_id = p_tenant_id AND provider = 'IGDT';
    SELECT (SELECT COUNT(*) FROM igdt_outbox WHERE tenant_id = p_tenant_id AND pending_key = 1) AS queued;
  END IF;
END $$

-- Próximos envíos listos de un tenant (los RQ antes que los candidatos: IGDT muestra la solicitud con su RQ).
DROP PROCEDURE IF EXISTS sp_igdt_outbox_next $$
CREATE PROCEDURE sp_igdt_outbox_next(IN p_tenant_id BIGINT UNSIGNED, IN p_limit INT)
BEGIN
  SELECT o.id, o.kind, o.entity_uuid
    FROM igdt_outbox o
   WHERE o.tenant_id = p_tenant_id AND o.pending_key = 1 AND o.next_at <= NOW(3)
   ORDER BY o.kind = 'RQ' DESC, o.id
   LIMIT p_limit;
END $$

DROP PROCEDURE IF EXISTS sp_igdt_outbox_result $$

-- Resultados de los envíos (lote). Si falló, reintenta con espera creciente (1, 2, 4… hasta 60 minutos):
-- [{"id":n,"ok":true|false,"error":"..."}]
DROP PROCEDURE IF EXISTS sp_igdt_outbox_results $$
CREATE PROCEDURE sp_igdt_outbox_results(IN p_results JSON)
BEGIN
  UPDATE igdt_outbox o
    JOIN JSON_TABLE(p_results, '$[*]' COLUMNS (id BIGINT UNSIGNED PATH '$.id', ok TINYINT PATH '$.ok',
                                              err VARCHAR(200) PATH '$.error')) j ON j.id = o.id
     SET o.sent_at = IF(j.ok = 1, NOW(3), NULL), o.last_error = IF(j.ok = 1, NULL, LEFT(j.err, 200)),
         o.next_at = IF(j.ok = 1, o.next_at, NOW(3) + INTERVAL LEAST(POW(2, LEAST(o.attempts, 6)), 60) MINUTE),
         o.attempts = IF(j.ok = 1, o.attempts, o.attempts + 1)
   WHERE o.sent_at IS NULL;
END $$

-- Depura los envíos ya hechos hace más de 30 días.
DROP PROCEDURE IF EXISTS sp_igdt_outbox_purge $$
CREATE PROCEDURE sp_igdt_outbox_purge()
BEGIN
  DELETE FROM igdt_outbox WHERE sent_at IS NOT NULL AND sent_at < NOW(3) - INTERVAL 30 DAY LIMIT 5000;
END $$

-- Estado de la cola del tenant (pantalla de integración).
DROP PROCEDURE IF EXISTS sp_igdt_outbox_status $$
CREATE PROCEDURE sp_igdt_outbox_status(IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  SELECT JSON_OBJECT('pending', (SELECT COUNT(*) FROM igdt_outbox WHERE tenant_id = p_tenant_id AND pending_key = 1),
           'failing', (SELECT COUNT(*) FROM igdt_outbox WHERE tenant_id = p_tenant_id AND pending_key = 1 AND attempts > 0),
           'lastError', (SELECT last_error FROM igdt_outbox WHERE tenant_id = p_tenant_id AND pending_key = 1 AND attempts > 0
                          ORDER BY id DESC LIMIT 1),
           'lastSentAt', (SELECT MAX(sent_at) FROM igdt_outbox WHERE tenant_id = p_tenant_id)) AS doc;
END $$

-- ---------------- Snapshots ----------------

-- RQ (lote: arreglo JSON de uuid) con vacantes y postulantes. Clientes y perfiles con el id de IGDT cuando están
-- mapeados. Los uuid que no existan no devuelven fila.
DROP PROCEDURE IF EXISTS sp_igdt_rq_snapshot $$
DROP PROCEDURE IF EXISTS sp_igdt_rq_snapshots $$
CREATE PROCEDURE sp_igdt_rq_snapshots(IN p_tenant_id BIGINT UNSIGNED, IN p_uuids JSON)
BEGIN
    DECLARE v_warn, v_crit INT;
    DECLARE v_notify, v_stale_on TINYINT(1);
    -- alertas de RQ sin movimiento del tenant: IGDT aplica las mismas vistas y etiquetas que el tablero de BDT
    CALL sp_rq_alert_days(p_tenant_id, v_warn, v_crit, v_notify);
    SET v_stale_on = EXISTS (SELECT 1 FROM feature f LEFT JOIN tenant_feature tf ON tf.feature_id = f.id AND tf.tenant_id = p_tenant_id
                              WHERE f.code = 'rq_stale_alerts' AND f.status = 'ACTIVE' AND IFNULL(tf.enabled, f.default_enabled) = 1);
    SELECT JSON_OBJECT('id', r.uuid, 'code', r.code, 'title', r.title, 'description', r.description,
             'createdAt', DATE_FORMAT(r.created_at, '%Y-%m-%dT%H:%i:%s.%fZ'),
             'alerts', JSON_OBJECT('enabled', v_stale_on = 1, 'warnDays', v_warn, 'criticalDays', v_crit),
             'status', JSON_OBJECT('code', s.code, 'label', JSON_UNQUOTE(JSON_EXTRACT(s.label_i18n, '$.es')), 'group', IFNULL(s.str1, 'OPEN')),
             'client', JSON_OBJECT('id', COALESCE((SELECT m.remote_uuid FROM igdt_map m WHERE m.tenant_id = p_tenant_id
                                                     AND m.entity = 'MASTER_ITEM' AND m.local_id = c.id), c.uuid),
                                   'label', JSON_UNQUOTE(JSON_EXTRACT(c.label_i18n, '$.es'))),
             'owners', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('nameEnc', TO_BASE64(u.full_name_enc)))
                                 FROM requirement_owner o JOIN app_user u ON u.id = o.user_id
                                WHERE o.requirement_id = r.id AND o.unassigned_at IS NULL), JSON_ARRAY()),
             'requestDate', r.request_date, 'dueDate', r.due_date,
             'closedAt', IF(r.closed_at IS NULL, NULL, DATE_FORMAT(r.closed_at, '%Y-%m-%dT%H:%i:%s.%fZ')),
             'conditions', JSON_OBJECT(
                 'workModality', (SELECT JSON_UNQUOTE(JSON_EXTRACT(label_i18n, '$.es')) FROM master_item WHERE id = r.work_modality_item_id),
                 'duration', IF(r.duration_value IS NULL, NULL, CONCAT(TRIM(TRAILING '.' FROM TRIM(TRAILING '0' FROM r.duration_value)), ' ',
                     (SELECT LOWER(JSON_UNQUOTE(JSON_EXTRACT(label_i18n, '$.es'))) FROM master_item WHERE id = r.duration_unit_item_id))),
                 'contract', IF(r.contract_value IS NULL, NULL, CONCAT(TRIM(TRAILING '.' FROM TRIM(TRAILING '0' FROM r.contract_value)), ' ',
                     (SELECT LOWER(JSON_UNQUOTE(JSON_EXTRACT(label_i18n, '$.es'))) FROM master_item WHERE id = r.contract_unit_item_id))),
                 'emo', r.needs_emo = 1, 'sctr', r.needs_sctr = 1, 'laptop', r.needs_laptop = 1),
             'stages', (SELECT JSON_ARRAYAGG(JSON_OBJECT('code', x.code, 'label', x.label, 'order', x.sort_order))
                          FROM (SELECT i.code, JSON_UNQUOTE(JSON_EXTRACT(i.label_i18n, '$.es')) label, i.sort_order
                                  FROM master_item i JOIN master_table t ON t.id = i.master_table_id AND t.code = 'CANDIDATE_STAGE'
                                 WHERE i.tenant_id IS NULL AND i.deleted_at IS NULL AND i.is_active = 1 ORDER BY i.sort_order) x),
             'vacancies', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', v.uuid,
                             'profile', JSON_OBJECT('id', COALESCE((SELECT m.remote_uuid FROM igdt_map m WHERE m.tenant_id = p_tenant_id
                                                                     AND m.entity = 'MASTER_ITEM' AND m.local_id = pi.id), pi.uuid),
                                                    'label', JSON_UNQUOTE(JSON_EXTRACT(pi.label_i18n, '$.es'))),
                             'quantity', v.quantity, 'rateInitial', v.rate_initial, 'rateFinal', v.rate_final))
                            FROM requirement_vacancy v JOIN master_item pi ON pi.id = v.profile_item_id
                           WHERE v.requirement_id = r.id AND v.deleted_at IS NULL), JSON_ARRAY()),
             'candidates', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', rc.uuid, 'vacancyId', v.uuid, 'talentId', tl.uuid,
                             'name', CONCAT_WS(' ', tl.first_name, tl.last_name, tl.second_last_name), 'headline', tl.headline,
                             'stage', JSON_OBJECT('code', cs.code, 'label', JSON_UNQUOTE(JSON_EXTRACT(cs.label_i18n, '$.es')), 'order', cs.sort_order),
                             'resigned', rc.resigned_at IS NOT NULL, 'hired', rc.hired_at IS NOT NULL, 'startDate', rc.hire_start_date,
                             'stageAt', DATE_FORMAT(IFNULL(rc.stage_at, rc.created_at), '%Y-%m-%dT%H:%i:%s.%fZ'),
                             'addedAt', DATE_FORMAT(rc.created_at, '%Y-%m-%dT%H:%i:%s.%fZ')))
                             FROM requirement_candidate rc
                             JOIN talent tl ON tl.id = rc.talent_id
                             JOIN requirement_vacancy v ON v.id = rc.vacancy_id
                             JOIN master_item cs ON cs.id = rc.stage_item_id
                            WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL), JSON_ARRAY()),
             'updatedAt', DATE_FORMAT(GREATEST(IFNULL(r.updated_at, r.created_at),
                             IFNULL((SELECT MAX(GREATEST(IFNULL(rc.updated_at, rc.created_at), IFNULL(rc.deleted_at, rc.created_at)))
                                       FROM requirement_candidate rc WHERE rc.requirement_id = r.id), r.created_at)), '%Y-%m-%dT%H:%i:%s.%fZ'),
             'deleted', r.deleted_at IS NOT NULL) AS doc
      FROM JSON_TABLE(p_uuids, '$[*]' COLUMNS (u CHAR(36) PATH '$')) j
      JOIN requirement r ON r.tenant_id = p_tenant_id AND r.uuid = j.u
      JOIN master_item s ON s.id = r.status_item_id
      JOIN master_item c ON c.id = r.client_item_id;
END $$

-- Candidatos (lote: arreglo JSON de uuid): active = confirmado, vigente y sin renuncia en un RQ no eliminado. Datos personales cifrados (base64).
DROP PROCEDURE IF EXISTS sp_igdt_hire_snapshot $$
DROP PROCEDURE IF EXISTS sp_igdt_hire_snapshots $$
CREATE PROCEDURE sp_igdt_hire_snapshots(IN p_tenant_id BIGINT UNSIGNED, IN p_uuids JSON)
BEGIN
  SELECT JSON_OBJECT('id', rc.uuid,
           'active', cs.code = 'CONFIRMED' AND rc.deleted_at IS NULL AND rc.resigned_at IS NULL AND r.deleted_at IS NULL
                     AND IFNULL(rc.stage_at, rc.created_at) >= IFNULL(ti.hires_from, '2000-01-01'),
           'requirement', JSON_OBJECT('id', r.uuid, 'code', r.code, 'title', r.title),
           'client', JSON_OBJECT('id', COALESCE((SELECT m.remote_uuid FROM igdt_map m WHERE m.tenant_id = p_tenant_id
                                                   AND m.entity = 'MASTER_ITEM' AND m.local_id = c.id), c.uuid),
                                 'label', JSON_UNQUOTE(JSON_EXTRACT(c.label_i18n, '$.es'))),
           'vacancy', JSON_OBJECT('id', v.uuid,
                        'profile', JSON_OBJECT('id', COALESCE((SELECT m.remote_uuid FROM igdt_map m WHERE m.tenant_id = p_tenant_id
                                                                AND m.entity = 'MASTER_ITEM' AND m.local_id = pi.id), pi.uuid),
                                               'label', JSON_UNQUOTE(JSON_EXTRACT(pi.label_i18n, '$.es'))),
                        'rateInitial', v.rate_initial, 'rateFinal', v.rate_final),
           'talent', JSON_OBJECT('id', tl.uuid, 'firstName', tl.first_name, 'lastName', tl.last_name,
                        'secondLastName', IFNULL(tl.second_last_name, ''), 'headline', IFNULL(tl.headline, ''),
                        'docType', JSON_OBJECT('code', IFNULL(dt.code, ''), 'label', IFNULL(JSON_UNQUOTE(JSON_EXTRACT(dt.label_i18n, '$.es')), '')),
                        'docNumberEnc', TO_BASE64(tl.doc_number_enc), 'emailEnc', TO_BASE64(tl.email_enc), 'phoneEnc', TO_BASE64(tl.phone_enc)),
           'conditions', JSON_OBJECT(
               'workModality', (SELECT JSON_UNQUOTE(JSON_EXTRACT(label_i18n, '$.es')) FROM master_item WHERE id = r.work_modality_item_id),
               'duration', IF(r.duration_value IS NULL, NULL, CONCAT(TRIM(TRAILING '.' FROM TRIM(TRAILING '0' FROM r.duration_value)), ' ',
                   (SELECT LOWER(JSON_UNQUOTE(JSON_EXTRACT(label_i18n, '$.es'))) FROM master_item WHERE id = r.duration_unit_item_id))),
               'contract', IF(r.contract_value IS NULL, NULL, CONCAT(TRIM(TRAILING '.' FROM TRIM(TRAILING '0' FROM r.contract_value)), ' ',
                   (SELECT LOWER(JSON_UNQUOTE(JSON_EXTRACT(label_i18n, '$.es'))) FROM master_item WHERE id = r.contract_unit_item_id))),
               'emo', r.needs_emo = 1, 'sctr', r.needs_sctr = 1, 'laptop', r.needs_laptop = 1),
           'confirmedByNameEnc', (SELECT TO_BASE64(u.full_name_enc) FROM app_user u WHERE u.id = IFNULL(rc.updated_by, rc.created_by)),
           'confirmedAt', DATE_FORMAT(IFNULL(rc.stage_at, IFNULL(rc.updated_at, rc.created_at)), '%Y-%m-%dT%H:%i:%s.%fZ')) AS doc
    FROM JSON_TABLE(p_uuids, '$[*]' COLUMNS (u CHAR(36) PATH '$')) j
    JOIN requirement_candidate rc ON rc.uuid = j.u
    JOIN requirement r ON r.id = rc.requirement_id AND r.tenant_id = p_tenant_id
    JOIN master_item cs ON cs.id = rc.stage_item_id
    JOIN master_item c ON c.id = r.client_item_id
    JOIN requirement_vacancy v ON v.id = rc.vacancy_id
    JOIN master_item pi ON pi.id = v.profile_item_id
    JOIN talent tl ON tl.id = rc.talent_id
    LEFT JOIN master_item dt ON dt.id = tl.doc_type_item_id
    LEFT JOIN tenant_integration ti ON ti.tenant_id = p_tenant_id AND ti.provider = 'IGDT';
END $$

DELIMITER ;

DELIMITER $$

-- Eventos de ingreso registrados en IGDT (desde el cursor), en orden:
--   HIRED: marca al candidato como ingresado (hired_at, hire_start_date).
--   START_CHANGED: al candidato ya ingresado le cambia la fecha de inicio (evento HIRE_START_CHANGED).
--   REVOKED: el cliente revocó el ingreso antes de empezar: el candidato deja de estar ingresado (evento HIRE_REVOKED). El RQ
--     cerrado NO se reabre solo (eso lo decide selección); la API avisa a los responsables y a quienes pueden reabrir.
-- Cierra como Atendido el RQ abierto que ya tiene tantos ingresos (vigentes) como vacantes. Los RQ afectados vuelven a
-- publicarse en IGDT. Devuelve: (1) los RQ cerrados; (2) los ingresos revocados (para avisar).
DROP PROCEDURE IF EXISTS sp_igdt_apply_hires $$
CREATE PROCEDURE sp_igdt_apply_hires(IN p_tenant_id BIGINT UNSIGNED, IN p_events JSON, IN p_cursor BIGINT UNSIGNED)
BEGIN
  DECLARE v_attended BIGINT UNSIGNED;
  DECLARE v_done TINYINT DEFAULT 0;
  DECLARE v_cand, v_rq BIGINT UNSIGNED;
  DECLARE v_hired_at DATETIME(3);
  DECLARE v_old_start DATE;
  DECLARE e_type VARCHAR(20);
  DECLARE e_cid CHAR(36);
  DECLARE e_start, e_prev DATE;
  DECLARE e_position VARCHAR(150);
  DECLARE e_reason VARCHAR(500);
  DECLARE cur_ev CURSOR FOR
    SELECT j.type, j.cid, j.start_date, j.prev_start, j.position, j.reason
      FROM JSON_TABLE(IFNULL(p_events, JSON_ARRAY()), '$[*]' COLUMNS (
             ord FOR ORDINALITY,
             type VARCHAR(20) PATH '$.type', cid CHAR(36) PATH '$.candidateId',
             start_date DATE PATH '$.startDate' NULL ON EMPTY NULL ON ERROR,
             prev_start DATE PATH '$.previousStartDate' NULL ON EMPTY NULL ON ERROR,
             position VARCHAR(150) PATH '$.position' NULL ON EMPTY NULL ON ERROR,
             reason VARCHAR(500) PATH '$.reason' NULL ON EMPTY NULL ON ERROR)) j
     WHERE j.type IN ('HIRED', 'START_CHANGED', 'REVOKED')
     ORDER BY j.ord;
  DECLARE CONTINUE HANDLER FOR NOT FOUND SET v_done = 1;
  DECLARE EXIT HANDLER FOR SQLEXCEPTION BEGIN ROLLBACK; RESIGNAL; END;
  SET v_attended = (SELECT i.id FROM master_item i JOIN master_table t ON t.id = i.master_table_id AND t.code = 'RQ_STATUS'
                     WHERE i.code = 'ATTENDED' AND i.tenant_id IS NULL AND i.deleted_at IS NULL);
  DROP TEMPORARY TABLE IF EXISTS tmp_igdt_rqs;
  CREATE TEMPORARY TABLE tmp_igdt_rqs (id BIGINT UNSIGNED PRIMARY KEY) ENGINE=MEMORY;
  DROP TEMPORARY TABLE IF EXISTS tmp_igdt_revoked;
  CREATE TEMPORARY TABLE tmp_igdt_revoked (candidate_id BIGINT UNSIGNED PRIMARY KEY, reason VARCHAR(500) NULL) ENGINE=MEMORY;
  START TRANSACTION;
  OPEN cur_ev;
  ev: LOOP
    FETCH cur_ev INTO e_type, e_cid, e_start, e_prev, e_position, e_reason;
    IF v_done = 1 THEN LEAVE ev; END IF;
    -- SET (SELECT ...) no dispara NOT FOUND: un candidato inexistente no corta el recorrido
    SET v_cand = (SELECT rc.id FROM requirement_candidate rc JOIN requirement r ON r.id = rc.requirement_id AND r.tenant_id = p_tenant_id
                   WHERE rc.uuid = e_cid LIMIT 1);
    IF v_cand IS NOT NULL THEN
      SET v_rq = (SELECT requirement_id FROM requirement_candidate WHERE id = v_cand);
      SET v_hired_at = (SELECT hired_at FROM requirement_candidate WHERE id = v_cand);
      SET v_old_start = (SELECT hire_start_date FROM requirement_candidate WHERE id = v_cand);
      IF e_type = 'HIRED' AND v_hired_at IS NULL THEN
        INSERT INTO requirement_candidate_event (candidate_id, type, data)
        VALUES (v_cand, 'HIRED', JSON_OBJECT('startDate', e_start, 'position', e_position, 'source', 'IGDT'));
        UPDATE requirement_candidate SET hired_at = NOW(3), hire_start_date = e_start WHERE id = v_cand;
        DELETE FROM tmp_igdt_revoked WHERE candidate_id = v_cand;
        INSERT IGNORE INTO tmp_igdt_rqs VALUES (v_rq);
      ELSEIF e_type = 'START_CHANGED' AND v_hired_at IS NOT NULL AND e_start IS NOT NULL AND NOT (v_old_start <=> e_start) THEN
        INSERT INTO requirement_candidate_event (candidate_id, type, data)
        VALUES (v_cand, 'HIRE_START_CHANGED', JSON_OBJECT('from', IFNULL(v_old_start, e_prev), 'to', e_start, 'reason', e_reason, 'source', 'IGDT'));
        UPDATE requirement_candidate SET hire_start_date = e_start WHERE id = v_cand;
        INSERT IGNORE INTO tmp_igdt_rqs VALUES (v_rq);
      ELSEIF e_type = 'REVOKED' AND v_hired_at IS NOT NULL THEN
        INSERT INTO requirement_candidate_event (candidate_id, type, data)
        VALUES (v_cand, 'HIRE_REVOKED', JSON_OBJECT('startDate', v_old_start, 'reason', e_reason, 'source', 'IGDT'));
        UPDATE requirement_candidate SET hired_at = NULL, hire_start_date = NULL WHERE id = v_cand;
        INSERT INTO tmp_igdt_revoked VALUES (v_cand, e_reason) ON DUPLICATE KEY UPDATE reason = VALUES(reason);
        INSERT IGNORE INTO tmp_igdt_rqs VALUES (v_rq);
      END IF;
    END IF;
  END LOOP;
  CLOSE cur_ev;
  -- RQ abiertos con todas sus vacantes cubiertas por ingresos vigentes: se cierran como Atendido
  DROP TEMPORARY TABLE IF EXISTS tmp_igdt_closed;
  CREATE TEMPORARY TABLE tmp_igdt_closed (id BIGINT UNSIGNED PRIMARY KEY) ENGINE=MEMORY;
  INSERT INTO tmp_igdt_closed
  SELECT r.id FROM tmp_igdt_rqs x
    JOIN requirement r ON r.id = x.id AND r.deleted_at IS NULL
    JOIN master_item s ON s.id = r.status_item_id AND s.str1 = 'OPEN'
   WHERE v_attended IS NOT NULL
     AND (SELECT IFNULL(SUM(v.quantity), 0) FROM requirement_vacancy v WHERE v.requirement_id = r.id AND v.deleted_at IS NULL) > 0
     AND (SELECT COUNT(*) FROM requirement_candidate rc WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL
           AND rc.resigned_at IS NULL AND rc.hired_at IS NOT NULL)
         >= (SELECT SUM(v.quantity) FROM requirement_vacancy v WHERE v.requirement_id = r.id AND v.deleted_at IS NULL);
  UPDATE requirement r JOIN tmp_igdt_closed c ON c.id = r.id
     SET r.status_item_id = v_attended, r.closed_at = IFNULL(r.closed_at, NOW(3)), r.version = r.version + 1;
  UPDATE tenant_integration SET hires_cursor = GREATEST(hires_cursor, p_cursor), last_error = NULL
   WHERE tenant_id = p_tenant_id AND provider = 'IGDT';
  COMMIT;
  INSERT INTO audit_log (tenant_id, user_id, api_key_id, action, entity, entity_id, detail, ip)
  SELECT p_tenant_id, NULL, NULL, 'AUTO_CLOSE', 'requirement', r.uuid, JSON_OBJECT('reason', 'IGDT_HIRES', 'to', 'ATTENDED'), NULL
    FROM tmp_igdt_closed c JOIN requirement r ON r.id = c.id;
  INSERT INTO audit_log (tenant_id, user_id, api_key_id, action, entity, entity_id, detail, ip)
  SELECT p_tenant_id, NULL, NULL, 'HIRE_REVOKED', 'requirement', r.uuid, JSON_OBJECT('candidate', rc.uuid, 'reason', x.reason, 'source', 'IGDT'), NULL
    FROM tmp_igdt_revoked x JOIN requirement_candidate rc ON rc.id = x.candidate_id JOIN requirement r ON r.id = rc.requirement_id;
  -- el RQ (con sus postulantes ingresados) vuelve a publicarse en IGDT
  INSERT INTO igdt_outbox (tenant_id, kind, entity_uuid)
  SELECT p_tenant_id, 'RQ', r.uuid FROM tmp_igdt_rqs x JOIN requirement r ON r.id = x.id
  ON DUPLICATE KEY UPDATE next_at = NOW(3), attempts = 0;
  SELECT r.uuid, r.code FROM tmp_igdt_closed c JOIN requirement r ON r.id = c.id;
  SELECT r.uuid AS rq_uuid, r.code, r.title, rc.uuid AS candidate_uuid,
         CONCAT_WS(' ', tl.first_name, tl.last_name, tl.second_last_name) AS talent_name, IFNULL(x.reason, '') AS reason
    FROM tmp_igdt_revoked x
    JOIN requirement_candidate rc ON rc.id = x.candidate_id
    JOIN requirement r ON r.id = rc.requirement_id
    JOIN talent tl ON tl.id = rc.talent_id;
  DROP TEMPORARY TABLE IF EXISTS tmp_igdt_rqs;
  DROP TEMPORARY TABLE IF EXISTS tmp_igdt_revoked;
  DROP TEMPORARY TABLE IF EXISTS tmp_igdt_closed;
END $$

DELIMITER ;
