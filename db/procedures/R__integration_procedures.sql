-- =====================================================================
-- API de integración (/integration/v1, API key con alcance talent.read): ficha de talentos que son candidatos en algún
-- RQ del tenant de la key, y sus CV; con el alcance talent.search, búsqueda en todo el banco del tenant y ficha de
-- cualquier talento. La usa IGDT con la llave BDT automática (tenant_integration.link_key_id).
-- Sin tarifas ni datos salariales. Los datos personales salen cifrados (base64, claves "...Enc"): la API los descifra.
-- Cada lectura queda en la auditoría con la key que la hizo.
-- =====================================================================
DELIMITER $$

-- Talento del tenant (no eliminado) que es candidato vigente en al menos un RQ no eliminado del tenant.
DROP PROCEDURE IF EXISTS sp_integration_candidate_resolve $$
CREATE PROCEDURE sp_integration_candidate_resolve(IN p_tenant_id BIGINT UNSIGNED, IN p_talent_uuid CHAR(36), OUT o_id BIGINT UNSIGNED)
BEGIN
  SET o_id = (SELECT t.id FROM talent t
               WHERE t.uuid = p_talent_uuid AND t.tenant_id = p_tenant_id AND t.deleted_at IS NULL
                 AND EXISTS (SELECT 1 FROM requirement_candidate rc
                               JOIN requirement r ON r.id = rc.requirement_id AND r.tenant_id = p_tenant_id AND r.deleted_at IS NULL
                              WHERE rc.talent_id = t.id AND rc.deleted_at IS NULL));
  IF o_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TALENT_NOT_FOUND'; END IF;
END $$

-- Talento accesible para la key: con el alcance talent.search, cualquier talento del tenant no eliminado (cualquier
-- status); si no, solo candidatos (sp_integration_candidate_resolve).
DROP PROCEDURE IF EXISTS sp_integration_talent_resolve $$
CREATE PROCEDURE sp_integration_talent_resolve(IN p_tenant_id BIGINT UNSIGNED, IN p_api_key_id BIGINT UNSIGNED,
  IN p_talent_uuid CHAR(36), OUT o_id BIGINT UNSIGNED)
BEGIN
  IF EXISTS (SELECT 1 FROM api_key k WHERE k.id = p_api_key_id AND k.tenant_id = p_tenant_id AND k.deleted_at IS NULL
               AND JSON_CONTAINS(k.scopes, JSON_QUOTE('talent.search'))) THEN
    SET o_id = (SELECT t.id FROM talent t WHERE t.uuid = p_talent_uuid AND t.tenant_id = p_tenant_id AND t.deleted_at IS NULL);
    IF o_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TALENT_NOT_FOUND'; END IF;
  ELSE
    CALL sp_integration_candidate_resolve(p_tenant_id, p_talent_uuid, o_id);
  END IF;
END $$

