-- =====================================================================
-- Lista negra de talentos
-- client NULL = restricción GLOBAL (todos los clientes); prevalece sobre las por cliente.
-- Cada movimiento queda en talent_blacklist_history con su motivo.
-- =====================================================================

DELIMITER $$

DROP PROCEDURE IF EXISTS sp_blacklist_history_add $$
CREATE PROCEDURE sp_blacklist_history_add(IN p_bl_id BIGINT UNSIGNED, IN p_movement VARCHAR(10), IN p_reason VARCHAR(1000), IN p_actor_id BIGINT UNSIGNED)
BEGIN
  INSERT INTO talent_blacklist_history (tenant_id, blacklist_id, talent_id, client_item_id, movement, reason, user_id)
  SELECT tenant_id, id, talent_id, client_item_id, p_movement, p_reason, p_actor_id FROM talent_blacklist WHERE id = p_bl_id;
END $$

-- ¿Está bloqueado el talento para un cliente? (la global prevalece)
DROP PROCEDURE IF EXISTS sp_blacklist_check $$
CREATE PROCEDURE sp_blacklist_check(IN p_talent_id BIGINT UNSIGNED, IN p_client_id BIGINT UNSIGNED, OUT o_blocked TINYINT(1))
BEGIN
  SET o_blocked = EXISTS (SELECT 1 FROM talent_blacklist WHERE talent_id = p_talent_id AND deleted_at IS NULL
                           AND (client_item_id IS NULL OR client_item_id = p_client_id));
END $$

DROP PROCEDURE IF EXISTS sp_blacklist_create $$
CREATE PROCEDURE sp_blacklist_create(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_talent_uuid CHAR(36), IN p_client_uuid CHAR(36), IN p_reason VARCHAR(1000), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_talent BIGINT UNSIGNED;
  DECLARE v_client BIGINT UNSIGNED;
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE done INT DEFAULT 0;
  DECLARE v_old BIGINT UNSIGNED;
  DECLARE v_msg VARCHAR(64);
  DECLARE cur CURSOR FOR SELECT id FROM talent_blacklist WHERE talent_id = v_talent AND deleted_at IS NULL AND client_item_id IS NOT NULL;
  DECLARE CONTINUE HANDLER FOR NOT FOUND SET done = 1;
  DECLARE EXIT HANDLER FOR SQLEXCEPTION BEGIN ROLLBACK; RESIGNAL; END;

  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'blacklist.manage');
  CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent);
  CALL sp_master_resolve(p_tenant_id, 'CLIENT', p_client_uuid, v_client);
  SET p_reason = TRIM(IFNULL(p_reason, ''));
  IF p_reason = '' OR CHAR_LENGTH(p_reason) > 1000 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:REASON_REQUIRED'; END IF;
  -- idempotencia: el mismo uuid reenviado no crea otra restricción
  IF NOT EXISTS (SELECT 1 FROM talent_blacklist WHERE uuid = p_uuid) THEN
    START TRANSACTION;
    IF EXISTS (SELECT 1 FROM talent_blacklist WHERE talent_id = v_talent AND deleted_at IS NULL AND client_item_id IS NULL) THEN
      SET v_msg = IF(v_client IS NULL, 'BDT:BLACKLIST_EXISTS', 'BDT:BLACKLIST_GLOBAL_EXISTS');
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = v_msg;
    END IF;
    IF v_client IS NOT NULL AND EXISTS (SELECT 1 FROM talent_blacklist WHERE talent_id = v_talent AND deleted_at IS NULL AND client_item_id = v_client) THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:BLACKLIST_EXISTS';
    END IF;
    IF v_client IS NULL THEN
      -- la global absorbe las restricciones por cliente (quedan en el historial)
      OPEN cur;
      lp: LOOP
        FETCH cur INTO v_old;
        IF done = 1 THEN LEAVE lp; END IF;
        UPDATE talent_blacklist SET deleted_at = NOW(3), deleted_by = p_actor_id WHERE id = v_old;
        CALL sp_blacklist_history_add(v_old, 'DELETE', CONCAT('Reemplazada por restricción global: ', LEFT(p_reason, 950)), p_actor_id);
      END LOOP;
      CLOSE cur;
    END IF;
    INSERT INTO talent_blacklist (uuid, tenant_id, talent_id, client_item_id, reason, created_by)
    VALUES (p_uuid, p_tenant_id, v_talent, v_client, p_reason, p_actor_id);
    SET v_id = LAST_INSERT_ID();
    CALL sp_blacklist_history_add(v_id, 'CREATE', p_reason, p_actor_id);
    COMMIT;
    CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'CREATE', 'talent_blacklist', p_uuid, JSON_OBJECT('talent', p_talent_uuid, 'client', p_client_uuid), p_ip);
  END IF;
  CALL sp_blacklist_status(p_actor_id, p_tenant_id, p_talent_uuid);
