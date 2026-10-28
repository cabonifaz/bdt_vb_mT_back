-- =====================================================================
-- Requerimientos (RQ): tablero, ficha, alta/edición con vacantes, estados, candidatos (pipeline) y contactos.
-- El tablero y la ficha se devuelven como documentos JSON; las tarifas y bandas salariales solo se incluyen
-- (y solo se guardan) si el actor tiene rq.rates.read. El nombre del responsable va cifrado aparte (lo descifra la app).
-- =====================================================================
DELIMITER $$


DROP PROCEDURE IF EXISTS sp_rq_resolve $$
CREATE PROCEDURE sp_rq_resolve(IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), OUT o_id BIGINT UNSIGNED, OUT o_closed TINYINT(1))
BEGIN
  SET o_id = NULL, o_closed = 0;
  SELECT r.id, (s.str1 = 'CLOSED') INTO o_id, o_closed
    FROM requirement r JOIN master_item s ON s.id = r.status_item_id
   WHERE r.uuid = p_uuid AND r.tenant_id = p_tenant_id AND r.deleted_at IS NULL;
  IF o_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:RQ_NOT_FOUND'; END IF;
END $$

DROP PROCEDURE IF EXISTS sp_rq_can_rates $$
CREATE PROCEDURE sp_rq_can_rates(IN p_actor_id BIGINT UNSIGNED, OUT o_ok TINYINT(1))
BEGIN
  SET o_ok = EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'rq.rates.read');
END $$

-- Tablero: RQ abiertos + cerrados actualizados en los últimos p_closed_days días.
-- Filtros opcionales: texto (código, título, cliente), cliente, responsable, solo los míos.
DROP PROCEDURE IF EXISTS sp_rq_board $$
CREATE PROCEDURE sp_rq_board(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_search VARCHAR(100),
  IN p_client_uuid CHAR(36), IN p_mine TINYINT(1), IN p_closed_days INT)
BEGIN
  DECLARE v_search VARCHAR(110);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.read');
  SET v_search = IF(p_search IS NULL OR TRIM(p_search) = '', NULL, CONCAT('%', LOWER(TRIM(p_search)), '%'));
  SELECT JSON_OBJECT(
           'id', r.uuid, 'code', r.code, 'title', r.title, 'version', r.version, 'boardOrder', r.board_order,
           'requestDate', r.request_date, 'dueDate', r.due_date, 'updatedAt', IFNULL(r.updated_at, r.created_at),
           'client', JSON_OBJECT('id', c.uuid, 'label', c.label_i18n),
           'status', JSON_OBJECT('id', s.uuid, 'code', s.code, 'label', s.label_i18n, 'group', s.str1),
           'modality', IF(wm.id IS NULL, NULL, JSON_OBJECT('code', wm.code, 'label', wm.label_i18n)),
           'ownerId', o.uuid,
           'vacancies', (SELECT IFNULL(SUM(v.quantity), 0) FROM requirement_vacancy v WHERE v.requirement_id = r.id AND v.deleted_at IS NULL),
           'covered', (SELECT COUNT(*) FROM requirement_candidate rc JOIN master_item cs ON cs.id = rc.stage_item_id
                        WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL AND cs.code = 'CONFIRMED'),
           'candidates', (SELECT COUNT(*) FROM requirement_candidate rc WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL),
           'profiles', (SELECT JSON_ARRAYAGG(JSON_OBJECT('label', p.label_i18n, 'total', v.quantity,
                          'covered', (SELECT COUNT(*) FROM requirement_candidate rc JOIN master_item cs ON cs.id = rc.stage_item_id
                                       WHERE rc.vacancy_id = v.id AND rc.deleted_at IS NULL AND cs.code = 'CONFIRMED')))
                          FROM requirement_vacancy v JOIN master_item p ON p.id = v.profile_item_id
                         WHERE v.requirement_id = r.id AND v.deleted_at IS NULL)
         ) AS doc,
         o.full_name_enc AS owner_name_enc
    FROM requirement r
    JOIN master_item s ON s.id = r.status_item_id
    JOIN master_item c ON c.id = r.client_item_id
    LEFT JOIN master_item wm ON wm.id = r.work_modality_item_id
    LEFT JOIN app_user o ON o.id = r.owner_user_id
   WHERE r.tenant_id = p_tenant_id AND r.deleted_at IS NULL
     AND (s.str1 = 'OPEN' OR IFNULL(r.updated_at, r.created_at) >= NOW(3) - INTERVAL IFNULL(p_closed_days, 60) DAY)
     AND (v_search IS NULL OR LOWER(r.code) LIKE v_search OR LOWER(r.title) LIKE v_search
          OR LOWER(JSON_UNQUOTE(JSON_EXTRACT(c.label_i18n, '$.es'))) LIKE v_search)
     AND (p_client_uuid IS NULL OR c.uuid = p_client_uuid)
     AND (IFNULL(p_mine, 0) = 0 OR r.owner_user_id = p_actor_id OR r.created_by = p_actor_id)
   ORDER BY s.sort_order, r.board_order, r.due_date, r.id
   LIMIT 1000;
