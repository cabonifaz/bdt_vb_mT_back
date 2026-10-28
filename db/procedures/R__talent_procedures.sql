-- =====================================================================
-- Procedimientos del módulo de talentos (repetible)
-- Todas las escrituras usan uuid generado por el cliente => idempotentes
-- (la misma operación reenviada desde la cola offline no duplica datos).
-- =====================================================================

DELIMITER $$

-- ---------------------------------------------------------------------
-- Utilidades
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_talent_resolve $$
CREATE PROCEDURE sp_talent_resolve(IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), OUT o_id BIGINT UNSIGNED)
BEGIN
  SET o_id = NULL;
  SELECT id INTO o_id FROM talent WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
  IF o_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TALENT_NOT_FOUND'; END IF;
END $$

-- Valor de catálogo visible para el tenant (global o propio). uuid NULL => NULL.
DROP PROCEDURE IF EXISTS sp_master_resolve $$
CREATE PROCEDURE sp_master_resolve(IN p_tenant_id BIGINT UNSIGNED, IN p_table_code VARCHAR(60), IN p_uuid CHAR(36), OUT o_id BIGINT UNSIGNED)
BEGIN
  SET o_id = NULL;
  IF p_uuid IS NOT NULL AND p_uuid <> '' THEN
    SELECT i.id INTO o_id FROM master_item i JOIN master_table m ON m.id = i.master_table_id
     WHERE i.uuid = p_uuid AND m.code = p_table_code AND i.deleted_at IS NULL AND i.is_active = 1
       AND (i.tenant_id IS NULL OR i.tenant_id = p_tenant_id);
    IF o_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:MASTER_ITEM_INVALID'; END IF;
  END IF;
END $$

-- Cada cambio en la ficha incrementa la versión del talento
DROP PROCEDURE IF EXISTS sp_talent_touch $$
CREATE PROCEDURE sp_talent_touch(IN p_talent_id BIGINT UNSIGNED, IN p_actor_id BIGINT UNSIGNED)
BEGIN
  UPDATE talent SET version = version + 1, updated_by = p_actor_id WHERE id = p_talent_id;
END $$

-- ---------------------------------------------------------------------
-- Búsqueda
-- p_skills: uuids de habilidades técnicas separados por coma (debe tener TODAS)
-- p_english_level: código mínimo de inglés (BASIC|INTERMEDIATE|ADVANCED|NATIVE)
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_talent_search $$
CREATE PROCEDURE sp_talent_search(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_search VARCHAR(150), IN p_skills VARCHAR(2000), IN p_english_level VARCHAR(30), IN p_min_years DECIMAL(5,1),
  IN p_position VARCHAR(150), IN p_education VARCHAR(150), IN p_grade_uuid CHAR(36), IN p_collection_uuid CHAR(36),
  IN p_status VARCHAR(20), IN p_page INT, IN p_page_size INT)
