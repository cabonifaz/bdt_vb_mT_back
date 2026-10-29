-- =====================================================================
-- Documentos de postulación: maestro del tenant (document_rule), documentos presentados por los postulantes
-- (talent_document) con su validación manual o automática, y ficha del postulante dentro del RQ
-- (documentos que pide el RQ + línea de tiempo).
-- Estado efectivo de un documento: el último presentado del tipo (si el tipo no es reutilizable, solo el de esa
-- postulación); EXPIRED si venció; MISSING si no hay ninguno.
-- Los nombres de usuarios van cifrados en base64 (campos *NameEnc): los descifra la app.
-- =====================================================================
DELIMITER $$

-- Postulante (requirement_candidate) del tenant
DROP PROCEDURE IF EXISTS sp_candidate_resolve $$
CREATE PROCEDURE sp_candidate_resolve(IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  OUT o_id BIGINT UNSIGNED, OUT o_rq BIGINT UNSIGNED, OUT o_talent BIGINT UNSIGNED)
BEGIN
  SET o_id = NULL, o_rq = NULL, o_talent = NULL;
  SELECT rc.id, rc.requirement_id, rc.talent_id INTO o_id, o_rq, o_talent
    FROM requirement_candidate rc
    JOIN requirement r ON r.id = rc.requirement_id AND r.tenant_id = p_tenant_id AND r.deleted_at IS NULL
   WHERE rc.uuid = p_uuid AND rc.deleted_at IS NULL;
  IF o_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
END $$