END $$

-- Ficha completa del RQ (con vacantes, contactos, bandas y candidatos)
DROP PROCEDURE IF EXISTS sp_rq_get $$
CREATE PROCEDURE sp_rq_get(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_closed TINYINT(1);
  DECLARE v_rates TINYINT(1);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.read');
  CALL sp_rq_resolve(p_tenant_id, p_uuid, v_id, v_closed);
  CALL sp_rq_can_rates(p_actor_id, v_rates);
  SELECT JSON_OBJECT(
           'id', r.uuid, 'code', r.code, 'title', r.title, 'description', r.description, 'version', r.version,
           'requestDate', r.request_date, 'dueDate', r.due_date, 'closed', v_closed = 1, 'canSeeRates', v_rates = 1,
           'createdAt', r.created_at, 'updatedAt', r.updated_at,
           'client', JSON_OBJECT('id', c.uuid, 'label', c.label_i18n, 'ruc', c.str1, 'address', c.str2),
           'status', JSON_OBJECT('id', s.uuid, 'code', s.code, 'label', s.label_i18n, 'group', s.str1),
           'durationValue', r.duration_value, 'durationUnitId', du.uuid,
           'contractValue', r.contract_value, 'contractUnitId', cu.uuid,
           'workModalityId', wm.uuid, 'needsEmo', r.needs_emo = 1, 'needsSctr', r.needs_sctr = 1, 'needsLaptop', r.needs_laptop = 1,
           'ownerId', o.uuid,
           'contacts', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', cc.uuid, 'firstName', cc.first_name, 'lastName', cc.last_name,
                          'secondLastName', cc.second_last_name, 'position', cc.position, 'email', cc.email, 'phone', cc.phone))
                        FROM requirement_contact x JOIN client_contact cc ON cc.id = x.contact_id
                       WHERE x.requirement_id = r.id AND cc.deleted_at IS NULL), JSON_ARRAY()),
           'billing', IF(v_rates = 1, IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('contractId', ct.uuid, 'contractLabel', ct.label_i18n,
                          'group', ct.str1, 'currencyId', cur.uuid, 'currencyCode', cur.code,
                          'baseMin', b.base_min, 'baseMax', b.base_max, 'mobilityMin', b.mobility_min, 'mobilityMax', b.mobility_max,
                          'monthlyMin', b.monthly_min, 'monthlyMax', b.monthly_max, 'quarterlyMin', b.quarterly_min, 'quarterlyMax', b.quarterly_max,
                          'semiannualMin', b.semiannual_min, 'semiannualMax', b.semiannual_max))
                        FROM requirement_billing b JOIN master_item ct ON ct.id = b.contract_item_id JOIN master_item cur ON cur.id = b.currency_item_id
                       WHERE b.requirement_id = r.id), JSON_ARRAY()), NULL),
           'vacancies', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', v.uuid, 'profileId', p.uuid, 'profile', p.label_i18n, 'quantity', v.quantity,
                          'rateInitial', IF(v_rates = 1, v.rate_initial, NULL), 'rateFinal', IF(v_rates = 1, v.rate_final, NULL),
                          'covered', (SELECT COUNT(*) FROM requirement_candidate rc JOIN master_item cs ON cs.id = rc.stage_item_id
                                       WHERE rc.vacancy_id = v.id AND rc.deleted_at IS NULL AND cs.code = 'CONFIRMED'),
                          'skills', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('skillId', sk.uuid, 'label', sk.label_i18n, 'years', vs.years, 'optional', vs.optional = 1))
                                      FROM vacancy_skill vs JOIN master_item sk ON sk.id = vs.skill_item_id WHERE vs.vacancy_id = v.id), JSON_ARRAY()),
                          'careers', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('career', vc.career, 'gradeId', g.uuid, 'grade', g.label_i18n, 'optional', vc.optional = 1))
                                      FROM vacancy_career vc LEFT JOIN master_item g ON g.id = vc.grade_item_id WHERE vc.vacancy_id = v.id), JSON_ARRAY())))
                        FROM requirement_vacancy v JOIN master_item p ON p.id = v.profile_item_id
                       WHERE v.requirement_id = r.id AND v.deleted_at IS NULL), JSON_ARRAY()),
           'candidates', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', rc.uuid, 'vacancyId', v.uuid, 'boardOrder', rc.board_order, 'notes', rc.notes,
                          'stage', JSON_OBJECT('id', cs.uuid, 'code', cs.code, 'label', cs.label_i18n), 'updatedAt', IFNULL(rc.updated_at, rc.created_at),
                          'talent', JSON_OBJECT('id', t.uuid, 'firstName', t.first_name, 'lastName', t.last_name, 'secondLastName', t.second_last_name,
                                     'headline', t.headline, 'photoId', ph.uuid, 'status', t.status,
                                     'blacklisted', EXISTS (SELECT 1 FROM talent_blacklist b WHERE b.talent_id = t.id AND b.deleted_at IS NULL
                                                            AND (b.client_item_id IS NULL OR b.client_item_id = r.client_item_id)))))
                        FROM requirement_candidate rc
                        JOIN requirement_vacancy v ON v.id = rc.vacancy_id
                        JOIN master_item cs ON cs.id = rc.stage_item_id
                        JOIN talent t ON t.id = rc.talent_id
                        LEFT JOIN file_object ph ON ph.id = t.photo_file_id AND ph.deleted_at IS NULL
                       WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL), JSON_ARRAY())
         ) AS doc,
         o.full_name_enc AS owner_name_enc
    FROM requirement r
    JOIN master_item s ON s.id = r.status_item_id
    JOIN master_item c ON c.id = r.client_item_id
    LEFT JOIN master_item du ON du.id = r.duration_unit_item_id
    LEFT JOIN master_item cu ON cu.id = r.contract_unit_item_id
    LEFT JOIN master_item wm ON wm.id = r.work_modality_item_id
    LEFT JOIN app_user o ON o.id = r.owner_user_id
   WHERE r.id = v_id;