BEGIN
  DECLARE v_offset INT;
  DECLARE v_skill_count INT DEFAULT 0;
  DECLARE v_en_lang BIGINT UNSIGNED;
  DECLARE v_en_min INT;
  DECLARE v_grade BIGINT UNSIGNED;
  DECLARE v_coll BIGINT UNSIGNED;
  DECLARE v_search VARCHAR(160);

  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  SET p_page_size = LEAST(GREATEST(IFNULL(p_page_size, 10), 1), 100);
  SET v_offset = (GREATEST(IFNULL(p_page, 1), 1) - 1) * p_page_size;
  SET v_search = IF(p_search IS NULL OR TRIM(p_search) = '', NULL, CONCAT('%', LOWER(TRIM(p_search)), '%'));
  IF p_skills IS NOT NULL AND p_skills <> '' THEN
    SET v_skill_count = LENGTH(p_skills) - LENGTH(REPLACE(p_skills, ',', '')) + 1;
  END IF;
  IF p_english_level IS NOT NULL AND p_english_level <> '' THEN
    SELECT i.id INTO v_en_lang FROM master_item i JOIN master_table m ON m.id = i.master_table_id
     WHERE m.code = 'LANGUAGE' AND i.code = 'EN' AND i.tenant_id IS NULL;
    SELECT i.sort_order INTO v_en_min FROM master_item i JOIN master_table m ON m.id = i.master_table_id
     WHERE m.code = 'LANGUAGE_LEVEL' AND i.code = p_english_level AND i.tenant_id IS NULL;
  END IF;
  IF p_grade_uuid IS NOT NULL AND p_grade_uuid <> '' THEN CALL sp_master_resolve(p_tenant_id, 'ACADEMIC_GRADE', p_grade_uuid, v_grade); END IF;
  IF p_collection_uuid IS NOT NULL AND p_collection_uuid <> '' THEN
    SELECT fc.id INTO v_coll FROM favorite_collection fc
     WHERE fc.uuid = p_collection_uuid AND fc.tenant_id = p_tenant_id AND fc.deleted_at IS NULL AND (fc.user_id = p_actor_id OR EXISTS (SELECT 1 FROM favorite_collection_share sh WHERE sh.collection_id = fc.id AND sh.user_id = p_actor_id));
    IF v_coll IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  END IF;

  DROP TEMPORARY TABLE IF EXISTS tmp_talent_hit;
  CREATE TEMPORARY TABLE tmp_talent_hit (id BIGINT UNSIGNED PRIMARY KEY, years DECIMAL(6,1), sort_ts DATETIME(3)) ENGINE=MEMORY;

  INSERT INTO tmp_talent_hit (id, years, sort_ts)
  SELECT t.id,
         IFNULL((SELECT ROUND(SUM(TIMESTAMPDIFF(MONTH, e.start_date, IFNULL(e.end_date, CURDATE()))) / 12, 1)
                   FROM talent_experience e WHERE e.talent_id = t.id AND e.deleted_at IS NULL), 0),
         IFNULL(t.updated_at, t.created_at)
    FROM talent t
   WHERE t.tenant_id = p_tenant_id AND t.deleted_at IS NULL
     AND t.status = IFNULL(NULLIF(p_status, ''), 'ACTIVE')
     AND (v_search IS NULL OR t.search_name LIKE v_search OR LOWER(t.headline) LIKE v_search)
     AND (v_skill_count = 0 OR (SELECT COUNT(DISTINCT s.skill_item_id) FROM talent_tech_skill s
                                  JOIN master_item mi ON mi.id = s.skill_item_id
                                 WHERE s.talent_id = t.id AND s.deleted_at IS NULL AND FIND_IN_SET(mi.uuid, p_skills) > 0) = v_skill_count)
     AND (v_en_min IS NULL OR EXISTS (SELECT 1 FROM talent_language l JOIN master_item lv ON lv.id = l.level_item_id
                                      WHERE l.talent_id = t.id AND l.deleted_at IS NULL AND l.language_item_id = v_en_lang
                                        AND lv.sort_order >= v_en_min))
     AND (p_position IS NULL OR p_position = '' OR LOWER(t.headline) LIKE CONCAT('%', LOWER(p_position), '%')
          OR EXISTS (SELECT 1 FROM talent_experience e WHERE e.talent_id = t.id AND e.deleted_at IS NULL
                       AND LOWER(e.position) LIKE CONCAT('%', LOWER(p_position), '%')))
     AND (p_education IS NULL OR p_education = '' OR EXISTS (SELECT 1 FROM talent_education d WHERE d.talent_id = t.id AND d.deleted_at IS NULL
                       AND (LOWER(d.career) LIKE CONCAT('%', LOWER(p_education), '%') OR LOWER(d.institution) LIKE CONCAT('%', LOWER(p_education), '%'))))
     AND (v_grade IS NULL OR EXISTS (SELECT 1 FROM talent_education d WHERE d.talent_id = t.id AND d.deleted_at IS NULL AND d.grade_item_id = v_grade))
     AND (v_coll IS NULL OR EXISTS (SELECT 1 FROM favorite_collection_talent c WHERE c.collection_id = v_coll AND c.talent_id = t.id));

  IF p_min_years IS NOT NULL AND p_min_years > 0 THEN
    DELETE FROM tmp_talent_hit WHERE years < p_min_years;
  END IF;

  SELECT COUNT(*) AS total FROM tmp_talent_hit;

  SELECT t.uuid, t.first_name, t.last_name, t.second_last_name, t.headline, c.label_i18n AS country, t.city,
         t.avg_rating, pf.uuid AS photo_uuid, h.years AS years_experience,
         EXISTS (SELECT 1 FROM favorite_collection_talent ct JOIN favorite_collection fc ON fc.id = ct.collection_id
                  WHERE ct.talent_id = t.id AND fc.deleted_at IS NULL AND (fc.user_id = p_actor_id OR EXISTS (SELECT 1 FROM favorite_collection_share sh WHERE sh.collection_id = fc.id AND sh.user_id = p_actor_id))) AS is_favorite,
         (SELECT JSON_ARRAYAGG(JSON_OBJECT('id', mi.uuid, 'label', mi.label_i18n, 'years', s.years))
            FROM talent_tech_skill s JOIN master_item mi ON mi.id = s.skill_item_id
           WHERE s.talent_id = t.id AND s.deleted_at IS NULL) AS skills,
         (SELECT lv.code FROM talent_language l JOIN master_item lv ON lv.id = l.level_item_id
           WHERE l.talent_id = t.id AND l.deleted_at IS NULL AND l.language_item_id = (
             SELECT i.id FROM master_item i JOIN master_table m ON m.id = i.master_table_id
              WHERE m.code = 'LANGUAGE' AND i.code = 'EN' AND i.tenant_id IS NULL) LIMIT 1) AS english_level,
         t.version, IFNULL(t.updated_at, t.created_at) AS updated_at,
         EXISTS (SELECT 1 FROM talent_blacklist b WHERE b.talent_id = t.id AND b.deleted_at IS NULL) AS is_blacklisted
    FROM tmp_talent_hit h
    JOIN talent t ON t.id = h.id
    LEFT JOIN master_item c ON c.id = t.country_item_id
    LEFT JOIN file_object pf ON pf.id = t.photo_file_id AND pf.deleted_at IS NULL
   ORDER BY h.sort_ts DESC, t.id DESC
   LIMIT v_offset, p_page_size;

  DROP TEMPORARY TABLE IF EXISTS tmp_talent_hit;
END $$