-- Búsqueda en el banco de talentos del tenant (alcance talent.search): como sp_talent_search, sin favoritos, colecciones
-- ni actor; solo talentos ACTIVE no eliminados. p_skill: texto contra la etiqueta (es/en) de las habilidades técnicas;
-- p_english_level: código mínimo de LANGUAGE_LEVEL (desconocido = VALIDATION). Result set 1: total; result set 2: un
-- documento JSON por talento (sin datos personales cifrados, tarifas ni lista negra). Audita una lectura con la key.
DROP PROCEDURE IF EXISTS sp_integration_talent_search $$
CREATE PROCEDURE sp_integration_talent_search(IN p_tenant_id BIGINT UNSIGNED, IN p_api_key_id BIGINT UNSIGNED,
  IN p_search VARCHAR(150), IN p_position VARCHAR(150), IN p_education VARCHAR(150), IN p_skill VARCHAR(150),
  IN p_english_level VARCHAR(30), IN p_min_years DECIMAL(5,1), IN p_page INT, IN p_page_size INT, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_offset INT;
  DECLARE v_en_lang BIGINT UNSIGNED;
  DECLARE v_en_min INT;
  DECLARE v_search VARCHAR(160);
  DECLARE v_position VARCHAR(160);
  DECLARE v_education VARCHAR(160);
  DECLARE v_skill VARCHAR(160);

  SET p_page = GREATEST(IFNULL(p_page, 1), 1);
  SET p_page_size = LEAST(GREATEST(IFNULL(p_page_size, 20), 1), 50);
  SET v_offset = (p_page - 1) * p_page_size;
  SET v_search = IF(p_search IS NULL OR TRIM(p_search) = '', NULL, CONCAT('%', LOWER(TRIM(p_search)), '%'));
  SET v_position = IF(p_position IS NULL OR TRIM(p_position) = '', NULL, CONCAT('%', LOWER(TRIM(p_position)), '%'));
  SET v_education = IF(p_education IS NULL OR TRIM(p_education) = '', NULL, CONCAT('%', LOWER(TRIM(p_education)), '%'));
  SET v_skill = IF(p_skill IS NULL OR TRIM(p_skill) = '', NULL, CONCAT('%', LOWER(TRIM(p_skill)), '%'));
  SET v_en_lang = (SELECT i.id FROM master_item i JOIN master_table m ON m.id = i.master_table_id
                    WHERE m.code = 'LANGUAGE' AND i.code = 'EN' AND i.tenant_id IS NULL LIMIT 1);
  IF p_english_level IS NOT NULL AND p_english_level <> '' THEN
    SET v_en_min = (SELECT i.sort_order FROM master_item i JOIN master_table m ON m.id = i.master_table_id
                     WHERE m.code = 'LANGUAGE_LEVEL' AND i.code = p_english_level AND i.tenant_id IS NULL AND i.deleted_at IS NULL
                     LIMIT 1);
    IF v_en_min IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  END IF;

  CALL sp_audit_insert(p_tenant_id, NULL, p_api_key_id, 'READ', 'talent_search', NULL,
                       JSON_OBJECT('via', 'integration', 'q', p_search, 'position', p_position, 'education', p_education,
                                   'skill', p_skill, 'english', p_english_level, 'minYears', p_min_years,
                                   'page', p_page, 'pageSize', p_page_size), p_ip);

  DROP TEMPORARY TABLE IF EXISTS tmp_integration_talent_hit;
  CREATE TEMPORARY TABLE tmp_integration_talent_hit (id BIGINT UNSIGNED PRIMARY KEY, years DECIMAL(6,1), sort_ts DATETIME(3)) ENGINE=MEMORY;

  INSERT INTO tmp_integration_talent_hit (id, years, sort_ts)
  SELECT t.id,
         IFNULL((SELECT ROUND(SUM(TIMESTAMPDIFF(MONTH, e.start_date, IFNULL(e.end_date, CURDATE()))) / 12, 1)
                   FROM talent_experience e WHERE e.talent_id = t.id AND e.deleted_at IS NULL), 0),
         IFNULL(t.updated_at, t.created_at)
    FROM talent t
   WHERE t.tenant_id = p_tenant_id AND t.deleted_at IS NULL AND t.status = 'ACTIVE'
     AND (v_search IS NULL OR t.search_name LIKE v_search OR LOWER(t.headline) LIKE v_search)
     AND (v_skill IS NULL OR EXISTS (SELECT 1 FROM talent_tech_skill s JOIN master_item mi ON mi.id = s.skill_item_id
                                     WHERE s.talent_id = t.id AND s.deleted_at IS NULL
                                       AND (LOWER(JSON_UNQUOTE(JSON_EXTRACT(mi.label_i18n, '$.es'))) LIKE v_skill
                                            OR LOWER(JSON_UNQUOTE(JSON_EXTRACT(mi.label_i18n, '$.en'))) LIKE v_skill)))
     AND (v_en_min IS NULL OR EXISTS (SELECT 1 FROM talent_language l JOIN master_item lv ON lv.id = l.level_item_id
                                      WHERE l.talent_id = t.id AND l.deleted_at IS NULL AND l.language_item_id = v_en_lang
                                        AND lv.sort_order >= v_en_min))
     AND (v_position IS NULL OR LOWER(t.headline) LIKE v_position
          OR EXISTS (SELECT 1 FROM talent_experience e WHERE e.talent_id = t.id AND e.deleted_at IS NULL
                       AND LOWER(e.position) LIKE v_position))
     AND (v_education IS NULL OR EXISTS (SELECT 1 FROM talent_education d WHERE d.talent_id = t.id AND d.deleted_at IS NULL
                       AND (LOWER(d.career) LIKE v_education OR LOWER(d.institution) LIKE v_education)));

  IF p_min_years IS NOT NULL AND p_min_years > 0 THEN
    DELETE FROM tmp_integration_talent_hit WHERE years < p_min_years;
  END IF;

  SELECT COUNT(*) AS total FROM tmp_integration_talent_hit;

  SELECT JSON_OBJECT(
           'id', t.uuid, 'firstName', t.first_name, 'lastName', t.last_name, 'secondLastName', t.second_last_name,
           'headline', t.headline, 'country', c.label_i18n, 'city', t.city, 'yearsExperience', h.years,
           'englishLevel', (SELECT lv.code FROM talent_language l JOIN master_item lv ON lv.id = l.level_item_id
                             WHERE l.talent_id = t.id AND l.deleted_at IS NULL AND l.language_item_id = v_en_lang LIMIT 1),
           'skills', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('label', x.label_i18n, 'years', x.years))
                               FROM (SELECT mi.label_i18n, s.years
                                       FROM talent_tech_skill s JOIN master_item mi ON mi.id = s.skill_item_id
                                      WHERE s.talent_id = t.id AND s.deleted_at IS NULL ORDER BY s.years DESC, s.id) x), JSON_ARRAY()),
           'avgRating', t.avg_rating,
           'updatedAt', DATE_FORMAT(IFNULL(t.updated_at, t.created_at), '%Y-%m-%dT%H:%i:%s.%fZ'),
           'isCandidate', IF(EXISTS (SELECT 1 FROM requirement_candidate rc
                                       JOIN requirement r ON r.id = rc.requirement_id AND r.tenant_id = p_tenant_id AND r.deleted_at IS NULL
                                      WHERE rc.talent_id = t.id AND rc.deleted_at IS NULL),
                             CAST('true' AS JSON), CAST('false' AS JSON))
         ) AS doc
    FROM tmp_integration_talent_hit h
    JOIN talent t ON t.id = h.id
    LEFT JOIN master_item c ON c.id = t.country_item_id
   ORDER BY h.sort_ts DESC, t.id DESC
   LIMIT v_offset, p_page_size;

  DROP TEMPORARY TABLE IF EXISTS tmp_integration_talent_hit;
