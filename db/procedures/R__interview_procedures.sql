-- =====================================================================
-- Entrevistas (virtual, presencial, telefónica)
-- Reglas por tipo y etapa; entrevistadores, grabaciones, respuestas del cuestionario telefónico.
-- =====================================================================

DELIMITER $$

-- Resuelve un valor de catálogo devolviendo id y código
DROP PROCEDURE IF EXISTS sp_master_resolve_code $$
CREATE PROCEDURE sp_master_resolve_code(IN p_tenant_id BIGINT UNSIGNED, IN p_table_code VARCHAR(60), IN p_uuid CHAR(36),
  OUT o_id BIGINT UNSIGNED, OUT o_code VARCHAR(60))
BEGIN
  SET o_id = NULL, o_code = NULL;
  IF p_uuid IS NOT NULL AND p_uuid <> '' THEN
    SELECT i.id, i.code INTO o_id, o_code FROM master_item i JOIN master_table m ON m.id = i.master_table_id
     WHERE i.uuid = p_uuid AND m.code = p_table_code AND i.deleted_at IS NULL
       AND (i.tenant_id IS NULL OR i.tenant_id = p_tenant_id);
    IF o_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:MASTER_ITEM_INVALID'; END IF;
  END IF;
END $$

DROP PROCEDURE IF EXISTS sp_interview_resolve $$
CREATE PROCEDURE sp_interview_resolve(IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), OUT o_id BIGINT UNSIGNED)
BEGIN
  SET o_id = NULL;
  SELECT id INTO o_id FROM interview WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
  IF o_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_NOT_FOUND'; END IF;
END $$