END $$

-- Alta o edición del RQ con sus vacantes (skills y carreras), contactos y bandas salariales.
-- p_code NULL o vacío = autogenerado (RQ-AAAA-0001). Vacantes que no vengan en p_vacancies se dan de baja
-- (salvo que tengan candidatos). Sin rq.rates.read se conservan las tarifas y bandas existentes.
DROP PROCEDURE IF EXISTS sp_rq_save $$
CREATE PROCEDURE sp_rq_save(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_code VARCHAR(40),
  IN p_client_uuid CHAR(36), IN p_title VARCHAR(200), IN p_description VARCHAR(2000), IN p_status_uuid CHAR(36),
  IN p_request_date DATE, IN p_due_date DATE, IN p_duration_value DECIMAL(8,2), IN p_duration_unit_uuid CHAR(36),
  IN p_contract_value DECIMAL(8,2), IN p_contract_unit_uuid CHAR(36), IN p_work_modality_uuid CHAR(36),
  IN p_emo TINYINT(1), IN p_sctr TINYINT(1), IN p_laptop TINYINT(1), IN p_owner_uuid CHAR(36),
  IN p_contacts JSON, IN p_billing JSON, IN p_vacancies JSON, IN p_expected_version INT, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id, v_client, v_status, v_du, v_cu, v_wm, v_owner BIGINT UNSIGNED;
  DECLARE v_version INT;
  DECLARE v_created TINYINT(1) DEFAULT 0;
  DECLARE v_rates TINYINT(1);
  DECLARE v_code VARCHAR(40);
  DECLARE v_year SMALLINT;
  DECLARE v_no INT UNSIGNED;
  DECLARE v_old_status VARCHAR(60);
  DECLARE v_new_status VARCHAR(60);
  DECLARE EXIT HANDLER FOR SQLEXCEPTION BEGIN ROLLBACK; RESIGNAL; END;

  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.write');
  CALL sp_rq_can_rates(p_actor_id, v_rates);
  IF p_title IS NULL OR TRIM(p_title) = '' OR p_request_date IS NULL OR p_due_date IS NULL OR p_due_date < p_request_date
     OR p_vacancies IS NULL OR JSON_LENGTH(p_vacancies) = 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION';
  END IF;
  CALL sp_master_resolve(p_tenant_id, 'CLIENT', p_client_uuid, v_client);
  CALL sp_master_resolve(p_tenant_id, 'RQ_STATUS', p_status_uuid, v_status);
  CALL sp_master_resolve(p_tenant_id, 'TIME_UNIT', p_duration_unit_uuid, v_du);
  CALL sp_master_resolve(p_tenant_id, 'TIME_UNIT', p_contract_unit_uuid, v_cu);
  CALL sp_master_resolve(p_tenant_id, 'WORK_MODALITY', p_work_modality_uuid, v_wm);
  IF v_client IS NULL OR v_status IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  IF p_owner_uuid IS NOT NULL THEN
    SELECT id INTO v_owner FROM app_user WHERE uuid = p_owner_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
    IF v_owner IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  END IF;
  SELECT code INTO v_new_status FROM master_item WHERE id = v_status;

  START TRANSACTION;
  SELECT r.id, r.version, s.code INTO v_id, v_version, v_old_status
    FROM requirement r JOIN master_item s ON s.id = r.status_item_id
   WHERE r.uuid = p_uuid AND r.tenant_id = p_tenant_id AND r.deleted_at IS NULL FOR UPDATE;
  IF v_id IS NOT NULL AND p_expected_version IS NOT NULL AND p_expected_version <> v_version THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VERSION_CONFLICT';
  END IF;

  SET v_code = NULLIF(TRIM(p_code), '');
  IF v_code IS NULL AND v_id IS NULL THEN
    SET v_year = YEAR(p_request_date);
    INSERT INTO requirement_sequence (tenant_id, year, last_no) VALUES (p_tenant_id, v_year, 1)
      ON DUPLICATE KEY UPDATE last_no = last_no + 1;
    SELECT last_no INTO v_no FROM requirement_sequence WHERE tenant_id = p_tenant_id AND year = v_year;
    SET v_code = CONCAT('RQ-', v_year, '-', LPAD(v_no, 4, '0'));
  END IF;
  IF v_code IS NOT NULL AND EXISTS (SELECT 1 FROM requirement WHERE tenant_id = p_tenant_id AND code = v_code
                                     AND deleted_at IS NULL AND (v_id IS NULL OR id <> v_id)) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:RQ_CODE_EXISTS';
  END IF;

  IF v_id IS NULL THEN
    SET v_created = 1;
    INSERT INTO requirement (uuid, tenant_id, code, client_item_id, title, description, status_item_id, request_date, due_date,
      duration_value, duration_unit_item_id, contract_value, contract_unit_item_id, work_modality_item_id,
      needs_emo, needs_sctr, needs_laptop, owner_user_id, created_by)
    VALUES (p_uuid, p_tenant_id, v_code, v_client, TRIM(p_title), NULLIF(TRIM(p_description), ''), v_status, p_request_date, p_due_date,
      p_duration_value, v_du, p_contract_value, v_cu, v_wm, IFNULL(p_emo, 0), IFNULL(p_sctr, 0), IFNULL(p_laptop, 0),
      IFNULL(v_owner, p_actor_id), p_actor_id);
    SET v_id = LAST_INSERT_ID(), v_version = 1;
  ELSE
    UPDATE requirement SET code = IFNULL(v_code, code), client_item_id = v_client, title = TRIM(p_title),
           description = NULLIF(TRIM(p_description), ''), status_item_id = v_status, request_date = p_request_date, due_date = p_due_date,
           duration_value = p_duration_value, duration_unit_item_id = v_du, contract_value = p_contract_value, contract_unit_item_id = v_cu,
           work_modality_item_id = v_wm, needs_emo = IFNULL(p_emo, 0), needs_sctr = IFNULL(p_sctr, 0), needs_laptop = IFNULL(p_laptop, 0),
           owner_user_id = IFNULL(v_owner, owner_user_id), version = version + 1, updated_by = p_actor_id
     WHERE id = v_id;
    SET v_version = v_version + 1;
  END IF;

  -- contactos: solo del mismo cliente
  DELETE FROM requirement_contact WHERE requirement_id = v_id;
  IF p_contacts IS NOT NULL THEN
    INSERT INTO requirement_contact (requirement_id, contact_id)
    SELECT DISTINCT v_id, cc.id FROM JSON_TABLE(p_contacts, '$[*]' COLUMNS (cid CHAR(36) PATH '$')) j
      JOIN client_contact cc ON cc.uuid = j.cid AND cc.tenant_id = p_tenant_id AND cc.client_item_id = v_client AND cc.deleted_at IS NULL;
  END IF;

  -- bandas salariales por modalidad de contrato (solo con permiso de tarifas)
  IF v_rates = 1 AND p_billing IS NOT NULL THEN
    DELETE FROM requirement_billing WHERE requirement_id = v_id;
    INSERT INTO requirement_billing (requirement_id, contract_item_id, currency_item_id, base_min, base_max, mobility_min, mobility_max,
      monthly_min, monthly_max, quarterly_min, quarterly_max, semiannual_min, semiannual_max)
    SELECT v_id, ct.id, cur.id, j.base_min, j.base_max, j.mobility_min, j.mobility_max, j.monthly_min, j.monthly_max,
           j.quarterly_min, j.quarterly_max, j.semiannual_min, j.semiannual_max
      FROM JSON_TABLE(p_billing, '$[*]' COLUMNS (
             contract CHAR(36) PATH '$.contractId', currency CHAR(36) PATH '$.currencyId',
             base_min DECIMAL(12,2) PATH '$.baseMin', base_max DECIMAL(12,2) PATH '$.baseMax',
             mobility_min DECIMAL(12,2) PATH '$.mobilityMin', mobility_max DECIMAL(12,2) PATH '$.mobilityMax',
             monthly_min DECIMAL(12,2) PATH '$.monthlyMin', monthly_max DECIMAL(12,2) PATH '$.monthlyMax',
             quarterly_min DECIMAL(12,2) PATH '$.quarterlyMin', quarterly_max DECIMAL(12,2) PATH '$.quarterlyMax',
             semiannual_min DECIMAL(12,2) PATH '$.semiannualMin', semiannual_max DECIMAL(12,2) PATH '$.semiannualMax')) j
      JOIN master_item ct ON ct.uuid = j.contract AND ct.deleted_at IS NULL
      JOIN master_table mct ON mct.id = ct.master_table_id AND mct.code = 'CONTRACT_MODALITY'
      JOIN master_item cur ON cur.uuid = j.currency AND cur.deleted_at IS NULL
      JOIN master_table mcur ON mcur.id = cur.master_table_id AND mcur.code = 'CURRENCY';
  END IF;

  -- vacantes: alta/edición por uuid; las que no vienen se dan de baja si no tienen candidatos
  DROP TEMPORARY TABLE IF EXISTS tmp_rq_vac;
  CREATE TEMPORARY TABLE tmp_rq_vac (idx INT, vid CHAR(36), profile CHAR(36), qty INT, r_ini DECIMAL(12,2), r_fin DECIMAL(12,2), skills JSON, careers JSON);
  INSERT INTO tmp_rq_vac
  SELECT j.idx, j.vid, j.profile, j.qty, j.r_ini, j.r_fin, j.skills, j.careers FROM JSON_TABLE(p_vacancies, '$[*]' COLUMNS (
    idx FOR ORDINALITY, vid CHAR(36) PATH '$.id', profile CHAR(36) PATH '$.profileId', qty INT PATH '$.quantity',
    r_ini DECIMAL(12,2) PATH '$.rateInitial', r_fin DECIMAL(12,2) PATH '$.rateFinal',
    skills JSON PATH '$.skills', careers JSON PATH '$.careers')) j;
  IF EXISTS (SELECT 1 FROM tmp_rq_vac WHERE vid IS NULL OR qty IS NULL OR qty < 1) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION';
  END IF;
  IF EXISTS (SELECT 1 FROM tmp_rq_vac t LEFT JOIN master_item p ON p.uuid = t.profile AND p.deleted_at IS NULL
               AND (p.tenant_id IS NULL OR p.tenant_id = p_tenant_id)
               AND p.master_table_id = (SELECT id FROM master_table WHERE code = 'RQ_PROFILE') WHERE p.id IS NULL) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:MASTER_ITEM_INVALID';
  END IF;
  IF EXISTS (SELECT 1 FROM requirement_vacancy v WHERE v.requirement_id = v_id AND v.deleted_at IS NULL
               AND v.uuid NOT IN (SELECT vid FROM tmp_rq_vac)
               AND EXISTS (SELECT 1 FROM requirement_candidate rc WHERE rc.vacancy_id = v.id AND rc.deleted_at IS NULL)) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VACANCY_HAS_CANDIDATES';
  END IF;
  UPDATE requirement_vacancy SET deleted_at = NOW(3)
   WHERE requirement_id = v_id AND deleted_at IS NULL AND uuid NOT IN (SELECT vid FROM tmp_rq_vac);
  INSERT INTO requirement_vacancy (uuid, requirement_id, profile_item_id, quantity, rate_initial, rate_final, sort_order)
  SELECT t.vid, v_id, p.id, t.qty, IF(v_rates = 1, t.r_ini, NULL), IF(v_rates = 1, t.r_fin, NULL), t.idx
    FROM tmp_rq_vac t JOIN master_item p ON p.uuid = t.profile
  ON DUPLICATE KEY UPDATE profile_item_id = VALUES(profile_item_id), quantity = VALUES(quantity), sort_order = VALUES(sort_order),
    rate_initial = IF(v_rates = 1, VALUES(rate_initial), rate_initial), rate_final = IF(v_rates = 1, VALUES(rate_final), rate_final),
    deleted_at = NULL;
  IF EXISTS (SELECT 1 FROM requirement_vacancy v JOIN tmp_rq_vac t ON t.vid = v.uuid WHERE v.requirement_id <> v_id) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; -- uuid de vacante de otro RQ
  END IF;
  -- la cantidad no puede quedar por debajo de los ya confirmados
  IF EXISTS (SELECT 1 FROM requirement_vacancy v WHERE v.requirement_id = v_id AND v.deleted_at IS NULL
               AND v.quantity < (SELECT COUNT(*) FROM requirement_candidate rc JOIN master_item cs ON cs.id = rc.stage_item_id
                                  WHERE rc.vacancy_id = v.id AND rc.deleted_at IS NULL AND cs.code = 'CONFIRMED')) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VACANCY_BELOW_CONFIRMED';
  END IF;

  DELETE vs FROM vacancy_skill vs JOIN requirement_vacancy v ON v.id = vs.vacancy_id WHERE v.requirement_id = v_id;
  INSERT IGNORE INTO vacancy_skill (vacancy_id, skill_item_id, years, optional)
  SELECT v.id, sk.id, LEAST(GREATEST(IFNULL(s.years, 0), 0), 60), IFNULL(s.opt, 0)
    FROM tmp_rq_vac t JOIN requirement_vacancy v ON v.uuid = t.vid
    JOIN JSON_TABLE(IFNULL(t.skills, JSON_ARRAY()), '$[*]' COLUMNS (sid CHAR(36) PATH '$.skillId', years DECIMAL(4,1) PATH '$.years',
         opt TINYINT PATH '$.optional')) s
    JOIN master_item sk ON sk.uuid = s.sid AND sk.deleted_at IS NULL AND (sk.tenant_id IS NULL OR sk.tenant_id = p_tenant_id)
    JOIN master_table mt ON mt.id = sk.master_table_id AND mt.code = 'TECH_SKILL';
  DELETE vc FROM vacancy_career vc JOIN requirement_vacancy v ON v.id = vc.vacancy_id WHERE v.requirement_id = v_id;
  INSERT INTO vacancy_career (vacancy_id, career, grade_item_id, optional)
  SELECT v.id, LEFT(TRIM(c.career), 150), g.id, IFNULL(c.opt, 0)
    FROM tmp_rq_vac t JOIN requirement_vacancy v ON v.uuid = t.vid
    JOIN JSON_TABLE(IFNULL(t.careers, JSON_ARRAY()), '$[*]' COLUMNS (career VARCHAR(300) PATH '$.career', gid CHAR(36) PATH '$.gradeId',
         opt TINYINT PATH '$.optional')) c
    LEFT JOIN master_item g ON g.uuid = c.gid AND g.deleted_at IS NULL
   WHERE c.career IS NOT NULL AND TRIM(c.career) <> '';
  DROP TEMPORARY TABLE IF EXISTS tmp_rq_vac;

  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, IF(v_created = 1, 'CREATE', 'UPDATE'), 'requirement', p_uuid,
       JSON_OBJECT('code', IFNULL(v_code, (SELECT code FROM requirement WHERE id = v_id)), 'status', v_new_status, 'previousStatus', v_old_status), p_ip);
  COMMIT;
  SELECT r.uuid, v_created AS created, r.version, r.code, v_old_status AS previous_status, v_new_status AS status, r.title,
         JSON_UNQUOTE(JSON_EXTRACT(c.label_i18n, '$.es')) AS client
    FROM requirement r JOIN master_item c ON c.id = r.client_item_id WHERE r.id = v_id;