END $$

-- Ficha del candidato (un documento JSON). Mismos datos que la ficha del talento (sp_talent_get) sin tarifas, solo los
-- archivos de CV, documentos y certificados, y las postulaciones del tenant (como sp_rq_by_talent, más la fecha de ingreso registrada en IGDT).
DROP PROCEDURE IF EXISTS sp_integration_talent_dossier $$
CREATE PROCEDURE sp_integration_talent_dossier(IN p_tenant_id BIGINT UNSIGNED, IN p_api_key_id BIGINT UNSIGNED,
  IN p_talent_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  CALL sp_integration_talent_resolve(p_tenant_id, p_api_key_id, p_talent_uuid, v_id);
  CALL sp_audit_insert(p_tenant_id, NULL, p_api_key_id, 'READ', 'talent', p_talent_uuid, JSON_OBJECT('via', 'integration'), p_ip);

  SELECT JSON_OBJECT(
           'id', t.uuid, 'firstName', t.first_name, 'lastName', t.last_name, 'secondLastName', t.second_last_name,
           'headline', t.headline,
           'docType', IF(dt.id IS NULL, NULL, JSON_OBJECT('code', dt.code, 'label', dt.label_i18n)),
           'docNumberEnc', TO_BASE64(t.doc_number_enc), 'emailEnc', TO_BASE64(t.email_enc), 'phoneEnc', TO_BASE64(t.phone_enc),
           'birthDateEnc', TO_BASE64(t.birth_date_enc),
           'country', c.label_i18n, 'city', t.city, 'originCountry', oc.label_i18n, 'originCity', t.origin_city,
           'linkedinUrl', t.linkedin_url, 'githubUrl', t.github_url, 'summary', t.summary,
           'availability', av.label_i18n,
           'hasEquipment', IF(t.has_equipment IS NULL, NULL, IF(t.has_equipment = 1, CAST('true' AS JSON), CAST('false' AS JSON))),
           'avgRating', t.avg_rating, 'status', t.status,
           'updatedAt', DATE_FORMAT(IFNULL(t.updated_at, t.created_at), '%Y-%m-%dT%H:%i:%s.%fZ'),
           'experiences', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', x.uuid, 'company', x.company, 'position', x.position,
                              'startDate', x.start_date, 'endDate', x.end_date,
                              'isCurrent', IF(x.is_current = 1, CAST('true' AS JSON), CAST('false' AS JSON)),
                              'months', x.months, 'functions', x.functions))
                            FROM (SELECT e.uuid, e.company, e.position, e.start_date, e.end_date, e.is_current, e.functions,
                                         TIMESTAMPDIFF(MONTH, e.start_date, IFNULL(e.end_date, CURDATE())) AS months
                                    FROM talent_experience e WHERE e.talent_id = v_id AND e.deleted_at IS NULL
                                   ORDER BY e.is_current DESC, e.start_date DESC) x), JSON_ARRAY()),
           'educations', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', x.uuid, 'institution', x.institution, 'career', x.career,
                              'grade', x.grade, 'startDate', x.start_date, 'endDate', x.end_date,
                              'isCurrent', IF(x.is_current = 1, CAST('true' AS JSON), CAST('false' AS JSON)),
                              'datePrecision', x.date_precision))
                            FROM (SELECT d.uuid, d.institution, d.career, g.label_i18n AS grade, d.start_date, d.end_date,
                                         d.is_current, d.date_precision
                                    FROM talent_education d LEFT JOIN master_item g ON g.id = d.grade_item_id
                                   WHERE d.talent_id = v_id AND d.deleted_at IS NULL
                                   ORDER BY d.is_current DESC, d.start_date DESC) x), JSON_ARRAY()),
           'languages', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', x.uuid, 'languageId', x.language_uuid, 'language', x.language,
                              'level', x.level, 'levelCode', x.level_code, 'stars', x.stars))
                            FROM (SELECT l.uuid, lg.uuid AS language_uuid, lg.label_i18n AS language, lv.label_i18n AS level,
                                         lv.code AS level_code, l.stars
                                    FROM talent_language l JOIN master_item lg ON lg.id = l.language_item_id
                                    JOIN master_item lv ON lv.id = l.level_item_id
                                   WHERE l.talent_id = v_id AND l.deleted_at IS NULL ORDER BY lg.sort_order) x), JSON_ARRAY()),
           'techSkills', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', x.uuid, 'skillId', x.skill_uuid, 'label', x.label_i18n, 'years', x.years))
                            FROM (SELECT s.uuid, mi.uuid AS skill_uuid, mi.label_i18n, s.years
                                    FROM talent_tech_skill s JOIN master_item mi ON mi.id = s.skill_item_id
                                   WHERE s.talent_id = v_id AND s.deleted_at IS NULL ORDER BY s.years DESC, s.id) x), JSON_ARRAY()),
           'softSkills', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', x.uuid, 'skillId', x.skill_uuid, 'label', x.label_i18n))
                            FROM (SELECT s.uuid, mi.uuid AS skill_uuid, mi.label_i18n
                                    FROM talent_soft_skill s JOIN master_item mi ON mi.id = s.skill_item_id
                                   WHERE s.talent_id = v_id AND s.deleted_at IS NULL ORDER BY s.id) x), JSON_ARRAY()),
           'feedback', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', x.uuid, 'stars', x.stars, 'comment', x.comment,
                              'authorNameEnc', x.author_name_enc, 'createdAt', x.created_at))
                            FROM (SELECT f.uuid, f.stars, f.comment, TO_BASE64(u.full_name_enc) AS author_name_enc,
                                         DATE_FORMAT(f.created_at, '%Y-%m-%dT%H:%i:%s.%fZ') AS created_at
                                    FROM talent_feedback f JOIN app_user u ON u.id = f.author_user_id
                                   WHERE f.talent_id = v_id AND f.deleted_at IS NULL ORDER BY f.created_at DESC) x), JSON_ARRAY()),
           'cvId', cf.uuid,
           'files', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', x.uuid, 'category', x.category, 'name', x.original_name,
                              'mimeType', x.mime_type, 'size', x.size_bytes, 'createdAt', x.created_at))
                            FROM (SELECT fo.uuid, fo.category, fo.original_name, fo.mime_type, fo.size_bytes,
                                         DATE_FORMAT(fo.created_at, '%Y-%m-%dT%H:%i:%s.%fZ') AS created_at
                                    FROM file_object fo
                                   WHERE fo.tenant_id = p_tenant_id AND fo.owner_type = 'TALENT' AND fo.owner_id = v_id
                                     AND fo.deleted_at IS NULL
                                     AND fo.category IN ('CV', 'CV_TENANT_ES', 'CV_TENANT_EN', 'DOCUMENT', 'CERTIFICATE')
                                   ORDER BY fo.created_at DESC) x), JSON_ARRAY()),
           'applications', IFNULL((SELECT JSON_ARRAYAGG(x.doc)
                            FROM (SELECT JSON_OBJECT('candidateId', rc.uuid, 'requirementId', r.uuid, 'code', r.code, 'title', r.title,
                                           'dueDate', r.due_date, 'client', cl.label_i18n, 'profile', p.label_i18n,
                                           'status', JSON_OBJECT('code', s.code, 'label', s.label_i18n, 'group', s.str1),
                                           'stage', JSON_OBJECT('code', cs.code, 'label', cs.label_i18n),
                                           'addedAt', DATE_FORMAT(rc.created_at, '%Y-%m-%dT%H:%i:%s.%fZ'),
                                           'resignation', IF(rc.resigned_at IS NULL, NULL, JSON_OBJECT(
                                               'at', DATE_FORMAT(rc.resigned_at, '%Y-%m-%dT%H:%i:%s.%fZ'), 'offboard', rc.offboard_status,
                                               'offboardAt', IF(rc.offboard_at IS NULL, NULL, DATE_FORMAT(rc.offboard_at, '%Y-%m-%dT%H:%i:%s.%fZ')))),
                                           'hiredAt', IF(rc.hired_at IS NULL, NULL, DATE_FORMAT(rc.hired_at, '%Y-%m-%dT%H:%i:%s.%fZ')),
                                           'hireStartDate', rc.hire_start_date) AS doc
                                    FROM requirement_candidate rc
                                    JOIN requirement r ON r.id = rc.requirement_id AND r.deleted_at IS NULL AND r.tenant_id = p_tenant_id
                                    JOIN master_item s ON s.id = r.status_item_id
                                    JOIN master_item cl ON cl.id = r.client_item_id
                                    JOIN requirement_vacancy v ON v.id = rc.vacancy_id
                                    JOIN master_item p ON p.id = v.profile_item_id
                                    JOIN master_item cs ON cs.id = rc.stage_item_id
                                   WHERE rc.talent_id = v_id AND rc.deleted_at IS NULL
                                   ORDER BY rc.created_at DESC) x), JSON_ARRAY())
         ) AS doc
    FROM talent t
    LEFT JOIN master_item dt ON dt.id = t.doc_type_item_id
    LEFT JOIN master_item c ON c.id = t.country_item_id
    LEFT JOIN master_item oc ON oc.id = t.origin_country_item_id
    LEFT JOIN master_item av ON av.id = t.availability_item_id
    LEFT JOIN file_object cf ON cf.id = t.cv_file_id AND cf.deleted_at IS NULL
   WHERE t.id = v_id;