-- Alta / edición. p_interviewers = [{"name","emailEnc"(hex),"notify"}], p_recordings = [{"url","date"}]
DROP PROCEDURE IF EXISTS sp_interview_save $$
CREATE PROCEDURE sp_interview_save(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_talent_uuid CHAR(36), IN p_type_uuid CHAR(36), IN p_stage_uuid CHAR(36), IN p_status_uuid CHAR(36),
  IN p_duration_uuid CHAR(36), IN p_client_uuid CHAR(36), IN p_requirement_ref VARCHAR(100), IN p_profile VARCHAR(200),
  IN p_date DATE, IN p_time TIME, IN p_link VARCHAR(500), IN p_location_url VARCHAR(500), IN p_address VARCHAR(500),
  IN p_r_general TINYINT, IN p_r_personal TINYINT, IN p_r_experience TINYINT, IN p_r_languages TINYINT, IN p_r_education TINYINT,
  IN p_n_personal TEXT, IN p_n_experience TEXT, IN p_n_languages TEXT, IN p_n_education TEXT, IN p_cancel_reason TEXT,
  IN p_interviewers JSON, IN p_recordings JSON, IN p_ack_blacklist TINYINT(1), IN p_expected_version INT, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_talent BIGINT UNSIGNED;
  DECLARE v_old_talent BIGINT UNSIGNED;
  DECLARE v_version INT;
  DECLARE v_type BIGINT UNSIGNED; DECLARE v_type_code VARCHAR(60);
  DECLARE v_stage BIGINT UNSIGNED; DECLARE v_stage_code VARCHAR(60);
  DECLARE v_status BIGINT UNSIGNED; DECLARE v_status_code VARCHAR(60);
  DECLARE v_duration BIGINT UNSIGNED; DECLARE v_dur_code VARCHAR(60);
  DECLARE v_client BIGINT UNSIGNED;
  DECLARE v_blocked TINYINT(1) DEFAULT 0;
  DECLARE v_n_interviewers INT;
  DECLARE v_bad INT;
  DECLARE v_created TINYINT(1) DEFAULT 0;
  DECLARE EXIT HANDLER FOR SQLEXCEPTION BEGIN ROLLBACK; RESIGNAL; END;

  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'interview.write');
  CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent);
  CALL sp_master_resolve_code(p_tenant_id, 'INTERVIEW_TYPE', p_type_uuid, v_type, v_type_code);
  CALL sp_master_resolve_code(p_tenant_id, 'INTERVIEW_STAGE', p_stage_uuid, v_stage, v_stage_code);
  CALL sp_master_resolve_code(p_tenant_id, 'INTERVIEW_STATUS', p_status_uuid, v_status, v_status_code);
  CALL sp_master_resolve_code(p_tenant_id, 'INTERVIEW_DURATION', p_duration_uuid, v_duration, v_dur_code);
  CALL sp_master_resolve(p_tenant_id, 'CLIENT', p_client_uuid, v_client);
  IF v_type IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_TYPE_REQUIRED'; END IF;
  IF v_stage IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_STAGE_REQUIRED'; END IF;
  IF v_status IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_STATUS_REQUIRED'; END IF;
  IF p_date IS NULL OR p_time IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_DATETIME_REQUIRED'; END IF;
  IF TRIM(IFNULL(p_profile, '')) = '' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_PROFILE_REQUIRED'; END IF;
  IF IFNULL(p_r_general,0) NOT BETWEEN 0 AND 5 OR IFNULL(p_r_personal,0) NOT BETWEEN 0 AND 5 OR IFNULL(p_r_experience,0) NOT BETWEEN 0 AND 5
     OR IFNULL(p_r_languages,0) NOT BETWEEN 0 AND 5 OR IFNULL(p_r_education,0) NOT BETWEEN 0 AND 5 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_STARS';
  END IF;

  -- Reglas por tipo
  IF v_type_code = 'VIRTUAL' THEN
    IF IFNULL(p_link, '') NOT REGEXP '^https://[^ ]+$' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_LINK_REQUIRED'; END IF;
    SET p_location_url = NULL, p_address = NULL;
  ELSEIF v_type_code = 'IN_PERSON' THEN
    IF IFNULL(p_location_url, '') NOT REGEXP '^https://(maps\\.app\\.goo\\.gl|goo\\.gl/maps|maps\\.google\\.[a-z.]+|(www\\.)?google\\.[a-z.]+/maps)' THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_MAPS_REQUIRED';
    END IF;
    IF TRIM(IFNULL(p_address, '')) = '' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_ADDRESS_REQUIRED'; END IF;
    SET p_link = NULL;
  ELSE -- PHONE: sin enlace, ubicación, dirección, duración ni grabaciones
    SET p_link = NULL, p_location_url = NULL, p_address = NULL, v_duration = NULL, p_recordings = JSON_ARRAY();
  END IF;
  -- En entrevistas no telefónicas el cliente es obligatorio (equivale al RQ del sistema anterior)
  IF v_type_code <> 'PHONE' AND v_client IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_CLIENT_REQUIRED'; END IF;
  IF v_type_code <> 'VIRTUAL' AND JSON_LENGTH(IFNULL(p_recordings, JSON_ARRAY())) > 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_RECORDINGS_VIRTUAL_ONLY';
  END IF;

  -- Entrevistadores: obligatorios salvo en la etapa con el equipo de R&S; email obligatorio si se notifica
  SET p_interviewers = IFNULL(p_interviewers, JSON_ARRAY());
  SELECT COUNT(*) INTO v_n_interviewers FROM JSON_TABLE(p_interviewers, '$[*]' COLUMNS(name VARCHAR(150) PATH '$.name')) j
   WHERE TRIM(IFNULL(j.name, '')) <> '';
  IF v_stage_code <> 'RS_TEAM' AND v_n_interviewers = 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEWERS_REQUIRED'; END IF;
  SELECT COUNT(*) INTO v_bad FROM JSON_TABLE(p_interviewers, '$[*]' COLUMNS(name VARCHAR(150) PATH '$.name', email VARCHAR(1024) PATH '$.emailEnc', notify INT PATH '$.notify')) j
   WHERE TRIM(IFNULL(j.name, '')) = '' OR (IFNULL(j.notify, 0) = 1 AND IFNULL(j.email, '') = '');
  IF v_bad > 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEWER_INVALID'; END IF;
  SELECT COUNT(*) INTO v_bad FROM JSON_TABLE(IFNULL(p_recordings, JSON_ARRAY()), '$[*]' COLUMNS(url VARCHAR(500) PATH '$.url', d VARCHAR(20) PATH '$.date')) j
   WHERE IFNULL(j.url, '') NOT REGEXP '^https://[^ ]+$' OR IFNULL(j.d, '') NOT REGEXP '^[0-9]{4}-[0-9]{2}-[0-9]{2}$';
  IF v_bad > 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:RECORDING_INVALID'; END IF;

  SELECT id, talent_id, version INTO v_id, v_old_talent, v_version FROM interview WHERE uuid = p_uuid AND deleted_at IS NULL;
  IF v_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM interview WHERE id = v_id AND tenant_id = p_tenant_id) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TENANT_MISMATCH';
  END IF;
  IF v_id IS NOT NULL AND v_old_talent <> v_talent THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_TALENT_IMMUTABLE'; END IF;
  IF v_id IS NOT NULL AND p_expected_version IS NOT NULL AND p_expected_version <> v_version THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VERSION_CONFLICT';
  END IF;

  -- Lista negra: aviso que se puede confirmar (igual que el sistema anterior)
  IF v_client IS NOT NULL THEN
    CALL sp_blacklist_check(v_talent, v_client, v_blocked);
    IF v_blocked = 1 AND IFNULL(p_ack_blacklist, 0) = 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TALENT_BLACKLISTED'; END IF;
  END IF;
  IF v_status_code <> 'CANCELLED' THEN SET p_cancel_reason = NULL; END IF;

  START TRANSACTION;
  IF v_id IS NULL THEN
    INSERT INTO interview (uuid, tenant_id, talent_id, type_item_id, stage_item_id, status_item_id, duration_item_id, client_item_id,
      requirement_ref, profile, scheduled_date, scheduled_time, meeting_link, location_url, address,
      rating_general, rating_personal, rating_experience, rating_languages, rating_education,
      notes_personal, notes_experience, notes_languages, notes_education, cancel_reason, created_by)
    VALUES (p_uuid, p_tenant_id, v_talent, v_type, v_stage, v_status, v_duration, v_client,
      NULLIF(TRIM(p_requirement_ref), ''), TRIM(p_profile), p_date, p_time, p_link, p_location_url, NULLIF(TRIM(p_address), ''),
      IFNULL(p_r_general,0), IFNULL(p_r_personal,0), IFNULL(p_r_experience,0), IFNULL(p_r_languages,0), IFNULL(p_r_education,0),
      p_n_personal, p_n_experience, p_n_languages, p_n_education, p_cancel_reason, p_actor_id);
    SET v_id = LAST_INSERT_ID();
    SET v_created = 1;
  ELSE
    UPDATE interview SET type_item_id = v_type, stage_item_id = v_stage, status_item_id = v_status, duration_item_id = v_duration,
           client_item_id = v_client, requirement_ref = NULLIF(TRIM(p_requirement_ref), ''), profile = TRIM(p_profile),
           scheduled_date = p_date, scheduled_time = p_time, meeting_link = p_link, location_url = p_location_url,
           address = NULLIF(TRIM(p_address), ''), rating_general = IFNULL(p_r_general,0), rating_personal = IFNULL(p_r_personal,0),
           rating_experience = IFNULL(p_r_experience,0), rating_languages = IFNULL(p_r_languages,0), rating_education = IFNULL(p_r_education,0),
           notes_personal = p_n_personal, notes_experience = p_n_experience, notes_languages = p_n_languages,
           notes_education = p_n_education, cancel_reason = p_cancel_reason, version = version + 1, updated_by = p_actor_id
     WHERE id = v_id;
  END IF;
  DELETE FROM interview_interviewer WHERE interview_id = v_id;
  INSERT INTO interview_interviewer (interview_id, full_name, email_enc, notify, sort_order)
  SELECT v_id, TRIM(j.name), IF(IFNULL(j.email, '') = '', NULL, UNHEX(j.email)), IFNULL(j.notify, 0), j.ord
    FROM JSON_TABLE(p_interviewers, '$[*]' COLUMNS(ord FOR ORDINALITY, name VARCHAR(150) PATH '$.name',
         email VARCHAR(1024) PATH '$.emailEnc', notify INT PATH '$.notify')) j;
  DELETE FROM interview_recording WHERE interview_id = v_id;
  INSERT INTO interview_recording (interview_id, url, recorded_on)
  SELECT v_id, j.url, j.d FROM JSON_TABLE(IFNULL(p_recordings, JSON_ARRAY()), '$[*]' COLUMNS(url VARCHAR(500) PATH '$.url', d DATE PATH '$.date')) j;
  COMMIT;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, IF(v_created = 1, 'CREATE', 'UPDATE'), 'interview', p_uuid,
       JSON_OBJECT('type', v_type_code, 'stage', v_stage_code, 'status', v_status_code, 'blacklistAck', v_blocked), p_ip);
  SELECT uuid, version, v_created AS created, v_type_code AS type_code FROM interview WHERE id = v_id;