END $$

-- Mover el RQ en el tablero: cambia el estado (y el orden en la columna)
DROP PROCEDURE IF EXISTS sp_rq_set_status $$
CREATE PROCEDURE sp_rq_set_status(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_status_uuid CHAR(36), IN p_board_order INT, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id, v_status BIGINT UNSIGNED;
  DECLARE v_closed TINYINT(1);
  DECLARE v_old VARCHAR(60);
  DECLARE v_new VARCHAR(60);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.write');
  CALL sp_rq_resolve(p_tenant_id, p_uuid, v_id, v_closed);
  CALL sp_master_resolve(p_tenant_id, 'RQ_STATUS', p_status_uuid, v_status);
  IF v_status IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  SELECT s.code INTO v_old FROM requirement r JOIN master_item s ON s.id = r.status_item_id WHERE r.id = v_id;
  SELECT code INTO v_new FROM master_item WHERE id = v_status;
  UPDATE requirement SET status_item_id = v_status, board_order = IFNULL(p_board_order, board_order),
         version = version + IF(status_item_id = v_status, 0, 1), updated_by = p_actor_id
   WHERE id = v_id;
  IF v_old <> v_new THEN
    CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'STATUS', 'requirement', p_uuid, JSON_OBJECT('from', v_old, 'to', v_new), p_ip);
  END IF;
  SELECT r.uuid, r.version, r.code, v_old AS previous_status, v_new AS status, r.title,
         JSON_UNQUOTE(JSON_EXTRACT(c.label_i18n, '$.es')) AS client
    FROM requirement r JOIN master_item c ON c.id = r.client_item_id WHERE r.id = v_id;