-- ---------------------------------------------------------------------
-- Ficha completa (9 result sets). Las tarifas solo con talent.rates.read.
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_talent_get $$
CREATE PROCEDURE sp_talent_get(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_rates TINYINT(1);
  DECLARE v_is_admin TINYINT(1);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  CALL sp_talent_resolve(p_tenant_id, p_uuid, v_id);
  SET v_rates = EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'talent.rates.read');
  SET v_is_admin = EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'tenant.users.manage');

  SELECT t.uuid, t.first_name, t.last_name, t.second_last_name, t.headline,
         dt.uuid AS doc_type_uuid, t.doc_number_enc, t.email_enc, t.phone_enc, t.birth_date_enc,
         c.uuid AS country_uuid, c.label_i18n AS country, t.city, oc.uuid AS origin_country_uuid, oc.label_i18n AS origin_country, t.origin_city,
         t.linkedin_url, t.github_url, t.summary,
         av.uuid AS availability_uuid, t.has_equipment, IF(v_rates = 1, t.rates_enc, NULL) AS rates_enc, v_rates AS can_see_rates,
         pf.uuid AS photo_uuid, cf.uuid AS cv_uuid, t.avg_rating, t.status, t.source, t.version,
         t.created_at, IFNULL(t.updated_at, t.created_at) AS updated_at
    FROM talent t
    LEFT JOIN master_item dt ON dt.id = t.doc_type_item_id
    LEFT JOIN master_item c ON c.id = t.country_item_id
    LEFT JOIN master_item av ON av.id = t.availability_item_id
    LEFT JOIN master_item oc ON oc.id = t.origin_country_item_id
    LEFT JOIN file_object pf ON pf.id = t.photo_file_id AND pf.deleted_at IS NULL
    LEFT JOIN file_object cf ON cf.id = t.cv_file_id AND cf.deleted_at IS NULL
   WHERE t.id = v_id;

  SELECT s.uuid, mi.uuid AS skill_uuid, mi.label_i18n, s.years
    FROM talent_tech_skill s JOIN master_item mi ON mi.id = s.skill_item_id
   WHERE s.talent_id = v_id AND s.deleted_at IS NULL ORDER BY s.years DESC, s.id;

  SELECT s.uuid, mi.uuid AS skill_uuid, mi.label_i18n
    FROM talent_soft_skill s JOIN master_item mi ON mi.id = s.skill_item_id
   WHERE s.talent_id = v_id AND s.deleted_at IS NULL ORDER BY s.id;

  SELECT e.uuid, e.company, e.position, e.start_date, e.end_date, e.is_current, e.functions,
         TIMESTAMPDIFF(MONTH, e.start_date, IFNULL(e.end_date, CURDATE())) AS months
    FROM talent_experience e WHERE e.talent_id = v_id AND e.deleted_at IS NULL
   ORDER BY e.is_current DESC, e.start_date DESC;

  SELECT d.uuid, d.institution, d.career, g.uuid AS grade_uuid, g.label_i18n AS grade, d.start_date, d.end_date,
         d.is_current, d.date_precision
    FROM talent_education d LEFT JOIN master_item g ON g.id = d.grade_item_id
   WHERE d.talent_id = v_id AND d.deleted_at IS NULL ORDER BY d.is_current DESC, d.start_date DESC;

  SELECT l.uuid, lg.uuid AS language_uuid, lg.label_i18n AS language, lv.uuid AS level_uuid, lv.label_i18n AS level,
         lv.code AS level_code, l.stars
    FROM talent_language l JOIN master_item lg ON lg.id = l.language_item_id JOIN master_item lv ON lv.id = l.level_item_id
   WHERE l.talent_id = v_id AND l.deleted_at IS NULL ORDER BY lg.sort_order;

  SELECT f.uuid, f.stars, f.comment, u.uuid AS author_uuid, u.full_name_enc AS author_name_enc, f.created_at,
         IF(f.author_user_id = p_actor_id, 1, 0) AS is_mine, IF(f.author_user_id = p_actor_id OR v_is_admin = 1, 1, 0) AS can_delete
    FROM talent_feedback f JOIN app_user u ON u.id = f.author_user_id
   WHERE f.talent_id = v_id AND f.deleted_at IS NULL ORDER BY f.created_at DESC;

  SELECT fo.uuid, fo.category, fo.original_name, fo.mime_type, fo.size_bytes, fo.created_at
    FROM file_object fo
   WHERE fo.tenant_id = p_tenant_id AND fo.owner_type = 'TALENT' AND fo.owner_id = v_id AND fo.deleted_at IS NULL
   ORDER BY fo.created_at DESC;

  SELECT fc.uuid FROM favorite_collection_talent ct JOIN favorite_collection fc ON fc.id = ct.collection_id
   WHERE ct.talent_id = v_id AND fc.deleted_at IS NULL AND (fc.user_id = p_actor_id OR EXISTS (SELECT 1 FROM favorite_collection_share sh WHERE sh.collection_id = fc.id AND sh.user_id = p_actor_id));
END $$