END $$

-- Respuestas del cuestionario telefónico: p_answers = [{"question":"uuid","answer":"..."}]
-- Concilia: guarda las que tienen texto y elimina las que quedaron vacías.
DROP PROCEDURE IF EXISTS sp_interview_answers_save $$
CREATE PROCEDURE sp_interview_answers_save(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_answers JSON)
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_type_code VARCHAR(60);
  DECLARE v_bad INT;
  DECLARE EXIT HANDLER FOR SQLEXCEPTION BEGIN ROLLBACK; RESIGNAL; END;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'interview.write');
  CALL sp_interview_resolve(p_tenant_id, p_uuid, v_id);
  SELECT ti.code INTO v_type_code FROM interview iv JOIN master_item ti ON ti.id = iv.type_item_id WHERE iv.id = v_id;
  IF v_type_code <> 'PHONE' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INTERVIEW_NOT_PHONE'; END IF;
  SET p_answers = IFNULL(p_answers, JSON_ARRAY());
  SELECT COUNT(*) INTO v_bad FROM JSON_TABLE(p_answers, '$[*]' COLUMNS(q CHAR(36) PATH '$.question')) j
    LEFT JOIN master_item i ON i.uuid = j.q AND (i.tenant_id IS NULL OR i.tenant_id = p_tenant_id)
    LEFT JOIN master_table m ON m.id = i.master_table_id AND m.code = 'PHONE_QUESTION'
   WHERE m.id IS NULL;
  IF v_bad > 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:MASTER_ITEM_INVALID'; END IF;
  START TRANSACTION;
  DELETE a FROM interview_answer a JOIN master_item i ON i.id = a.question_item_id
   WHERE a.interview_id = v_id AND i.uuid IN (
     SELECT j.q FROM JSON_TABLE(p_answers, '$[*]' COLUMNS(q CHAR(36) PATH '$.question', ans TEXT PATH '$.answer')) j WHERE TRIM(IFNULL(j.ans, '')) = '');
  INSERT INTO interview_answer (interview_id, question_item_id, answer, created_by)
  SELECT v_id, i.id, TRIM(j.ans), p_actor_id
    FROM JSON_TABLE(p_answers, '$[*]' COLUMNS(q CHAR(36) PATH '$.question', ans TEXT PATH '$.answer')) j
    JOIN master_item i ON i.uuid = j.q
   WHERE TRIM(IFNULL(j.ans, '')) <> ''
  ON DUPLICATE KEY UPDATE answer = VALUES(answer), updated_by = p_actor_id;
  UPDATE interview SET version = version + 1, updated_by = p_actor_id WHERE id = v_id;
  COMMIT;