END $$

DROP PROCEDURE IF EXISTS sp_rq_delete $$
CREATE PROCEDURE sp_rq_delete(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_closed TINYINT(1);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.write');
  CALL sp_rq_resolve(p_tenant_id, p_uuid, v_id, v_closed);
  IF EXISTS (SELECT 1 FROM requirement_candidate rc JOIN master_item cs ON cs.id = rc.stage_item_id
              WHERE rc.requirement_id = v_id AND rc.deleted_at IS NULL AND cs.code = 'CONFIRMED') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:RQ_HAS_CONFIRMED';
  END IF;
  UPDATE requirement SET deleted_at = NOW(3), deleted_by = p_actor_id WHERE id = v_id;
  UPDATE requirement_candidate SET deleted_at = NOW(3) WHERE requirement_id = v_id AND deleted_at IS NULL;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'DELETE', 'requirement', p_uuid, NULL, p_ip);
END $$

-- ---------------- Candidatos (pipeline) ----------------

-- Agrega un talento a una vacante del RQ (etapa inicial: Observado). RQ cerrado = bloqueado.
DROP PROCEDURE IF EXISTS sp_rq_candidate_add $$
CREATE PROCEDURE sp_rq_candidate_add(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_rq_uuid CHAR(36),
  IN p_uuid CHAR(36), IN p_talent_uuid CHAR(36), IN p_vacancy_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_rq, v_vac, v_talent, v_stage, v_existing BIGINT UNSIGNED;
  DECLARE v_closed TINYINT(1);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.candidates');
  CALL sp_rq_resolve(p_tenant_id, p_rq_uuid, v_rq, v_closed);
  IF v_closed = 1 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:RQ_CLOSED'; END IF;
  SELECT id INTO v_vac FROM requirement_vacancy WHERE uuid = p_vacancy_uuid AND requirement_id = v_rq AND deleted_at IS NULL;
  SELECT id INTO v_talent FROM talent WHERE uuid = p_talent_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
  IF v_vac IS NULL OR v_talent IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  SELECT id INTO v_existing FROM requirement_candidate WHERE requirement_id = v_rq AND talent_id = v_talent AND deleted_at IS NULL;
  IF v_existing IS NOT NULL THEN
    IF EXISTS (SELECT 1 FROM requirement_candidate WHERE id = v_existing AND uuid = p_uuid) THEN
      SELECT p_uuid AS uuid, 0 AS created; -- reintento de la misma operación
    ELSE
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:CANDIDATE_EXISTS';
    END IF;
  ELSE
    SELECT i.id INTO v_stage FROM master_item i JOIN master_table m ON m.id = i.master_table_id
     WHERE m.code = 'CANDIDATE_STAGE' AND i.code = 'OBSERVED';
    INSERT INTO requirement_candidate (uuid, requirement_id, vacancy_id, talent_id, stage_item_id, created_by)
    VALUES (p_uuid, v_rq, v_vac, v_talent, v_stage, p_actor_id);
    CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'CANDIDATE_ADD', 'requirement', p_rq_uuid, JSON_OBJECT('talent', p_talent_uuid), p_ip);
    SELECT p_uuid AS uuid, 1 AS created;
  END IF;