-- ---------------------------------------------------------------------
-- Alta / edición de datos principales
-- p_set_rates = 1 solo si el cliente envía tarifas (requiere talent.rates.read)
-- p_expected_version NULL = sin control de versión
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_talent_upsert $$
CREATE PROCEDURE sp_talent_upsert(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_first_name VARCHAR(100), IN p_last_name VARCHAR(100), IN p_second_last_name VARCHAR(100), IN p_headline VARCHAR(150),
  IN p_doc_type_uuid CHAR(36), IN p_doc_enc VARBINARY(256), IN p_doc_bidx CHAR(64),
  IN p_email_enc VARBINARY(512), IN p_email_bidx CHAR(64), IN p_phone_enc VARBINARY(256), IN p_birth_enc VARBINARY(256),
  IN p_country_uuid CHAR(36), IN p_city VARCHAR(120), IN p_origin_country_uuid CHAR(36), IN p_origin_city VARCHAR(120), IN p_linkedin VARCHAR(300), IN p_github VARCHAR(300),
  IN p_summary TEXT, IN p_availability_uuid CHAR(36), IN p_has_equipment TINYINT(1),
  IN p_rates_enc VARBINARY(2048), IN p_set_rates TINYINT(1), IN p_expected_version INT, IN p_source VARCHAR(20), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_version INT;
  DECLARE v_doc_type BIGINT UNSIGNED;
  DECLARE v_country BIGINT UNSIGNED;
  DECLARE v_availability BIGINT UNSIGNED;
  DECLARE v_origin BIGINT UNSIGNED;
  DECLARE v_created TINYINT(1) DEFAULT 0;

  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  IF p_set_rates = 1 AND NOT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'talent.rates.read') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FORBIDDEN';
  END IF;
  IF TRIM(IFNULL(p_first_name, '')) = '' OR TRIM(IFNULL(p_last_name, '')) = '' THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TALENT_NAME_REQUIRED';
  END IF;
  IF p_linkedin IS NOT NULL AND p_linkedin <> '' AND p_linkedin NOT REGEXP '^https://([a-z]{2,3}\\.)?linkedin\\.com/' THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_LINKEDIN';
  END IF;
  IF p_github IS NOT NULL AND p_github <> '' AND p_github NOT REGEXP '^https://(www\\.)?github\\.com/' THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_GITHUB';
  END IF;
  CALL sp_master_resolve(p_tenant_id, 'DOC_IDENTITY_TYPE', p_doc_type_uuid, v_doc_type);
  CALL sp_master_resolve(p_tenant_id, 'COUNTRY', p_country_uuid, v_country);
  CALL sp_master_resolve(p_tenant_id, 'TALENT_AVAILABILITY', p_availability_uuid, v_availability);
  CALL sp_master_resolve(p_tenant_id, 'COUNTRY', p_origin_country_uuid, v_origin);

  SELECT id, version INTO v_id, v_version FROM talent WHERE uuid = p_uuid AND deleted_at IS NULL;
  IF v_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM talent WHERE id = v_id AND tenant_id = p_tenant_id) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TENANT_MISMATCH';
  END IF;
  IF v_id IS NOT NULL AND p_expected_version IS NOT NULL AND p_expected_version <> v_version THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VERSION_CONFLICT';
  END IF;
  IF p_email_bidx IS NOT NULL AND EXISTS (SELECT 1 FROM talent WHERE tenant_id = p_tenant_id AND email_bidx = p_email_bidx
                                           AND deleted_at IS NULL AND (v_id IS NULL OR id <> v_id)) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TALENT_EMAIL_EXISTS';
  END IF;
  IF p_doc_bidx IS NOT NULL AND EXISTS (SELECT 1 FROM talent WHERE tenant_id = p_tenant_id AND doc_number_bidx = p_doc_bidx
                                         AND deleted_at IS NULL AND (v_id IS NULL OR id <> v_id)) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TALENT_DOC_EXISTS';
  END IF;

  IF v_id IS NULL THEN
    INSERT INTO talent (uuid, tenant_id, first_name, last_name, second_last_name, headline, doc_type_item_id, doc_number_enc,
                        doc_number_bidx, email_enc, email_bidx, phone_enc, birth_date_enc, country_item_id, city, origin_country_item_id, origin_city, linkedin_url, github_url,
                        summary, availability_item_id, has_equipment, rates_enc, source, created_by)
    VALUES (p_uuid, p_tenant_id, TRIM(p_first_name), TRIM(p_last_name), NULLIF(TRIM(p_second_last_name), ''), NULLIF(TRIM(p_headline), ''),
            v_doc_type, p_doc_enc, p_doc_bidx, p_email_enc, p_email_bidx, p_phone_enc, p_birth_enc, v_country, NULLIF(TRIM(p_city), ''), v_origin, NULLIF(TRIM(p_origin_city), ''),
            NULLIF(p_linkedin, ''), NULLIF(p_github, ''), p_summary, v_availability, p_has_equipment,
            IF(p_set_rates = 1, p_rates_enc, NULL), IFNULL(NULLIF(p_source, ''), 'MANUAL'), p_actor_id);
    SET v_id = LAST_INSERT_ID();
    SET v_created = 1;
  ELSE
    UPDATE talent
       SET first_name = TRIM(p_first_name), last_name = TRIM(p_last_name), second_last_name = NULLIF(TRIM(p_second_last_name), ''),
           headline = NULLIF(TRIM(p_headline), ''), doc_type_item_id = v_doc_type, doc_number_enc = p_doc_enc,
           doc_number_bidx = p_doc_bidx, email_enc = p_email_enc, email_bidx = p_email_bidx, phone_enc = p_phone_enc, birth_date_enc = p_birth_enc,
           country_item_id = v_country, city = NULLIF(TRIM(p_city), ''),
           origin_country_item_id = v_origin, origin_city = NULLIF(TRIM(p_origin_city), ''), linkedin_url = NULLIF(p_linkedin, ''),
           github_url = NULLIF(p_github, ''), summary = p_summary, availability_item_id = v_availability,
           has_equipment = p_has_equipment, rates_enc = IF(p_set_rates = 1, p_rates_enc, rates_enc),
           version = version + 1, updated_by = p_actor_id
     WHERE id = v_id;
  END IF;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, IF(v_created = 1, 'CREATE', 'UPDATE'), 'talent', p_uuid, NULL, p_ip);
  SELECT uuid, version, v_created AS created FROM talent WHERE id = v_id;
END $$

DROP PROCEDURE IF EXISTS sp_talent_set_status $$
CREATE PROCEDURE sp_talent_set_status(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_status VARCHAR(20), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  CALL sp_talent_resolve(p_tenant_id, p_uuid, v_id);
  IF p_status NOT IN ('ACTIVE','ARCHIVED') THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_STATUS'; END IF;
  UPDATE talent SET status = p_status, version = version + 1, updated_by = p_actor_id WHERE id = v_id;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'SET_STATUS', 'talent', p_uuid, JSON_OBJECT('status', p_status), p_ip);
END $$