END $$

-- Detalle (5 result sets)
DROP PROCEDURE IF EXISTS sp_interview_get $$
CREATE PROCEDURE sp_interview_get(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'interview.read');
  CALL sp_interview_resolve(p_tenant_id, p_uuid, v_id);
  SELECT iv.uuid, t.uuid AS talent_uuid, t.first_name, t.last_name, t.second_last_name, t.headline,
         ti.uuid AS type_uuid, ti.code AS type_code, ti.label_i18n AS type_label,
         st.uuid AS stage_uuid, st.code AS stage_code, st.label_i18n AS stage_label,
         ss.uuid AS status_uuid, ss.code AS status_code, ss.label_i18n AS status_label,
         du.uuid AS duration_uuid, CAST(du.num1 AS SIGNED) AS duration_minutes,
         cl.uuid AS client_uuid, cl.label_i18n AS client_label, iv.requirement_ref, iv.profile,
         iv.scheduled_date, iv.scheduled_time, iv.meeting_link, iv.location_url, iv.address,
         iv.rating_general, iv.rating_personal, iv.rating_experience, iv.rating_languages, iv.rating_education,
         iv.notes_personal, iv.notes_experience, iv.notes_languages, iv.notes_education, iv.cancel_reason,
         iv.version, iv.created_at, IFNULL(iv.updated_at, iv.created_at) AS updated_at,
         EXISTS (SELECT 1 FROM talent_blacklist b WHERE b.talent_id = t.id AND b.deleted_at IS NULL
                   AND (b.client_item_id IS NULL OR b.client_item_id = iv.client_item_id)) AS talent_blacklisted
    FROM interview iv
    JOIN talent t ON t.id = iv.talent_id
    JOIN master_item ti ON ti.id = iv.type_item_id
    JOIN master_item st ON st.id = iv.stage_item_id
    JOIN master_item ss ON ss.id = iv.status_item_id
    LEFT JOIN master_item du ON du.id = iv.duration_item_id
    LEFT JOIN master_item cl ON cl.id = iv.client_item_id
   WHERE iv.id = v_id;
  SELECT full_name, email_enc, notify FROM interview_interviewer WHERE interview_id = v_id ORDER BY sort_order, id;
  SELECT url, recorded_on FROM interview_recording WHERE interview_id = v_id ORDER BY recorded_on, id;
  -- todas las preguntas vigentes + respuestas guardadas (las de preguntas retiradas también)
  SELECT q.uuid AS question_uuid, q.label_i18n AS question, a.answer, (q.deleted_at IS NULL AND q.is_active = 1) AS active, q.sort_order
    FROM master_item q JOIN master_table m ON m.id = q.master_table_id AND m.code = 'PHONE_QUESTION'
    LEFT JOIN interview_answer a ON a.question_item_id = q.id AND a.interview_id = v_id
   WHERE (q.tenant_id IS NULL OR q.tenant_id = p_tenant_id) AND ((q.deleted_at IS NULL AND q.is_active = 1) OR a.id IS NOT NULL)
   ORDER BY active DESC, q.sort_order, q.id;
  SELECT f.uuid, f.category, f.original_name, f.mime_type, f.size_bytes, f.created_at
    FROM file_object f WHERE f.tenant_id = p_tenant_id AND f.owner_type = 'INTERVIEW' AND f.owner_id = v_id AND f.deleted_at IS NULL
   ORDER BY f.created_at DESC;