END $$

-- Mueve el candidato de etapa (y opcionalmente de vacante / orden). Confirmar exige haber pasado de "Observado"
-- y que la vacante tenga cupo (confirmados < cantidad).
DROP PROCEDURE IF EXISTS sp_rq_candidate_move $$
CREATE PROCEDURE sp_rq_candidate_move(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_stage_uuid CHAR(36), IN p_vacancy_uuid CHAR(36), IN p_board_order INT, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id, v_rq, v_vac, v_new_vac, v_stage BIGINT UNSIGNED;
  DECLARE v_rq_uuid CHAR(36);
  DECLARE v_closed TINYINT(1);
  DECLARE v_from VARCHAR(60);
  DECLARE v_to VARCHAR(60);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.candidates');
  SELECT rc.id, rc.requirement_id, rc.vacancy_id, cs.code, r.uuid, (s.str1 = 'CLOSED')
    INTO v_id, v_rq, v_vac, v_from, v_rq_uuid, v_closed
    FROM requirement_candidate rc
    JOIN requirement r ON r.id = rc.requirement_id AND r.tenant_id = p_tenant_id AND r.deleted_at IS NULL
    JOIN master_item s ON s.id = r.status_item_id
    JOIN master_item cs ON cs.id = rc.stage_item_id
   WHERE rc.uuid = p_uuid AND rc.deleted_at IS NULL;
  IF v_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  IF v_closed = 1 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:RQ_CLOSED'; END IF;
  CALL sp_master_resolve(p_tenant_id, 'CANDIDATE_STAGE', p_stage_uuid, v_stage);
  IF v_stage IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  SELECT code INTO v_to FROM master_item WHERE id = v_stage;
  SET v_new_vac = v_vac;
  IF p_vacancy_uuid IS NOT NULL THEN
    SELECT id INTO v_new_vac FROM requirement_vacancy WHERE uuid = p_vacancy_uuid AND requirement_id = v_rq AND deleted_at IS NULL;
    IF v_new_vac IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  END IF;
  IF v_to = 'CONFIRMED' AND (v_from <> 'CONFIRMED' OR v_new_vac <> v_vac) THEN
    IF v_from = 'OBSERVED' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:CANDIDATE_NOT_READY'; END IF;
    IF (SELECT COUNT(*) FROM requirement_candidate rc JOIN master_item cs ON cs.id = rc.stage_item_id
         WHERE rc.vacancy_id = v_new_vac AND rc.deleted_at IS NULL AND cs.code = 'CONFIRMED' AND rc.id <> v_id)
       >= (SELECT quantity FROM requirement_vacancy WHERE id = v_new_vac) THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VACANCY_FULL';
    END IF;
  END IF;
  UPDATE requirement_candidate SET stage_item_id = v_stage, vacancy_id = v_new_vac, board_order = IFNULL(p_board_order, board_order),
         updated_by = p_actor_id WHERE id = v_id;
  UPDATE requirement SET updated_by = p_actor_id, updated_at = NOW(3) WHERE id = v_rq;
  IF v_from <> v_to THEN
    CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'CANDIDATE_STAGE', 'requirement', v_rq_uuid,
         JSON_OBJECT('candidate', p_uuid, 'from', v_from, 'to', v_to), p_ip);
  END IF;
  SELECT v_rq_uuid AS rq_uuid, v_from AS previous_stage, v_to AS stage;