END $$

-- Archivo de un candidato (CV original, CV formato empresa ES/EN, documento o certificado): mismas columnas que
-- sp_talent_file_get.
DROP PROCEDURE IF EXISTS sp_integration_talent_file $$
CREATE PROCEDURE sp_integration_talent_file(IN p_tenant_id BIGINT UNSIGNED, IN p_api_key_id BIGINT UNSIGNED,
  IN p_talent_uuid CHAR(36), IN p_file_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  CALL sp_integration_talent_resolve(p_tenant_id, p_api_key_id, p_talent_uuid, v_id);
  IF NOT EXISTS (SELECT 1 FROM file_object WHERE uuid = p_file_uuid AND tenant_id = p_tenant_id AND owner_type = 'TALENT'
                   AND owner_id = v_id AND deleted_at IS NULL
                   AND category IN ('CV', 'CV_TENANT_ES', 'CV_TENANT_EN', 'DOCUMENT', 'CERTIFICATE')) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FILE_NOT_FOUND';
  END IF;
  CALL sp_audit_insert(p_tenant_id, NULL, p_api_key_id, 'DOWNLOAD', 'talent_file', p_file_uuid,
                       JSON_OBJECT('via', 'integration', 'talent', p_talent_uuid), p_ip);
  SELECT f.id, f.uuid, f.tenant_id, f.owner_type, f.category, f.storage_key, f.original_name, f.mime_type, f.size_bytes, f.sha256, f.is_encrypted
    FROM file_object f
   WHERE f.uuid = p_file_uuid AND f.tenant_id = p_tenant_id AND f.owner_type = 'TALENT' AND f.owner_id = v_id AND f.deleted_at IS NULL;
END $$

-- Insumos para generar el CV con la marca del tenant (como lo hace el front de BDT con buildCvData/buildCvTemplate).
-- Result set 1: un documento JSON {talent, courseGradeId, template} con el talento en la forma del type Talent del front
-- (solo los campos que usa el CV; etiquetas de catálogo como {es,en}) y la plantilla del tenant (sin imágenes).
-- Result set 2: imágenes a incrustar (kind HEADER | FOOTER | LOGO | PHOTO) con su ubicación en el almacenamiento; el logo
-- (oscuro o normal) solo si no hay cabecera y la foto del talento solo si la plantilla la muestra.
DROP PROCEDURE IF EXISTS sp_integration_talent_cv_source $$
CREATE PROCEDURE sp_integration_talent_cv_source(IN p_tenant_id BIGINT UNSIGNED, IN p_api_key_id BIGINT UNSIGNED,
  IN p_talent_uuid CHAR(36), IN p_lang VARCHAR(5), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  CALL sp_integration_talent_resolve(p_tenant_id, p_api_key_id, p_talent_uuid, v_id);
  CALL sp_audit_insert(p_tenant_id, NULL, p_api_key_id, 'READ', 'talent', p_talent_uuid,
                       JSON_OBJECT('via', 'integration', 'cvSource', p_lang), p_ip);

  SELECT JSON_OBJECT(
           'talent', JSON_OBJECT(
             'id', t.uuid, 'firstName', t.first_name, 'lastName', t.last_name, 'secondLastName', t.second_last_name,
             'headline', t.headline, 'summary', t.summary, 'city', t.city, 'country', c.label_i18n, 'photoId', pf.uuid,
             'updatedAt', DATE_FORMAT(IFNULL(t.updated_at, t.created_at), '%Y-%m-%dT%H:%i:%s.%fZ'),
             'experiences', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', x.uuid, 'company', x.company, 'position', x.position,
                                'startDate', x.start_date, 'endDate', x.end_date,
                                'isCurrent', IF(x.is_current = 1, CAST('true' AS JSON), CAST('false' AS JSON)),
                                'functions', x.functions))
                              FROM (SELECT e.uuid, e.company, e.position, e.start_date, e.end_date, e.is_current, e.functions
                                      FROM talent_experience e WHERE e.talent_id = v_id AND e.deleted_at IS NULL
                                     ORDER BY e.is_current DESC, e.start_date DESC) x), JSON_ARRAY()),
             'educations', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', x.uuid, 'gradeId', x.grade_uuid, 'grade', x.grade,
                                'institution', x.institution, 'career', x.career, 'startDate', x.start_date, 'endDate', x.end_date,
                                'isCurrent', IF(x.is_current = 1, CAST('true' AS JSON), CAST('false' AS JSON)),
                                'datePrecision', x.date_precision))
                              FROM (SELECT d.uuid, g.uuid AS grade_uuid, g.label_i18n AS grade, d.institution, d.career,
                                           d.start_date, d.end_date, d.is_current, d.date_precision
                                      FROM talent_education d LEFT JOIN master_item g ON g.id = d.grade_item_id
                                     WHERE d.talent_id = v_id AND d.deleted_at IS NULL
                                     ORDER BY d.is_current DESC, d.start_date DESC) x), JSON_ARRAY()),
             'techSkills', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('label', x.label_i18n, 'years', x.years))
                              FROM (SELECT mi.label_i18n, s.years
                                      FROM talent_tech_skill s JOIN master_item mi ON mi.id = s.skill_item_id
                                     WHERE s.talent_id = v_id AND s.deleted_at IS NULL ORDER BY s.years DESC, s.id) x), JSON_ARRAY()),
             'softSkills', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('label', x.label_i18n))
                              FROM (SELECT mi.label_i18n
                                      FROM talent_soft_skill s JOIN master_item mi ON mi.id = s.skill_item_id
                                     WHERE s.talent_id = v_id AND s.deleted_at IS NULL ORDER BY s.id) x), JSON_ARRAY()),
             'languages', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('language', x.language, 'level', x.level))
                              FROM (SELECT lg.label_i18n AS language, lv.label_i18n AS level
                                      FROM talent_language l JOIN master_item lg ON lg.id = l.language_item_id
                                      JOIN master_item lv ON lv.id = l.level_item_id
                                     WHERE l.talent_id = v_id AND l.deleted_at IS NULL ORDER BY lg.sort_order) x), JSON_ARRAY())),
           'courseGradeId', (SELECT mi.uuid FROM master_item mi JOIN master_table mt ON mt.id = mi.master_table_id
                              WHERE mt.code = 'ACADEMIC_GRADE' AND mi.code = 'COURSE' AND mi.deleted_at IS NULL
                                AND (mi.tenant_id IS NULL OR mi.tenant_id = p_tenant_id)
                              ORDER BY mi.tenant_id IS NULL DESC, mi.id LIMIT 1),
           'template', JSON_OBJECT(
             'displayName', b.display_name, 'colorPrimary', b.color_primary, 'colorSecondary', b.color_secondary,
             'cvFooterText', b.cv_footer_text,
             'cvShowPhoto', IF(IFNULL(b.cv_show_photo, 0) = 1, CAST('true' AS JSON), CAST('false' AS JSON)),
             'cvStyle', b.cv_style)
         ) AS doc
    FROM talent t
    LEFT JOIN master_item c ON c.id = t.country_item_id
    LEFT JOIN file_object pf ON pf.id = t.photo_file_id AND pf.deleted_at IS NULL
    LEFT JOIN tenant_branding b ON b.tenant_id = p_tenant_id
   WHERE t.id = v_id;

  SELECT x.kind, x.storage_key, x.mime_type, x.size_bytes, x.is_encrypted
    FROM (SELECT 'HEADER' AS kind, f.storage_key, f.mime_type, f.size_bytes, f.is_encrypted
            FROM tenant_branding b JOIN file_object f ON f.id = b.cv_header_file_id AND f.deleted_at IS NULL
           WHERE b.tenant_id = p_tenant_id
          UNION ALL
          SELECT 'FOOTER', f.storage_key, f.mime_type, f.size_bytes, f.is_encrypted
            FROM tenant_branding b JOIN file_object f ON f.id = b.cv_footer_file_id AND f.deleted_at IS NULL
           WHERE b.tenant_id = p_tenant_id
          UNION ALL
          SELECT 'LOGO', f.storage_key, f.mime_type, f.size_bytes, f.is_encrypted
            FROM tenant_branding b
            LEFT JOIN file_object h ON h.id = b.cv_header_file_id AND h.deleted_at IS NULL
            LEFT JOIN file_object ld ON ld.id = b.logo_dark_file_id AND ld.deleted_at IS NULL
            JOIN file_object f ON f.id = IFNULL(ld.id, b.logo_file_id) AND f.deleted_at IS NULL
           WHERE b.tenant_id = p_tenant_id AND h.id IS NULL
          UNION ALL
          SELECT 'PHOTO', f.storage_key, f.mime_type, f.size_bytes, f.is_encrypted
            FROM talent t
            JOIN tenant_branding b ON b.tenant_id = p_tenant_id AND b.cv_show_photo = 1
            JOIN file_object f ON f.id = t.photo_file_id AND f.deleted_at IS NULL
           WHERE t.id = v_id) x;
END $$

DELIMITER ;