END $$

DROP PROCEDURE IF EXISTS sp_blacklist_resolve $$
CREATE PROCEDURE sp_blacklist_resolve(IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), OUT o_id BIGINT UNSIGNED, OUT o_talent_uuid CHAR(36), OUT o_is_global TINYINT(1))
BEGIN
  SET o_id = NULL;
  SELECT b.id, t.uuid, b.client_item_id IS NULL INTO o_id, o_talent_uuid, o_is_global
    FROM talent_blacklist b JOIN talent t ON t.id = b.talent_id
   WHERE b.uuid = p_uuid AND b.tenant_id = p_tenant_id AND b.deleted_at IS NULL;
  IF o_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
END $$

-- Solo cambia el motivo (el cliente es inmutable)
DROP PROCEDURE IF EXISTS sp_blacklist_update $$
CREATE PROCEDURE sp_blacklist_update(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_reason VARCHAR(1000), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_talent CHAR(36);
  DECLARE v_global TINYINT(1);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'blacklist.manage');
  CALL sp_blacklist_resolve(p_tenant_id, p_uuid, v_id, v_talent, v_global);
  SET p_reason = TRIM(IFNULL(p_reason, ''));
  IF p_reason = '' OR CHAR_LENGTH(p_reason) > 1000 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:REASON_REQUIRED'; END IF;
  UPDATE talent_blacklist SET reason = p_reason, updated_by = p_actor_id WHERE id = v_id;
  CALL sp_blacklist_history_add(v_id, 'UPDATE', p_reason, p_actor_id);
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'UPDATE', 'talent_blacklist', p_uuid, NULL, p_ip);
  CALL sp_blacklist_status(p_actor_id, p_tenant_id, v_talent);
END $$

-- Quita una restricción; el motivo de la baja queda en el historial
DROP PROCEDURE IF EXISTS sp_blacklist_remove $$
CREATE PROCEDURE sp_blacklist_remove(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_reason VARCHAR(1000), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_talent CHAR(36);
  DECLARE v_global TINYINT(1);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'blacklist.manage');
  CALL sp_blacklist_resolve(p_tenant_id, p_uuid, v_id, v_talent, v_global);
  SET p_reason = TRIM(IFNULL(p_reason, ''));
  IF p_reason = '' OR CHAR_LENGTH(p_reason) > 1000 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:REASON_REQUIRED'; END IF;
  UPDATE talent_blacklist SET deleted_at = NOW(3), deleted_by = p_actor_id WHERE id = v_id;
  CALL sp_blacklist_history_add(v_id, 'DELETE', p_reason, p_actor_id);
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'DELETE', 'talent_blacklist', p_uuid, NULL, p_ip);
  CALL sp_blacklist_status(p_actor_id, p_tenant_id, v_talent);
END $$