END $$

DROP PROCEDURE IF EXISTS sp_rq_candidate_remove $$
CREATE PROCEDURE sp_rq_candidate_remove(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id, v_rq BIGINT UNSIGNED;
  DECLARE v_rq_uuid CHAR(36);
  DECLARE v_closed TINYINT(1);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.candidates');
  SELECT rc.id, r.id, r.uuid, (s.str1 = 'CLOSED') INTO v_id, v_rq, v_rq_uuid, v_closed
    FROM requirement_candidate rc
    JOIN requirement r ON r.id = rc.requirement_id AND r.tenant_id = p_tenant_id AND r.deleted_at IS NULL
    JOIN master_item s ON s.id = r.status_item_id
   WHERE rc.uuid = p_uuid AND rc.deleted_at IS NULL;
  IF v_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  IF v_closed = 1 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:RQ_CLOSED'; END IF;
  UPDATE requirement_candidate SET deleted_at = NOW(3), updated_by = p_actor_id WHERE id = v_id;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'CANDIDATE_REMOVE', 'requirement', v_rq_uuid, JSON_OBJECT('candidate', p_uuid), p_ip);
END $$

-- Postulaciones de un talento (ficha del talento)
DROP PROCEDURE IF EXISTS sp_rq_by_talent $$
CREATE PROCEDURE sp_rq_by_talent(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_talent_uuid CHAR(36))
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.read');
  SELECT JSON_OBJECT('candidateId', rc.uuid, 'requirementId', r.uuid, 'code', r.code, 'title', r.title, 'dueDate', r.due_date,
           'client', c.label_i18n, 'profile', p.label_i18n,
           'status', JSON_OBJECT('code', s.code, 'label', s.label_i18n, 'group', s.str1),
           'stage', JSON_OBJECT('code', cs.code, 'label', cs.label_i18n), 'addedAt', rc.created_at) AS doc
    FROM requirement_candidate rc
    JOIN requirement r ON r.id = rc.requirement_id AND r.deleted_at IS NULL AND r.tenant_id = p_tenant_id
    JOIN talent t ON t.id = rc.talent_id AND t.uuid = p_talent_uuid
    JOIN master_item s ON s.id = r.status_item_id
    JOIN master_item c ON c.id = r.client_item_id
    JOIN requirement_vacancy v ON v.id = rc.vacancy_id
    JOIN master_item p ON p.id = v.profile_item_id
    JOIN master_item cs ON cs.id = rc.stage_item_id
   WHERE rc.deleted_at IS NULL
   ORDER BY rc.created_at DESC;
