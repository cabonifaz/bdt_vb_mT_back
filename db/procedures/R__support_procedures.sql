-- =====================================================================
-- Soporte (solicitudes al Super Admin), novedades de la plataforma y guías de primer uso.
-- Solicitudes: las crea cualquier usuario de un tenant; las ve su autor y quien tiene platform.support.manage.
-- Los nombres van cifrados en base64 (campos *NameEnc): los descifra la app.
-- =====================================================================
DELIMITER $$

-- Solicitud visible para el actor: su autor o el Super Admin. Devuelve id y si el actor es staff.
DROP PROCEDURE IF EXISTS sp_support_resolve $$
CREATE PROCEDURE sp_support_resolve(IN p_actor_id BIGINT UNSIGNED, IN p_uuid CHAR(36), OUT o_id BIGINT UNSIGNED, OUT o_staff TINYINT(1))
BEGIN
  SET o_id = NULL;
  SET o_staff = EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'platform.support.manage');
  SELECT id INTO o_id FROM support_ticket WHERE uuid = p_uuid AND (o_staff = 1 OR reporter_user_id = p_actor_id);
  IF o_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
END $$

-- Adjunta archivos (file_object SUPPORT subidos por el actor y aún sin solicitud) a un mensaje
DROP PROCEDURE IF EXISTS sp_support_attach $$
CREATE PROCEDURE sp_support_attach(IN p_actor_id BIGINT UNSIGNED, IN p_ticket BIGINT UNSIGNED, IN p_message BIGINT UNSIGNED, IN p_files JSON)
BEGIN
  DECLARE v_count, v_valid INT;
  SET v_count = IF(p_files IS NULL, 0, JSON_LENGTH(p_files));
  IF v_count > 0 THEN
    SELECT COUNT(DISTINCT f.id) INTO v_valid
      FROM JSON_TABLE(p_files, '$[*]' COLUMNS (fid CHAR(36) PATH '$')) j
      JOIN file_object f ON f.uuid = j.fid AND f.owner_type = 'SUPPORT' AND f.owner_id IS NULL AND f.created_by = p_actor_id AND f.deleted_at IS NULL;
    IF v_valid <> v_count OR v_count > 10 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
    INSERT INTO support_attachment (ticket_id, message_id, file_id)
    SELECT p_ticket, p_message, f.id FROM JSON_TABLE(p_files, '$[*]' COLUMNS (fid CHAR(36) PATH '$')) j JOIN file_object f ON f.uuid = j.fid;
    UPDATE file_object f JOIN JSON_TABLE(p_files, '$[*]' COLUMNS (fid CHAR(36) PATH '$')) j ON j.fid = f.uuid SET f.owner_id = p_ticket;
  END IF;
END $$

-- Alta de una solicitud con su primer mensaje. Devuelve uuid y número.
DROP PROCEDURE IF EXISTS sp_support_create $$
CREATE PROCEDURE sp_support_create(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_kind VARCHAR(15), IN p_impact VARCHAR(10), IN p_title VARCHAR(200), IN p_body TEXT, IN p_origin JSON, IN p_refs JSON,
  IN p_msg_uuid CHAR(36), IN p_files JSON, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id, v_msg BIGINT UNSIGNED;
  DECLARE v_number INT UNSIGNED;
  DECLARE EXIT HANDLER FOR SQLEXCEPTION BEGIN ROLLBACK; RESIGNAL; END;
  CALL sp_assert_member(p_actor_id, p_tenant_id);
  IF p_kind NOT IN ('INCIDENT','OBSERVATION','IMPROVEMENT') OR p_impact NOT IN ('LOW','MEDIUM','HIGH')
     OR TRIM(IFNULL(p_title, '')) = '' OR TRIM(IFNULL(p_body, '')) = '' THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION';
  END IF;
  START TRANSACTION;
  SELECT IFNULL(MAX(number), 0) + 1 INTO v_number FROM support_ticket FOR UPDATE;
  INSERT INTO support_ticket (uuid, number, tenant_id, reporter_user_id, kind, impact, title, origin, refs)
  VALUES (p_uuid, v_number, p_tenant_id, p_actor_id, p_kind, p_impact, TRIM(p_title), p_origin, p_refs);
  SET v_id = LAST_INSERT_ID();
  INSERT INTO support_message (uuid, ticket_id, author_id, is_staff, body) VALUES (p_msg_uuid, v_id, p_actor_id, 0, TRIM(p_body));
  SET v_msg = LAST_INSERT_ID();
  CALL sp_support_attach(p_actor_id, v_id, v_msg, p_files);
  COMMIT;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'CREATE', 'support_ticket', p_uuid, JSON_OBJECT('kind', p_kind, 'number', v_number), p_ip);
  SELECT p_uuid AS uuid, v_number AS number;
END $$