-- Edición rápida desde la ficha: disponibilidad y equipo propio (sin control de versión: un solo campo)
DROP PROCEDURE IF EXISTS sp_talent_set_availability $$
CREATE PROCEDURE sp_talent_set_availability(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_availability_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_availability BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  CALL sp_talent_resolve(p_tenant_id, p_uuid, v_id);
  CALL sp_master_resolve(p_tenant_id, 'TALENT_AVAILABILITY', p_availability_uuid, v_availability);
  UPDATE talent SET availability_item_id = v_availability, version = version + 1, updated_by = p_actor_id WHERE id = v_id;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'SET_AVAILABILITY', 'talent', p_uuid, JSON_OBJECT('availability', p_availability_uuid), p_ip);
END $$

DROP PROCEDURE IF EXISTS sp_talent_set_equipment $$
CREATE PROCEDURE sp_talent_set_equipment(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_has_equipment TINYINT(1), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  CALL sp_talent_resolve(p_tenant_id, p_uuid, v_id);
  UPDATE talent SET has_equipment = p_has_equipment, version = version + 1, updated_by = p_actor_id WHERE id = v_id;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'SET_EQUIPMENT', 'talent', p_uuid, JSON_OBJECT('hasEquipment', p_has_equipment), p_ip);
END $$

-- ---------------------------------------------------------------------
-- Habilidades
-- ---------------------------------------------------------------------
-- Busca la habilidad por nombre (global o del tenant); si no existe la crea en el tenant.
DROP PROCEDURE IF EXISTS sp_skill_ensure $$
CREATE PROCEDURE sp_skill_ensure(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_table_code VARCHAR(60),
  IN p_uuid CHAR(36), IN p_label VARCHAR(100))
BEGIN
  DECLARE v_table BIGINT UNSIGNED;
  DECLARE v_uuid CHAR(36);
  DECLARE v_code VARCHAR(60);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  IF p_table_code NOT IN ('TECH_SKILL','SOFT_SKILL') THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:MASTER_SCOPE'; END IF;
  SET p_label = TRIM(p_label);
  IF p_label = '' OR CHAR_LENGTH(p_label) > 100 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  SELECT id INTO v_table FROM master_table WHERE code = p_table_code;
  SELECT i.uuid INTO v_uuid FROM master_item i
   WHERE i.master_table_id = v_table AND i.deleted_at IS NULL AND (i.tenant_id IS NULL OR i.tenant_id = p_tenant_id)
     AND LOWER(JSON_UNQUOTE(JSON_EXTRACT(i.label_i18n, '$.es'))) = LOWER(p_label)
   ORDER BY i.tenant_id IS NULL DESC LIMIT 1;
  IF v_uuid IS NULL THEN
    SET v_code = LEFT(UPPER(TRIM(BOTH '_' FROM REGEXP_REPLACE(p_label, '[^A-Za-z0-9]+', '_'))), 50);
    IF v_code = '' THEN SET v_code = 'SKILL'; END IF;
    IF EXISTS (SELECT 1 FROM master_item WHERE master_table_id = v_table AND tenant_key IN (0, p_tenant_id) AND code = v_code) THEN
      SET v_code = CONCAT(LEFT(v_code, 41), '_', LEFT(REPLACE(p_uuid, '-', ''), 8));
    END IF;
    INSERT INTO master_item (uuid, master_table_id, tenant_id, code, label_i18n, created_by)
    VALUES (p_uuid, v_table, p_tenant_id, v_code, JSON_OBJECT('es', p_label, 'en', p_label), p_actor_id);
    SET v_uuid = p_uuid;
    CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'CREATE', 'master_item', p_uuid, JSON_OBJECT('table', p_table_code, 'label', p_label), NULL);
  END IF;
  SELECT i.uuid, i.label_i18n FROM master_item i WHERE i.uuid = v_uuid;
END $$

DROP PROCEDURE IF EXISTS sp_talent_tech_skill_upsert $$
CREATE PROCEDURE sp_talent_tech_skill_upsert(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_talent_uuid CHAR(36), IN p_uuid CHAR(36), IN p_skill_uuid CHAR(36), IN p_years DECIMAL(4,1))
BEGIN
  DECLARE v_talent BIGINT UNSIGNED;
  DECLARE v_skill BIGINT UNSIGNED;
  DECLARE v_existing BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent);
  CALL sp_master_resolve(p_tenant_id, 'TECH_SKILL', p_skill_uuid, v_skill);
  IF v_skill IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  IF p_years IS NULL OR p_years < 0 OR p_years > 60 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_YEARS'; END IF;
  -- misma habilidad ya registrada: se actualizan los años (no se duplica)
  SELECT id INTO v_existing FROM talent_tech_skill
   WHERE talent_id = v_talent AND deleted_at IS NULL AND (uuid = p_uuid OR skill_item_id = v_skill) LIMIT 1;
  IF v_existing IS NULL THEN
    INSERT INTO talent_tech_skill (uuid, tenant_id, talent_id, skill_item_id, years, created_by)
    VALUES (p_uuid, p_tenant_id, v_talent, v_skill, p_years, p_actor_id);
  ELSE
    UPDATE talent_tech_skill SET skill_item_id = v_skill, years = p_years, updated_by = p_actor_id WHERE id = v_existing;
  END IF;
  CALL sp_talent_touch(v_talent, p_actor_id);
END $$

DROP PROCEDURE IF EXISTS sp_talent_soft_skill_add $$
CREATE PROCEDURE sp_talent_soft_skill_add(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_talent_uuid CHAR(36), IN p_uuid CHAR(36), IN p_skill_uuid CHAR(36))
BEGIN
  DECLARE v_talent BIGINT UNSIGNED;
  DECLARE v_skill BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent);
  CALL sp_master_resolve(p_tenant_id, 'SOFT_SKILL', p_skill_uuid, v_skill);
  IF v_skill IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  IF NOT EXISTS (SELECT 1 FROM talent_soft_skill WHERE talent_id = v_talent AND deleted_at IS NULL AND (uuid = p_uuid OR skill_item_id = v_skill)) THEN
    INSERT INTO talent_soft_skill (uuid, tenant_id, talent_id, skill_item_id, created_by)
    VALUES (p_uuid, p_tenant_id, v_talent, v_skill, p_actor_id);
    CALL sp_talent_touch(v_talent, p_actor_id);
  END IF;