END $$

-- ---------------- Contactos del cliente ----------------

DROP PROCEDURE IF EXISTS sp_client_contacts $$
CREATE PROCEDURE sp_client_contacts(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_client_uuid CHAR(36))
BEGIN
  DECLARE v_client BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.read');
  CALL sp_master_resolve(p_tenant_id, 'CLIENT', p_client_uuid, v_client);
  SELECT JSON_OBJECT('id', uuid, 'firstName', first_name, 'lastName', last_name, 'secondLastName', second_last_name,
           'position', position, 'phone', phone, 'phone2', phone2, 'email', email, 'email2', email2) AS doc
    FROM client_contact WHERE tenant_id = p_tenant_id AND client_item_id = v_client AND deleted_at IS NULL
   ORDER BY first_name, last_name;
END $$

DROP PROCEDURE IF EXISTS sp_client_contact_save $$
CREATE PROCEDURE sp_client_contact_save(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_client_uuid CHAR(36), IN p_first VARCHAR(100), IN p_last VARCHAR(100), IN p_second VARCHAR(100), IN p_position VARCHAR(150),
  IN p_phone VARCHAR(30), IN p_phone2 VARCHAR(30), IN p_email VARCHAR(254), IN p_email2 VARCHAR(254), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_client BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.write');
  CALL sp_master_resolve(p_tenant_id, 'CLIENT', p_client_uuid, v_client);
  IF v_client IS NULL OR TRIM(IFNULL(p_first, '')) = '' OR TRIM(IFNULL(p_last, '')) = '' THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION';
  END IF;
  IF EXISTS (SELECT 1 FROM client_contact WHERE uuid = p_uuid AND (tenant_id <> p_tenant_id OR client_item_id <> v_client)) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FORBIDDEN';
  END IF;
  INSERT INTO client_contact (uuid, tenant_id, client_item_id, first_name, last_name, second_last_name, position, phone, phone2, email, email2, created_by)
  VALUES (p_uuid, p_tenant_id, v_client, TRIM(p_first), TRIM(p_last), NULLIF(TRIM(p_second), ''), NULLIF(TRIM(p_position), ''),
          NULLIF(TRIM(p_phone), ''), NULLIF(TRIM(p_phone2), ''), NULLIF(LOWER(TRIM(p_email)), ''), NULLIF(LOWER(TRIM(p_email2)), ''), p_actor_id)
  ON DUPLICATE KEY UPDATE first_name = VALUES(first_name), last_name = VALUES(last_name), second_last_name = VALUES(second_last_name),
    position = VALUES(position), phone = VALUES(phone), phone2 = VALUES(phone2), email = VALUES(email), email2 = VALUES(email2), deleted_at = NULL;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'UPSERT', 'client_contact', p_uuid, NULL, p_ip);
END $$

DROP PROCEDURE IF EXISTS sp_client_contact_delete $$
CREATE PROCEDURE sp_client_contact_delete(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.write');
  UPDATE client_contact SET deleted_at = NOW(3) WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
  IF ROW_COUNT() = 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'DELETE', 'client_contact', p_uuid, NULL, p_ip);
END $$

DELIMITER ;