-- Respuesta en la conversación. El staff puede cambiar el estado (por defecto, ANSWERED); si responde el autor,
-- una solicitud respondida o cerrada vuelve a OPEN. Devuelve lo necesario para notificar.
DROP PROCEDURE IF EXISTS sp_support_reply $$
CREATE PROCEDURE sp_support_reply(IN p_actor_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_msg_uuid CHAR(36), IN p_body TEXT,
  IN p_status VARCHAR(15), IN p_files JSON, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id, v_msg BIGINT UNSIGNED;
  DECLARE v_staff TINYINT(1);
  DECLARE v_status VARCHAR(15);
  DECLARE v_new_status VARCHAR(15);
  DECLARE EXIT HANDLER FOR SQLEXCEPTION BEGIN ROLLBACK; RESIGNAL; END;
  CALL sp_support_resolve(p_actor_id, p_uuid, v_id, v_staff);
  SELECT status INTO v_status FROM support_ticket WHERE id = v_id;
  IF v_staff = 1 THEN
    SET v_new_status = IFNULL(NULLIF(p_status, ''), 'ANSWERED');
    IF v_new_status NOT IN ('OPEN','IN_PROGRESS','ANSWERED','CLOSED') THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  ELSE
    SET v_new_status = IF(v_status IN ('ANSWERED','CLOSED'), 'OPEN', v_status);
  END IF;
  -- un cambio de estado del staff puede ir sin texto; el autor siempre escribe algo
  IF TRIM(IFNULL(p_body, '')) = '' AND (v_staff = 0 OR v_new_status = v_status) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION';
  END IF;
  START TRANSACTION;
  INSERT INTO support_message (uuid, ticket_id, author_id, is_staff, body, status_to)
  VALUES (p_msg_uuid, v_id, p_actor_id, v_staff, TRIM(IFNULL(p_body, '')), IF(v_new_status <> v_status, v_new_status, NULL));
  SET v_msg = LAST_INSERT_ID();
  CALL sp_support_attach(p_actor_id, v_id, v_msg, p_files);
  UPDATE support_ticket SET status = v_new_status, updated_at = NOW(3),
         last_staff_at = IF(v_staff = 1, NOW(3), last_staff_at),
         closed_at = IF(v_new_status = 'CLOSED', NOW(3), NULL)
   WHERE id = v_id;
  COMMIT;
  CALL sp_audit_insert(NULL, p_actor_id, NULL, 'REPLY', 'support_ticket', p_uuid, JSON_OBJECT('status', v_new_status, 'staff', v_staff), p_ip);
  SELECT t.uuid, t.number, t.title, t.status, v_staff AS is_staff, t.tenant_id, tn.slug, ru.id AS reporter_id
    FROM support_ticket t JOIN tenant tn ON tn.id = t.tenant_id JOIN app_user ru ON ru.id = t.reporter_user_id
   WHERE t.id = v_id;
END $$

-- Listado: el autor ve las suyas; el staff, todas (con filtro de estado y texto).
DROP PROCEDURE IF EXISTS sp_support_list $$
CREATE PROCEDURE sp_support_list(IN p_actor_id BIGINT UNSIGNED, IN p_status VARCHAR(15), IN p_search VARCHAR(100), IN p_limit INT)
BEGIN
  DECLARE v_staff TINYINT(1);
  DECLARE v_search VARCHAR(110);
  DECLARE v_limit INT DEFAULT LEAST(GREATEST(IFNULL(p_limit, 100), 1), 500);
  SET v_staff = EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'platform.support.manage');
  SET v_search = IF(TRIM(IFNULL(p_search, '')) = '', NULL, CONCAT('%', LOWER(TRIM(p_search)), '%'));
  SELECT JSON_OBJECT('id', t.uuid, 'number', t.number, 'kind', t.kind, 'impact', t.impact, 'title', t.title, 'status', t.status,
           'createdAt', t.created_at, 'updatedAt', t.updated_at, 'lastStaffAt', t.last_staff_at, 'origin', t.origin,
           'tenant', JSON_OBJECT('slug', tn.slug, 'name', tn.name), 'reporterNameEnc', TO_BASE64(ru.full_name_enc),
           'messages', (SELECT COUNT(*) FROM support_message m WHERE m.ticket_id = t.id),
           'attachments', (SELECT COUNT(*) FROM support_attachment a WHERE a.ticket_id = t.id)) AS doc
    FROM support_ticket t
    JOIN tenant tn ON tn.id = t.tenant_id
    JOIN app_user ru ON ru.id = t.reporter_user_id
   WHERE (v_staff = 1 OR t.reporter_user_id = p_actor_id)
     AND (p_status IS NULL OR p_status = '' OR t.status = p_status OR (p_status = 'ACTIVE' AND t.status <> 'CLOSED'))
     AND (v_search IS NULL OR LOWER(t.title) LIKE v_search OR CAST(t.number AS CHAR) = TRIM(p_search) OR LOWER(tn.name) LIKE v_search)
   ORDER BY FIELD(t.status, 'OPEN', 'IN_PROGRESS', 'ANSWERED', 'CLOSED'), t.updated_at DESC
   LIMIT v_limit;
END $$