-- Levanta la restricción global conservando la de algunos clientes, de forma ATÓMICA.
-- p_keep = [{"client":"uuid","reason":"..."}]  (vacío = el talento sale de la lista negra)
DROP PROCEDURE IF EXISTS sp_blacklist_lift_global $$
CREATE PROCEDURE sp_blacklist_lift_global(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_reason VARCHAR(1000), IN p_keep JSON, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_talent CHAR(36);
  DECLARE v_talent_id BIGINT UNSIGNED;
  DECLARE v_global TINYINT(1);
  DECLARE v_bad INT;
  DECLARE v_kept INT;
  DECLARE EXIT HANDLER FOR SQLEXCEPTION BEGIN ROLLBACK; RESIGNAL; END;

  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'blacklist.manage');
  CALL sp_blacklist_resolve(p_tenant_id, p_uuid, v_id, v_talent, v_global);
  IF v_global = 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:BLACKLIST_NOT_GLOBAL'; END IF;
  SET p_reason = TRIM(IFNULL(p_reason, ''));
  IF p_reason = '' OR CHAR_LENGTH(p_reason) > 1000 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:REASON_REQUIRED'; END IF;
  SET p_keep = IFNULL(p_keep, JSON_ARRAY());
  -- todos los clientes a conservar deben existir en el tenant y traer motivo
  SELECT COUNT(*) INTO v_bad FROM JSON_TABLE(p_keep, '$[*]' COLUMNS(client CHAR(36) PATH '$.client', reason VARCHAR(1000) PATH '$.reason')) k
   LEFT JOIN master_item i ON i.uuid = k.client AND i.deleted_at IS NULL AND i.tenant_id = p_tenant_id
   LEFT JOIN master_table m ON m.id = i.master_table_id AND m.code = 'CLIENT'
   WHERE m.id IS NULL OR TRIM(IFNULL(k.reason, '')) = '';
  IF v_bad > 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:BLACKLIST_KEEP_INVALID'; END IF;

  SELECT talent_id INTO v_talent_id FROM talent_blacklist WHERE id = v_id;
  START TRANSACTION;
  UPDATE talent_blacklist SET deleted_at = NOW(3), deleted_by = p_actor_id WHERE id = v_id;
  CALL sp_blacklist_history_add(v_id, 'DELETE', p_reason, p_actor_id);
  INSERT INTO talent_blacklist (uuid, tenant_id, talent_id, client_item_id, reason, created_by)
  SELECT UUID(), p_tenant_id, v_talent_id, i.id, MIN(TRIM(k.reason)), p_actor_id
    FROM JSON_TABLE(p_keep, '$[*]' COLUMNS(client CHAR(36) PATH '$.client', reason VARCHAR(1000) PATH '$.reason')) k
    JOIN master_item i ON i.uuid = k.client AND i.tenant_id = p_tenant_id
   GROUP BY i.id; -- un cliente repetido se conserva una sola vez
  SET v_kept = ROW_COUNT();
  INSERT INTO talent_blacklist_history (tenant_id, blacklist_id, talent_id, client_item_id, movement, reason, user_id)
  SELECT b.tenant_id, b.id, b.talent_id, b.client_item_id, 'CREATE', b.reason, p_actor_id
    FROM talent_blacklist b
   WHERE b.talent_id = v_talent_id AND b.deleted_at IS NULL AND b.client_item_id IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM talent_blacklist_history h WHERE h.blacklist_id = b.id);
  COMMIT;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'LIFT_GLOBAL', 'talent_blacklist', p_uuid, JSON_OBJECT('kept', v_kept), p_ip);
  CALL sp_blacklist_status(p_actor_id, p_tenant_id, v_talent);
END $$

-- Restricciones activas de un talento (vacío = no bloqueado)
DROP PROCEDURE IF EXISTS sp_blacklist_status $$
CREATE PROCEDURE sp_blacklist_status(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_talent_uuid CHAR(36))
BEGIN
  DECLARE v_talent BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent);
  SELECT b.uuid, c.uuid AS client_uuid, c.label_i18n AS client, b.reason, IFNULL(b.updated_at, b.created_at) AS changed_at,
         u.full_name_enc AS user_name_enc
    FROM talent_blacklist b
    LEFT JOIN master_item c ON c.id = b.client_item_id
    LEFT JOIN app_user u ON u.id = IFNULL(b.updated_by, b.created_by)
   WHERE b.talent_id = v_talent AND b.deleted_at IS NULL
   ORDER BY b.client_item_id IS NOT NULL, b.created_at;