END $$

DROP PROCEDURE IF EXISTS sp_interview_search $$
CREATE PROCEDURE sp_interview_search(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_search VARCHAR(150),
  IN p_status_uuid CHAR(36), IN p_stage_uuid CHAR(36), IN p_type_uuid CHAR(36), IN p_client_uuid CHAR(36), IN p_date DATE,
  IN p_talent_uuid CHAR(36), IN p_page INT, IN p_page_size INT)
BEGIN
  DECLARE v_status BIGINT UNSIGNED;
  DECLARE v_stage BIGINT UNSIGNED;
  DECLARE v_type BIGINT UNSIGNED;
  DECLARE v_client BIGINT UNSIGNED;
  DECLARE v_talent BIGINT UNSIGNED;
  DECLARE v_offset INT;
  DECLARE v_search VARCHAR(160);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'interview.read');
  CALL sp_master_resolve(p_tenant_id, 'INTERVIEW_STATUS', p_status_uuid, v_status);
  CALL sp_master_resolve(p_tenant_id, 'INTERVIEW_STAGE', p_stage_uuid, v_stage);
  CALL sp_master_resolve(p_tenant_id, 'INTERVIEW_TYPE', p_type_uuid, v_type);
  CALL sp_master_resolve(p_tenant_id, 'CLIENT', p_client_uuid, v_client);
  IF p_talent_uuid IS NOT NULL AND p_talent_uuid <> '' THEN CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent); END IF;
  SET p_page_size = LEAST(GREATEST(IFNULL(p_page_size, 20), 1), 100);
  SET v_offset = (GREATEST(IFNULL(p_page, 1), 1) - 1) * p_page_size;
  SET v_search = IF(p_search IS NULL OR TRIM(p_search) = '', NULL, CONCAT('%', LOWER(TRIM(p_search)), '%'));

  SELECT COUNT(*) AS total
    FROM interview iv JOIN talent t ON t.id = iv.talent_id
   WHERE iv.tenant_id = p_tenant_id AND iv.deleted_at IS NULL
     AND (v_search IS NULL OR t.search_name LIKE v_search OR LOWER(iv.profile) LIKE v_search OR LOWER(iv.requirement_ref) LIKE v_search)
     AND (v_status IS NULL OR iv.status_item_id = v_status) AND (v_stage IS NULL OR iv.stage_item_id = v_stage)
     AND (v_type IS NULL OR iv.type_item_id = v_type) AND (v_client IS NULL OR iv.client_item_id = v_client)
     AND (p_date IS NULL OR iv.scheduled_date = p_date) AND (v_talent IS NULL OR iv.talent_id = v_talent);

  SELECT iv.uuid, t.uuid AS talent_uuid, t.first_name, t.last_name, t.second_last_name,
         ti.code AS type_code, ti.label_i18n AS type_label, st.code AS stage_code, st.label_i18n AS stage_label,
         ss.code AS status_code, ss.label_i18n AS status_label, cl.label_i18n AS client_label, iv.requirement_ref,
         iv.profile, iv.scheduled_date, iv.scheduled_time, iv.rating_general
    FROM interview iv
    JOIN talent t ON t.id = iv.talent_id
    JOIN master_item ti ON ti.id = iv.type_item_id
    JOIN master_item st ON st.id = iv.stage_item_id
    JOIN master_item ss ON ss.id = iv.status_item_id
    LEFT JOIN master_item cl ON cl.id = iv.client_item_id
   WHERE iv.tenant_id = p_tenant_id AND iv.deleted_at IS NULL
     AND (v_search IS NULL OR t.search_name LIKE v_search OR LOWER(iv.profile) LIKE v_search OR LOWER(iv.requirement_ref) LIKE v_search)
     AND (v_status IS NULL OR iv.status_item_id = v_status) AND (v_stage IS NULL OR iv.stage_item_id = v_stage)
     AND (v_type IS NULL OR iv.type_item_id = v_type) AND (v_client IS NULL OR iv.client_item_id = v_client)
     AND (p_date IS NULL OR iv.scheduled_date = p_date) AND (v_talent IS NULL OR iv.talent_id = v_talent)
   ORDER BY iv.scheduled_date DESC, iv.scheduled_time DESC, iv.id DESC
   LIMIT v_offset, p_page_size;