-- Detalle con la conversación y sus adjuntos
DROP PROCEDURE IF EXISTS sp_support_get $$
CREATE PROCEDURE sp_support_get(IN p_actor_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_staff TINYINT(1);
  CALL sp_support_resolve(p_actor_id, p_uuid, v_id, v_staff);
  SELECT JSON_OBJECT('id', t.uuid, 'number', t.number, 'kind', t.kind, 'impact', t.impact, 'title', t.title, 'status', t.status,
           'createdAt', t.created_at, 'updatedAt', t.updated_at, 'closedAt', t.closed_at, 'origin', t.origin, 'refs', t.refs,
           'canManage', v_staff = 1,
           'tenant', JSON_OBJECT('slug', tn.slug, 'name', tn.name),
           'reporter', JSON_OBJECT('id', ru.uuid, 'nameEnc', TO_BASE64(ru.full_name_enc)),
           'messages', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', m.uuid, 'isStaff', m.is_staff = 1, 'body', m.body, 'statusTo', m.status_to,
                          'at', m.created_at, 'authorNameEnc', TO_BASE64(au.full_name_enc),
                          'attachments', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', f.uuid, 'name', f.original_name, 'mimeType', f.mime_type, 'size', f.size_bytes))
                                                   FROM support_attachment a JOIN file_object f ON f.id = a.file_id WHERE a.message_id = m.id), JSON_ARRAY())))
                        FROM support_message m JOIN app_user au ON au.id = m.author_id WHERE m.ticket_id = t.id), JSON_ARRAY())
         ) AS doc
    FROM support_ticket t
    JOIN tenant tn ON tn.id = t.tenant_id
    JOIN app_user ru ON ru.id = t.reporter_user_id
   WHERE t.id = v_id;
END $$

-- Archivo de una solicitud (solo su autor o el staff)
DROP PROCEDURE IF EXISTS sp_support_file_get $$
CREATE PROCEDURE sp_support_file_get(IN p_actor_id BIGINT UNSIGNED, IN p_file_uuid CHAR(36))
BEGIN
  SELECT f.id, f.uuid, f.tenant_id, f.owner_type, f.category, f.storage_key, f.original_name, f.mime_type, f.size_bytes, f.sha256, f.is_encrypted
    FROM file_object f
    JOIN support_attachment a ON a.file_id = f.id
    JOIN support_ticket t ON t.id = a.ticket_id
   WHERE f.uuid = p_file_uuid AND f.deleted_at IS NULL
     AND (t.reporter_user_id = p_actor_id
          OR EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'platform.support.manage'));
END $$

-- ---------------- Novedades ----------------
DROP PROCEDURE IF EXISTS sp_release_publish $$
CREATE PROCEDURE sp_release_publish(IN p_actor_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_title JSON, IN p_body JSON,
  IN p_module VARCHAR(40), IN p_tour VARCHAR(40), IN p_ip VARCHAR(45))
BEGIN
  CALL sp_assert_actor(p_actor_id, NULL, 'platform.releases.manage');
  INSERT INTO release_note (uuid, title_i18n, body_i18n, module, tour_key, created_by)
  VALUES (p_uuid, p_title, p_body, NULLIF(p_module, ''), NULLIF(p_tour, ''), p_actor_id);
  -- la guía relacionada se vuelve a mostrar a todos
  IF NULLIF(p_tour, '') IS NOT NULL THEN
    DELETE FROM user_tour WHERE tour_key = p_tour;
  END IF;
  CALL sp_audit_insert(NULL, p_actor_id, NULL, 'PUBLISH', 'release_note', p_uuid, JSON_OBJECT('module', p_module), p_ip);
END $$

DROP PROCEDURE IF EXISTS sp_release_list $$
CREATE PROCEDURE sp_release_list(IN p_actor_id BIGINT UNSIGNED, IN p_limit INT)
BEGIN
  DECLARE v_limit INT DEFAULT LEAST(GREATEST(IFNULL(p_limit, 30), 1), 100);
  IF NOT EXISTS (SELECT 1 FROM app_user WHERE id = p_actor_id AND status = 'ACTIVE' AND deleted_at IS NULL) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FORBIDDEN';
  END IF;
  SELECT JSON_OBJECT('id', r.uuid, 'title', r.title_i18n, 'body', r.body_i18n, 'module', r.module, 'tourKey', r.tour_key,
           'publishedAt', r.published_at) AS doc
    FROM release_note r ORDER BY r.published_at DESC LIMIT v_limit;
END $$

-- ---------------- Guías de primer uso ----------------
DROP PROCEDURE IF EXISTS sp_tours_get $$
CREATE PROCEDURE sp_tours_get(IN p_actor_id BIGINT UNSIGNED)
BEGIN
  SELECT tour_key, version FROM user_tour WHERE user_id = p_actor_id;
END $$

DROP PROCEDURE IF EXISTS sp_tour_seen $$
CREATE PROCEDURE sp_tour_seen(IN p_actor_id BIGINT UNSIGNED, IN p_key VARCHAR(40), IN p_version INT)
BEGIN
  INSERT INTO user_tour (user_id, tour_key, version) VALUES (p_actor_id, p_key, p_version)
  ON DUPLICATE KEY UPDATE version = GREATEST(version, VALUES(version)), seen_at = NOW(3);
END $$

DELIMITER ;