END $$

DROP PROCEDURE IF EXISTS sp_blacklist_history $$
CREATE PROCEDURE sp_blacklist_history(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_talent_uuid CHAR(36), IN p_page INT, IN p_page_size INT)
BEGIN
  DECLARE v_talent BIGINT UNSIGNED;
  DECLARE v_offset INT;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent);
  SET p_page_size = LEAST(GREATEST(IFNULL(p_page_size, 20), 1), 100);
  SET v_offset = (GREATEST(IFNULL(p_page, 1), 1) - 1) * p_page_size;
  SELECT COUNT(*) AS total FROM talent_blacklist_history WHERE talent_id = v_talent;
  SELECT h.id, h.movement, c.label_i18n AS client, h.reason, h.created_at, u.full_name_enc AS user_name_enc
    FROM talent_blacklist_history h
    LEFT JOIN master_item c ON c.id = h.client_item_id
    LEFT JOIN app_user u ON u.id = h.user_id
   WHERE h.talent_id = v_talent
   ORDER BY h.id DESC LIMIT v_offset, p_page_size;
END $$

-- Listado agrupado por talento (paginación por talento).
-- p_client_uuid: solo talentos restringidos para ese cliente; p_global_only = 1: solo restricción global.
DROP PROCEDURE IF EXISTS sp_blacklist_list $$
CREATE PROCEDURE sp_blacklist_list(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_search VARCHAR(150),
  IN p_client_uuid CHAR(36), IN p_global_only TINYINT(1), IN p_page INT, IN p_page_size INT)
BEGIN
  DECLARE v_client BIGINT UNSIGNED;
  DECLARE v_offset INT;
  DECLARE v_search VARCHAR(160);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  CALL sp_master_resolve(p_tenant_id, 'CLIENT', p_client_uuid, v_client);
  SET p_page_size = LEAST(GREATEST(IFNULL(p_page_size, 20), 1), 100);
  SET v_offset = (GREATEST(IFNULL(p_page, 1), 1) - 1) * p_page_size;
  SET v_search = IF(p_search IS NULL OR TRIM(p_search) = '', NULL, CONCAT('%', LOWER(TRIM(p_search)), '%'));

  DROP TEMPORARY TABLE IF EXISTS tmp_bl_talent;
  CREATE TEMPORARY TABLE tmp_bl_talent (talent_id BIGINT UNSIGNED PRIMARY KEY, last_change DATETIME(3)) ENGINE=MEMORY;
  INSERT INTO tmp_bl_talent (talent_id, last_change)
  SELECT b.talent_id, MAX(IFNULL(b.updated_at, b.created_at))
    FROM talent_blacklist b JOIN talent t ON t.id = b.talent_id AND t.deleted_at IS NULL
   WHERE b.tenant_id = p_tenant_id AND b.deleted_at IS NULL
     AND (v_search IS NULL OR t.search_name LIKE v_search)
     AND (v_client IS NULL OR b.client_item_id = v_client)
     AND (IFNULL(p_global_only, 0) = 0 OR b.client_item_id IS NULL)
   GROUP BY b.talent_id;

  SELECT COUNT(*) AS total FROM tmp_bl_talent;
  SELECT t.uuid, t.first_name, t.last_name, t.second_last_name, t.headline, x.last_change,
         EXISTS (SELECT 1 FROM talent_blacklist g WHERE g.talent_id = t.id AND g.deleted_at IS NULL AND g.client_item_id IS NULL) AS is_global,
         (SELECT COUNT(*) FROM talent_blacklist c WHERE c.talent_id = t.id AND c.deleted_at IS NULL AND c.client_item_id IS NOT NULL) AS client_count
    FROM tmp_bl_talent x JOIN talent t ON t.id = x.talent_id
   ORDER BY x.last_change DESC, t.id DESC
   LIMIT v_offset, p_page_size;
  DROP TEMPORARY TABLE IF EXISTS tmp_bl_talent;
END $$

DELIMITER ;