END $$

-- Archivos de la entrevista (binario cifrado en el volumen)
DROP PROCEDURE IF EXISTS sp_interview_file_create $$
CREATE PROCEDURE sp_interview_file_create(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_interview_uuid CHAR(36),
  IN p_uuid CHAR(36), IN p_type_uuid CHAR(36), IN p_storage_key VARCHAR(500), IN p_original_name VARCHAR(255),
  IN p_mime VARCHAR(120), IN p_size BIGINT UNSIGNED, IN p_sha256 CHAR(64), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_type BIGINT UNSIGNED;
  DECLARE v_code VARCHAR(60);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'interview.write');
  CALL sp_interview_resolve(p_tenant_id, p_interview_uuid, v_id);
  CALL sp_master_resolve_code(p_tenant_id, 'INTERVIEW_FILE_TYPE', p_type_uuid, v_type, v_code);
  IF v_type IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_KIND'; END IF;
  INSERT INTO file_object (uuid, tenant_id, owner_type, owner_id, category, storage_key, original_name, mime_type, size_bytes, sha256, is_encrypted, created_by)
  VALUES (p_uuid, p_tenant_id, 'INTERVIEW', v_id, v_code, p_storage_key, p_original_name, p_mime, p_size, p_sha256, 1, p_actor_id);
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'UPLOAD', 'interview_file', p_uuid, JSON_OBJECT('type', v_code), p_ip);
  SELECT uuid, category, original_name, mime_type, size_bytes, created_at FROM file_object WHERE uuid = p_uuid;
END $$

DROP PROCEDURE IF EXISTS sp_interview_file_get $$
CREATE PROCEDURE sp_interview_file_get(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'interview.read');
  SELECT f.id, f.uuid, f.tenant_id, f.owner_type, f.category, f.storage_key, f.original_name, f.mime_type, f.size_bytes, f.sha256, f.is_encrypted
    FROM file_object f JOIN interview iv ON iv.id = f.owner_id AND iv.tenant_id = p_tenant_id AND iv.deleted_at IS NULL
   WHERE f.uuid = p_uuid AND f.tenant_id = p_tenant_id AND f.owner_type = 'INTERVIEW' AND f.deleted_at IS NULL;
END $$

DROP PROCEDURE IF EXISTS sp_interview_file_delete $$
CREATE PROCEDURE sp_interview_file_delete(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'interview.write');
  UPDATE file_object SET deleted_at = NOW(3), deleted_by = p_actor_id
   WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND owner_type = 'INTERVIEW' AND deleted_at IS NULL;
  IF ROW_COUNT() > 0 THEN CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'DELETE', 'interview_file', p_uuid, NULL, p_ip); END IF;
END $$

DELIMITER ;