-- Talento de una postulación (para guardar el archivo antes de registrar el documento)
DROP PROCEDURE IF EXISTS sp_candidate_talent $$
CREATE PROCEDURE sp_candidate_talent(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  DECLARE v_id, v_rq, v_talent BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.candidates');
  CALL sp_candidate_resolve(p_tenant_id, p_uuid, v_id, v_rq, v_talent);
  SELECT uuid FROM talent WHERE id = v_talent;
END $$

-- ---------------- Maestro de documentos del tenant ----------------
DROP PROCEDURE IF EXISTS sp_doc_rules $$
CREATE PROCEDURE sp_doc_rules(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.read');
  SELECT JSON_OBJECT('typeId', i.uuid, 'code', i.code, 'label', i.label_i18n, 'defaultMode', d.default_mode,
           'reusable', d.reusable = 1, 'validityDays', d.validity_days, 'verification', d.verification,
           'active', d.is_active = 1, 'sortOrder', d.sort_order) AS doc
    FROM document_rule d JOIN master_item i ON i.id = d.document_type_item_id AND i.deleted_at IS NULL
   WHERE d.tenant_id = p_tenant_id
   ORDER BY d.sort_order, i.sort_order, i.id;
END $$

-- Alta o edición de una regla. La validación automática solo existe para el CUL.
DROP PROCEDURE IF EXISTS sp_doc_rule_save $$
CREATE PROCEDURE sp_doc_rule_save(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_type_uuid CHAR(36),
  IN p_mode VARCHAR(10), IN p_reusable TINYINT(1), IN p_validity INT, IN p_verification VARCHAR(10), IN p_active TINYINT(1),
  IN p_sort INT, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_type BIGINT UNSIGNED;
  DECLARE v_code VARCHAR(60);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.masters.manage');
  CALL sp_master_resolve_code(p_tenant_id, 'DOCUMENT_TYPE', p_type_uuid, v_type, v_code);
  IF v_type IS NULL OR p_mode NOT IN ('REQUIRED','OPTIONAL') OR p_verification NOT IN ('MANUAL','AUTO_CUL')
     OR (p_verification = 'AUTO_CUL' AND v_code <> 'CUL') OR (p_validity IS NOT NULL AND (p_validity < 1 OR p_validity > 3650)) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION';
  END IF;
  INSERT INTO document_rule (tenant_id, document_type_item_id, default_mode, reusable, validity_days, verification, is_active, sort_order, created_by)
  VALUES (p_tenant_id, v_type, p_mode, IFNULL(p_reusable, 1), p_validity, p_verification, IFNULL(p_active, 1), IFNULL(p_sort, 0), p_actor_id)
  ON DUPLICATE KEY UPDATE default_mode = VALUES(default_mode), reusable = VALUES(reusable), validity_days = VALUES(validity_days),
    verification = VALUES(verification), is_active = VALUES(is_active), sort_order = VALUES(sort_order), updated_by = p_actor_id;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'UPSERT', 'document_rule', p_type_uuid,
    JSON_OBJECT('mode', p_mode, 'reusable', p_reusable, 'validity', p_validity, 'verification', p_verification, 'active', p_active), p_ip);
END $$

-- ---------------- Ficha del postulante dentro del RQ ----------------
-- Documentos que pide el RQ con el estado del postulante en cada uno, y su línea de tiempo.
DROP PROCEDURE IF EXISTS sp_rq_candidate_detail $$
CREATE PROCEDURE sp_rq_candidate_detail(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_today DATE)
BEGIN
  DECLARE v_id, v_rq, v_talent BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.read');
  CALL sp_candidate_resolve(p_tenant_id, p_uuid, v_id, v_rq, v_talent);
  SELECT JSON_OBJECT(
           'id', rc.uuid, 'updatedAt', IFNULL(rc.updated_at, rc.created_at), 'addedAt', rc.created_at,
           'requirement', JSON_OBJECT('id', r.uuid, 'code', r.code, 'title', r.title, 'closed', s.str1 = 'CLOSED'),
           'stage', JSON_OBJECT('id', cs.uuid, 'code', cs.code, 'label', cs.label_i18n),
           'vacancy', JSON_OBJECT('id', v.uuid, 'profile', p.label_i18n),
           'resignation', IF(rc.resigned_at IS NULL, NULL, JSON_OBJECT('at', rc.resigned_at, 'offboard', rc.offboard_status, 'offboardAt', rc.offboard_at)),
           'talent', JSON_OBJECT('id', t.uuid, 'firstName', t.first_name, 'lastName', t.last_name, 'secondLastName', t.second_last_name,
                      'headline', t.headline, 'photoId', ph.uuid),
           'documents', IFNULL((
             SELECT JSON_ARRAYAGG(JSON_OBJECT('typeId', dt.uuid, 'code', dt.code, 'label', dt.label_i18n, 'mode', rd.mode,
                      'reusable', IFNULL(dr.reusable, 1) = 1, 'validityDays', dr.validity_days, 'verification', IFNULL(dr.verification, 'MANUAL'), 'sortOrder', IFNULL(dr.sort_order, 0),
                      'current', (SELECT JSON_OBJECT('id', td.uuid, 'fileId', f.uuid, 'fileName', f.original_name,
                                     'status', IF(td.expires_at IS NOT NULL AND td.expires_at < p_today, 'EXPIRED', td.status),
                                     'method', td.method, 'reason', td.reason, 'issuedAt', td.issued_at, 'expiresAt', td.expires_at,
                                     'createdAt', td.created_at, 'verifiedAt', td.verified_at, 'autoResult', td.auto_result,
                                     'verifiedByNameEnc', TO_BASE64(vu.full_name_enc), 'fromOtherRq', td.candidate_id IS NULL OR td.candidate_id <> rc.id)
                                    FROM talent_document td
                                    JOIN file_object f ON f.id = td.file_id
                                    LEFT JOIN app_user vu ON vu.id = td.verified_by
                                   WHERE td.talent_id = rc.talent_id AND td.document_type_item_id = rd.document_type_item_id AND td.deleted_at IS NULL
                                     AND (IFNULL(dr.reusable, 1) = 1 OR td.candidate_id = rc.id)
                                   ORDER BY td.id DESC LIMIT 1)))
               FROM requirement_document rd
               JOIN master_item dt ON dt.id = rd.document_type_item_id
               LEFT JOIN document_rule dr ON dr.tenant_id = r.tenant_id AND dr.document_type_item_id = rd.document_type_item_id
              WHERE rd.requirement_id = r.id), JSON_ARRAY()),
           'events', IFNULL((
             SELECT JSON_ARRAYAGG(JSON_OBJECT('type', e.type, 'data', e.data, 'at', e.created_at, 'byNameEnc', TO_BASE64(u.full_name_enc)))
               FROM requirement_candidate_event e LEFT JOIN app_user u ON u.id = e.created_by
              WHERE e.candidate_id = rc.id), JSON_ARRAY())
         ) AS doc
    FROM requirement_candidate rc
    JOIN requirement r ON r.id = rc.requirement_id
    JOIN master_item s ON s.id = r.status_item_id
    JOIN master_item cs ON cs.id = rc.stage_item_id
    JOIN requirement_vacancy v ON v.id = rc.vacancy_id
    JOIN master_item p ON p.id = v.profile_item_id
    JOIN talent t ON t.id = rc.talent_id
    LEFT JOIN file_object ph ON ph.id = t.photo_file_id AND ph.deleted_at IS NULL
   WHERE rc.id = v_id;
END $$

-- ---------------- Documentos presentados ----------------
-- Datos para validar un documento: regla, RUC del tenant, documento de identidad del talento (cifrado) y huella del archivo.
DROP PROCEDURE IF EXISTS sp_talent_document_context $$
CREATE PROCEDURE sp_talent_document_context(IN p_tenant_id BIGINT UNSIGNED, IN p_doc_id BIGINT UNSIGNED)
BEGIN
  SELECT td.uuid, dt.code, IFNULL(dr.verification, 'MANUAL'), dr.validity_days, tn.tax_id, t.doc_number_enc, it.code, f.uuid, f.sha256
    FROM talent_document td
    JOIN master_item dt ON dt.id = td.document_type_item_id
    LEFT JOIN document_rule dr ON dr.tenant_id = td.tenant_id AND dr.document_type_item_id = td.document_type_item_id
    JOIN tenant tn ON tn.id = td.tenant_id
    JOIN talent t ON t.id = td.talent_id
    LEFT JOIN master_item it ON it.id = t.doc_type_item_id
    JOIN file_object f ON f.id = td.file_id
   WHERE td.id = p_doc_id AND td.tenant_id = p_tenant_id;
END $$

-- Registra un documento ya subido (file_object del talento) para un tipo que pide el RQ del postulante.
-- Guarda la postulación de origen; si el tipo es reutilizable vale para cualquier RQ del talento mientras esté vigente,
-- si no, solo para esta postulación. Vigencia: desde hoy por validity_days
-- (la validación automática la corrige con la fecha de emisión real).
DROP PROCEDURE IF EXISTS sp_talent_document_create $$
CREATE PROCEDURE sp_talent_document_create(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_candidate_uuid CHAR(36), IN p_type_uuid CHAR(36), IN p_file_uuid CHAR(36), IN p_today DATE, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_cand, v_rq, v_talent, v_type, v_file, v_doc BIGINT UNSIGNED;
  DECLARE v_code VARCHAR(60);
  DECLARE v_validity INT;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.candidates');
  CALL sp_candidate_resolve(p_tenant_id, p_candidate_uuid, v_cand, v_rq, v_talent);
  CALL sp_master_resolve_code(p_tenant_id, 'DOCUMENT_TYPE', p_type_uuid, v_type, v_code);
  SELECT id INTO v_file FROM file_object
   WHERE uuid = p_file_uuid AND tenant_id = p_tenant_id AND owner_type = 'TALENT' AND owner_id = v_talent AND deleted_at IS NULL;
  IF v_type IS NULL OR v_file IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  SELECT validity_days INTO v_validity FROM document_rule WHERE tenant_id = p_tenant_id AND document_type_item_id = v_type;
  INSERT INTO talent_document (uuid, tenant_id, talent_id, document_type_item_id, file_id, candidate_id, issued_at, expires_at, created_by)
  VALUES (p_uuid, p_tenant_id, v_talent, v_type, v_file, v_cand,
          p_today, IF(v_validity IS NULL, NULL, p_today + INTERVAL v_validity DAY), p_actor_id);
  SET v_doc = LAST_INSERT_ID();
  INSERT INTO requirement_candidate_event (candidate_id, type, data, created_by)
  VALUES (v_cand, 'DOC_UPLOADED', JSON_OBJECT('doc', p_uuid, 'type', v_code), p_actor_id);
  CALL sp_talent_touch(v_talent, p_actor_id);
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'UPLOAD', 'talent_document', p_uuid, JSON_OBJECT('type', v_code, 'candidate', p_candidate_uuid), p_ip);
  CALL sp_talent_document_context(p_tenant_id, v_doc);
END $$

DROP PROCEDURE IF EXISTS sp_talent_document_get $$
CREATE PROCEDURE sp_talent_document_get(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  DECLARE v_doc BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.candidates');
  SELECT id INTO v_doc FROM talent_document WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
  IF v_doc IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  CALL sp_talent_document_context(p_tenant_id, v_doc);
END $$

-- Resultado de una validación (manual: aprobado o rechazado con motivo; automática: resultado del MTPE).
-- p_candidate_uuid es la postulación desde donde se validó (para su línea de tiempo).
DROP PROCEDURE IF EXISTS sp_talent_document_verify $$
CREATE PROCEDURE sp_talent_document_verify(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_candidate_uuid CHAR(36), IN p_status VARCHAR(10), IN p_method VARCHAR(10), IN p_reason VARCHAR(500), IN p_result JSON,
  IN p_issued DATE, IN p_expires DATE, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_doc, v_cand, v_rq, v_talent BIGINT UNSIGNED;
  DECLARE v_code VARCHAR(60);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.candidates');
  CALL sp_candidate_resolve(p_tenant_id, p_candidate_uuid, v_cand, v_rq, v_talent);
  SELECT td.id, dt.code INTO v_doc, v_code FROM talent_document td JOIN master_item dt ON dt.id = td.document_type_item_id
   WHERE td.uuid = p_uuid AND td.tenant_id = p_tenant_id AND td.talent_id = v_talent AND td.deleted_at IS NULL;
  IF v_doc IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  IF p_status NOT IN ('PENDING','VERIFIED','REJECTED') OR p_method NOT IN ('MANUAL','AUTO')
     OR (p_status = 'REJECTED' AND p_method = 'MANUAL' AND TRIM(IFNULL(p_reason, '')) = '') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION';
  END IF;
  UPDATE talent_document SET status = p_status, method = p_method, reason = NULLIF(TRIM(p_reason), ''), auto_result = p_result,
         issued_at = IFNULL(p_issued, issued_at), expires_at = IFNULL(p_expires, expires_at),
         verified_by = IF(p_status = 'PENDING', NULL, p_actor_id), verified_at = IF(p_status = 'PENDING', NULL, NOW(3))
   WHERE id = v_doc;
  INSERT INTO requirement_candidate_event (candidate_id, type, data, created_by)
  VALUES (v_cand, CASE p_status WHEN 'VERIFIED' THEN 'DOC_VERIFIED' WHEN 'REJECTED' THEN 'DOC_REJECTED' ELSE 'DOC_CHECK_FAILED' END,
          JSON_OBJECT('doc', p_uuid, 'type', v_code, 'method', p_method, 'reason', NULLIF(TRIM(p_reason), '')), p_actor_id);
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'VERIFY', 'talent_document', p_uuid,
    JSON_OBJECT('status', p_status, 'method', p_method), p_ip);
END $$

DELIMITER ;