END $$

-- Borrado lógico genérico de una sección de la ficha (tech/soft/experience/education/language)
DROP PROCEDURE IF EXISTS sp_talent_section_delete $$
CREATE PROCEDURE sp_talent_section_delete(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_section VARCHAR(20), IN p_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_talent BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  CASE p_section
    WHEN 'TECH_SKILL' THEN
      SELECT talent_id INTO v_talent FROM talent_tech_skill WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
      UPDATE talent_tech_skill SET deleted_at = NOW(3), deleted_by = p_actor_id WHERE uuid = p_uuid AND tenant_id = p_tenant_id;
    WHEN 'SOFT_SKILL' THEN
      SELECT talent_id INTO v_talent FROM talent_soft_skill WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
      UPDATE talent_soft_skill SET deleted_at = NOW(3), deleted_by = p_actor_id WHERE uuid = p_uuid AND tenant_id = p_tenant_id;
    WHEN 'EXPERIENCE' THEN
      SELECT talent_id INTO v_talent FROM talent_experience WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
      UPDATE talent_experience SET deleted_at = NOW(3), deleted_by = p_actor_id WHERE uuid = p_uuid AND tenant_id = p_tenant_id;
    WHEN 'EDUCATION' THEN
      SELECT talent_id INTO v_talent FROM talent_education WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
      UPDATE talent_education SET deleted_at = NOW(3), deleted_by = p_actor_id WHERE uuid = p_uuid AND tenant_id = p_tenant_id;
    WHEN 'LANGUAGE' THEN
      SELECT talent_id INTO v_talent FROM talent_language WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
      UPDATE talent_language SET deleted_at = NOW(3), deleted_by = p_actor_id WHERE uuid = p_uuid AND tenant_id = p_tenant_id;
    ELSE SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION';
  END CASE;
  -- idempotente: borrar algo ya borrado no es error
  IF v_talent IS NOT NULL THEN
    CALL sp_talent_touch(v_talent, p_actor_id);
    CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'DELETE', CONCAT('talent_', LOWER(p_section)), p_uuid, NULL, p_ip);
  END IF;
END $$

-- ---------------------------------------------------------------------
-- Experiencia, educación, idiomas
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_talent_experience_upsert $$
CREATE PROCEDURE sp_talent_experience_upsert(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_talent_uuid CHAR(36), IN p_uuid CHAR(36), IN p_company VARCHAR(150), IN p_position VARCHAR(150),
  IN p_start DATE, IN p_end DATE, IN p_is_current TINYINT(1), IN p_functions TEXT)
BEGIN
  DECLARE v_talent BIGINT UNSIGNED;
  DECLARE v_id BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent);
  IF TRIM(IFNULL(p_company,'')) = '' OR TRIM(IFNULL(p_position,'')) = '' OR p_start IS NULL THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION';
  END IF;
  IF p_is_current = 1 THEN SET p_end = NULL; END IF;
  IF p_is_current = 0 AND p_end IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:END_DATE_REQUIRED'; END IF;
  IF p_end IS NOT NULL AND p_end < p_start THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_DATES'; END IF;
  IF p_start > CURDATE() THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_DATES'; END IF;
  SELECT id INTO v_id FROM talent_experience WHERE uuid = p_uuid AND talent_id = v_talent AND deleted_at IS NULL;
  IF v_id IS NULL THEN
    INSERT INTO talent_experience (uuid, tenant_id, talent_id, company, position, start_date, end_date, is_current, functions, created_by)
    VALUES (p_uuid, p_tenant_id, v_talent, TRIM(p_company), TRIM(p_position), p_start, p_end, p_is_current, p_functions, p_actor_id);
  ELSE
    UPDATE talent_experience SET company = TRIM(p_company), position = TRIM(p_position), start_date = p_start, end_date = p_end,
           is_current = p_is_current, functions = p_functions, updated_by = p_actor_id WHERE id = v_id;
  END IF;
  CALL sp_talent_touch(v_talent, p_actor_id);
END $$

DROP PROCEDURE IF EXISTS sp_talent_education_upsert $$
CREATE PROCEDURE sp_talent_education_upsert(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_talent_uuid CHAR(36), IN p_uuid CHAR(36), IN p_institution VARCHAR(150), IN p_career VARCHAR(150),
  IN p_grade_uuid CHAR(36), IN p_start DATE, IN p_end DATE, IN p_is_current TINYINT(1), IN p_precision VARCHAR(5))
BEGIN
  DECLARE v_talent BIGINT UNSIGNED;
  DECLARE v_grade BIGINT UNSIGNED;
  DECLARE v_id BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent);
  CALL sp_master_resolve(p_tenant_id, 'ACADEMIC_GRADE', p_grade_uuid, v_grade);
  IF TRIM(IFNULL(p_institution,'')) = '' OR TRIM(IFNULL(p_career,'')) = '' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  IF p_is_current = 1 THEN SET p_end = NULL; END IF;
  IF p_start IS NOT NULL AND p_end IS NOT NULL AND p_end < p_start THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_DATES'; END IF;
  SET p_precision = IF(p_precision = 'YEAR', 'YEAR', 'MONTH');
  SELECT id INTO v_id FROM talent_education WHERE uuid = p_uuid AND talent_id = v_talent AND deleted_at IS NULL;
  IF v_id IS NULL THEN
    INSERT INTO talent_education (uuid, tenant_id, talent_id, institution, career, grade_item_id, start_date, end_date, is_current, date_precision, created_by)
    VALUES (p_uuid, p_tenant_id, v_talent, TRIM(p_institution), TRIM(p_career), v_grade, p_start, p_end, p_is_current, p_precision, p_actor_id);
  ELSE
    UPDATE talent_education SET institution = TRIM(p_institution), career = TRIM(p_career), grade_item_id = v_grade, start_date = p_start,
           end_date = p_end, is_current = p_is_current, date_precision = p_precision, updated_by = p_actor_id WHERE id = v_id;
  END IF;
  CALL sp_talent_touch(v_talent, p_actor_id);
END $$

DROP PROCEDURE IF EXISTS sp_talent_language_upsert $$
CREATE PROCEDURE sp_talent_language_upsert(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_talent_uuid CHAR(36), IN p_uuid CHAR(36), IN p_language_uuid CHAR(36), IN p_level_uuid CHAR(36), IN p_stars TINYINT)
BEGIN
  DECLARE v_talent BIGINT UNSIGNED;
  DECLARE v_lang BIGINT UNSIGNED;
  DECLARE v_level BIGINT UNSIGNED;
  DECLARE v_existing BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent);
  CALL sp_master_resolve(p_tenant_id, 'LANGUAGE', p_language_uuid, v_lang);
  CALL sp_master_resolve(p_tenant_id, 'LANGUAGE_LEVEL', p_level_uuid, v_level);
  IF v_lang IS NULL OR v_level IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  IF p_stars IS NULL OR p_stars < 0 OR p_stars > 5 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_STARS'; END IF;
  SELECT id INTO v_existing FROM talent_language
   WHERE talent_id = v_talent AND deleted_at IS NULL AND (uuid = p_uuid OR language_item_id = v_lang) LIMIT 1;
  IF v_existing IS NULL THEN
    INSERT INTO talent_language (uuid, tenant_id, talent_id, language_item_id, level_item_id, stars, created_by)
    VALUES (p_uuid, p_tenant_id, v_talent, v_lang, v_level, p_stars, p_actor_id);
  ELSE
    UPDATE talent_language SET language_item_id = v_lang, level_item_id = v_level, stars = p_stars, updated_by = p_actor_id WHERE id = v_existing;
  END IF;
  CALL sp_talent_touch(v_talent, p_actor_id);
END $$

-- ---------------------------------------------------------------------
-- Feedback (solo el autor edita; autor o admin elimina) y promedio de estrellas
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_talent_rating_refresh $$
CREATE PROCEDURE sp_talent_rating_refresh(IN p_talent_id BIGINT UNSIGNED)
BEGIN
  UPDATE talent SET avg_rating = (SELECT ROUND(AVG(stars), 2) FROM talent_feedback WHERE talent_id = p_talent_id AND deleted_at IS NULL)
   WHERE id = p_talent_id;
END $$

DROP PROCEDURE IF EXISTS sp_talent_feedback_upsert $$
CREATE PROCEDURE sp_talent_feedback_upsert(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_talent_uuid CHAR(36), IN p_uuid CHAR(36), IN p_stars TINYINT, IN p_comment TEXT)
BEGIN
  DECLARE v_talent BIGINT UNSIGNED;
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_author BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent);
  IF p_stars IS NULL OR p_stars < 1 OR p_stars > 5 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_STARS'; END IF;
  IF TRIM(IFNULL(p_comment, '')) = '' OR CHAR_LENGTH(p_comment) > 5000 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  SELECT id, author_user_id INTO v_id, v_author FROM talent_feedback WHERE uuid = p_uuid AND talent_id = v_talent AND deleted_at IS NULL;
  IF v_id IS NULL THEN
    INSERT INTO talent_feedback (uuid, tenant_id, talent_id, author_user_id, stars, comment, created_by)
    VALUES (p_uuid, p_tenant_id, v_talent, p_actor_id, p_stars, TRIM(p_comment), p_actor_id);
  ELSE
    IF v_author <> p_actor_id THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FORBIDDEN'; END IF;
    UPDATE talent_feedback SET stars = p_stars, comment = TRIM(p_comment), updated_by = p_actor_id WHERE id = v_id;
  END IF;
  CALL sp_talent_rating_refresh(v_talent);
  CALL sp_talent_touch(v_talent, p_actor_id);
  SELECT avg_rating FROM talent WHERE id = v_talent;
END $$

DROP PROCEDURE IF EXISTS sp_talent_feedback_delete $$
CREATE PROCEDURE sp_talent_feedback_delete(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_talent BIGINT UNSIGNED;
  DECLARE v_author BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  SELECT talent_id, author_user_id INTO v_talent, v_author FROM talent_feedback WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
  IF v_talent IS NOT NULL THEN
    IF v_author <> p_actor_id AND NOT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'tenant.users.manage') THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FORBIDDEN';
    END IF;
    UPDATE talent_feedback SET deleted_at = NOW(3), deleted_by = p_actor_id WHERE uuid = p_uuid;
    CALL sp_talent_rating_refresh(v_talent);
    CALL sp_talent_touch(v_talent, p_actor_id);
    CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'DELETE', 'talent_feedback', p_uuid, NULL, p_ip);
  END IF;
END $$

-- ---------------------------------------------------------------------
-- Archivos del talento (CV, foto, certificados). El binario está en el volumen.
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_talent_file_create $$
CREATE PROCEDURE sp_talent_file_create(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_talent_uuid CHAR(36),
  IN p_uuid CHAR(36), IN p_category VARCHAR(40), IN p_storage_key VARCHAR(500), IN p_original_name VARCHAR(255),
  IN p_mime VARCHAR(120), IN p_size BIGINT UNSIGNED, IN p_sha256 CHAR(64), IN p_encrypted TINYINT(1), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_talent BIGINT UNSIGNED;
  DECLARE v_file BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent);
  IF p_category NOT IN ('PHOTO','CV','CV_TENANT_ES','CV_TENANT_EN','CERTIFICATE','OTHER') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_KIND';
  END IF;
  INSERT INTO file_object (uuid, tenant_id, owner_type, owner_id, category, storage_key, original_name, mime_type,
                           size_bytes, sha256, is_encrypted, created_by)
  VALUES (p_uuid, p_tenant_id, 'TALENT', v_talent, p_category, p_storage_key, p_original_name, p_mime, p_size, p_sha256, p_encrypted, p_actor_id);
  SET v_file = LAST_INSERT_ID();
  -- La foto y el CV principal son únicos: el anterior queda reemplazado
  IF p_category = 'PHOTO' THEN
    UPDATE file_object SET deleted_at = NOW(3), deleted_by = p_actor_id
     WHERE owner_type = 'TALENT' AND owner_id = v_talent AND category = 'PHOTO' AND id <> v_file AND deleted_at IS NULL;
    UPDATE talent SET photo_file_id = v_file WHERE id = v_talent;
  ELSEIF p_category = 'CV' THEN
    UPDATE talent SET cv_file_id = v_file WHERE id = v_talent;
  END IF;
  CALL sp_talent_touch(v_talent, p_actor_id);
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'UPLOAD', 'talent_file', p_uuid, JSON_OBJECT('category', p_category), p_ip);
  SELECT uuid, category, original_name, mime_type, size_bytes, created_at FROM file_object WHERE id = v_file;
END $$

DROP PROCEDURE IF EXISTS sp_talent_file_get $$
CREATE PROCEDURE sp_talent_file_get(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  SELECT f.id, f.uuid, f.tenant_id, f.owner_type, f.category, f.storage_key, f.original_name, f.mime_type, f.size_bytes, f.sha256, f.is_encrypted
    FROM file_object f
    JOIN talent t ON t.id = f.owner_id AND t.tenant_id = p_tenant_id AND t.deleted_at IS NULL
   WHERE f.uuid = p_uuid AND f.tenant_id = p_tenant_id AND f.owner_type = 'TALENT' AND f.deleted_at IS NULL;
END $$

DROP PROCEDURE IF EXISTS sp_talent_file_delete $$
CREATE PROCEDURE sp_talent_file_delete(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_file BIGINT UNSIGNED;
  DECLARE v_talent BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  SELECT id, owner_id INTO v_file, v_talent FROM file_object
   WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND owner_type = 'TALENT' AND deleted_at IS NULL;
  IF v_file IS NOT NULL THEN
    UPDATE file_object SET deleted_at = NOW(3), deleted_by = p_actor_id WHERE id = v_file;
    UPDATE talent SET photo_file_id = IF(photo_file_id = v_file, NULL, photo_file_id),
                      cv_file_id = IF(cv_file_id = v_file, NULL, cv_file_id) WHERE id = v_talent;
    CALL sp_talent_touch(v_talent, p_actor_id);
    CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'DELETE', 'talent_file', p_uuid, NULL, p_ip);
  END IF;
END $$

-- ---------------------------------------------------------------------
-- Idempotencia de operaciones (cola offline): resultado guardado de una operación
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_sync_get $$
CREATE PROCEDURE sp_sync_get(IN p_tenant_id BIGINT UNSIGNED, IN p_client_op_id CHAR(36))
BEGIN
  SELECT status, result FROM sync_operation WHERE tenant_id = p_tenant_id AND client_op_id = p_client_op_id;
END $$

-- Talento existente con el mismo correo o documento (índices ciegos) o el mismo nombre completo (sin distinguir
-- mayúsculas ni tildes; si uno no tiene segundo apellido basta nombres + primer apellido): la carga rápida con CV
-- actualiza en vez de duplicar. Prioridad: correo, documento, nombre.
DROP PROCEDURE IF EXISTS sp_talent_match $$
CREATE PROCEDURE sp_talent_match(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_email_bidx CHAR(64), IN p_doc_bidx CHAR(64),
  IN p_first VARCHAR(100), IN p_last VARCHAR(100), IN p_second VARCHAR(100))
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  SELECT t.uuid, t.first_name, t.last_name, t.second_last_name, t.headline,
         CASE WHEN p_email_bidx IS NOT NULL AND t.email_bidx = p_email_bidx THEN 'EMAIL'
              WHEN p_doc_bidx IS NOT NULL AND t.doc_number_bidx = p_doc_bidx THEN 'DOC' ELSE 'NAME' END AS matched_by
    FROM talent t
   WHERE t.tenant_id = p_tenant_id AND t.deleted_at IS NULL
     AND ((p_email_bidx IS NOT NULL AND t.email_bidx = p_email_bidx) OR (p_doc_bidx IS NOT NULL AND t.doc_number_bidx = p_doc_bidx)
          OR (p_first IS NOT NULL AND p_last IS NOT NULL AND t.first_name = p_first AND t.last_name = p_last
              AND (p_second IS NULL OR t.second_last_name IS NULL OR t.second_last_name = p_second)))
   ORDER BY (p_email_bidx IS NOT NULL AND t.email_bidx = p_email_bidx) DESC, (p_doc_bidx IS NOT NULL AND t.doc_number_bidx = p_doc_bidx) DESC,
            (t.second_last_name = p_second) DESC, t.updated_at DESC
   LIMIT 1;
END $$

DELIMITER ;
