-- =====================================================================
-- Favoritos: colecciones por usuario, compartibles con usuarios del mismo tenant
-- Dueño: renombra, elimina, comparte. Compartido VIEW: ve. EDIT: agrega/quita talentos.
-- =====================================================================

DELIMITER $$

-- Resuelve una colección visible para el actor y su nivel de acceso: OWNER | EDIT | VIEW
DROP PROCEDURE IF EXISTS sp_collection_access $$
CREATE PROCEDURE sp_collection_access(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  OUT o_id BIGINT UNSIGNED, OUT o_access VARCHAR(10))
BEGIN
  DECLARE v_owner BIGINT UNSIGNED;
  SET o_id = NULL, o_access = NULL;
  SELECT id, user_id INTO o_id, v_owner FROM favorite_collection
   WHERE uuid = p_uuid AND tenant_id = p_tenant_id AND deleted_at IS NULL;
  IF o_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  IF v_owner = p_actor_id THEN
    SET o_access = 'OWNER';
  ELSE
    SELECT permission INTO o_access FROM favorite_collection_share WHERE collection_id = o_id AND user_id = p_actor_id;
    IF o_access IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  END IF;
END $$

DROP PROCEDURE IF EXISTS sp_collection_list $$
CREATE PROCEDURE sp_collection_list(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  SELECT c.uuid, c.name,
         (SELECT COUNT(*) FROM favorite_collection_talent ct JOIN talent t ON t.id = ct.talent_id AND t.deleted_at IS NULL
           WHERE ct.collection_id = c.id) AS talent_count,
         IF(c.user_id = p_actor_id, 'OWNER', sh.permission) AS access,
         u.uuid AS owner_uuid, u.full_name_enc AS owner_name_enc,
         (SELECT COUNT(*) FROM favorite_collection_share x WHERE x.collection_id = c.id) AS share_count,
         IFNULL(c.updated_at, c.created_at) AS updated_at
    FROM favorite_collection c
    JOIN app_user u ON u.id = c.user_id
    LEFT JOIN favorite_collection_share sh ON sh.collection_id = c.id AND sh.user_id = p_actor_id
   WHERE c.tenant_id = p_tenant_id AND c.deleted_at IS NULL
     AND (c.user_id = p_actor_id OR sh.user_id IS NOT NULL)
   ORDER BY c.user_id <> p_actor_id, c.name;
END $$

DROP PROCEDURE IF EXISTS sp_collection_create $$
CREATE PROCEDURE sp_collection_create(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_name VARCHAR(100))
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  SET p_name = TRIM(p_name);
  IF p_name = '' OR CHAR_LENGTH(p_name) > 100 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  IF NOT EXISTS (SELECT 1 FROM favorite_collection WHERE uuid = p_uuid) THEN
    IF EXISTS (SELECT 1 FROM favorite_collection WHERE user_id = p_actor_id AND name = p_name AND deleted_at IS NULL) THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:COLLECTION_EXISTS';
    END IF;
    INSERT INTO favorite_collection (uuid, tenant_id, user_id, name, created_by) VALUES (p_uuid, p_tenant_id, p_actor_id, p_name, p_actor_id);
  END IF;
  CALL sp_collection_list(p_actor_id, p_tenant_id);
END $$

DROP PROCEDURE IF EXISTS sp_collection_rename $$
CREATE PROCEDURE sp_collection_rename(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_name VARCHAR(100))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_access VARCHAR(10);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  CALL sp_collection_access(p_actor_id, p_tenant_id, p_uuid, v_id, v_access);
  IF v_access <> 'OWNER' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FORBIDDEN'; END IF;
  SET p_name = TRIM(p_name);
  IF p_name = '' OR CHAR_LENGTH(p_name) > 100 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  IF EXISTS (SELECT 1 FROM favorite_collection WHERE user_id = p_actor_id AND name = p_name AND deleted_at IS NULL AND id <> v_id) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:COLLECTION_EXISTS';
  END IF;
  UPDATE favorite_collection SET name = p_name, updated_by = p_actor_id WHERE id = v_id;
END $$

DROP PROCEDURE IF EXISTS sp_collection_delete $$
CREATE PROCEDURE sp_collection_delete(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_access VARCHAR(10);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  CALL sp_collection_access(p_actor_id, p_tenant_id, p_uuid, v_id, v_access);
  IF v_access = 'OWNER' THEN
    UPDATE favorite_collection SET deleted_at = NOW(3), deleted_by = p_actor_id WHERE id = v_id;
    DELETE FROM favorite_collection_share WHERE collection_id = v_id;
  ELSE
    -- un usuario con quien se compartió solo puede "salir" de la colección
    DELETE FROM favorite_collection_share WHERE collection_id = v_id AND user_id = p_actor_id;
  END IF;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, IF(v_access = 'OWNER', 'DELETE', 'LEAVE'), 'favorite_collection', p_uuid, NULL, NULL);
END $$

DROP PROCEDURE IF EXISTS sp_collection_set_talent $$
CREATE PROCEDURE sp_collection_set_talent(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_collection_uuid CHAR(36), IN p_talent_uuid CHAR(36), IN p_in TINYINT(1))
BEGIN
  DECLARE v_coll BIGINT UNSIGNED;
  DECLARE v_access VARCHAR(10);
  DECLARE v_talent BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  CALL sp_talent_resolve(p_tenant_id, p_talent_uuid, v_talent);
  CALL sp_collection_access(p_actor_id, p_tenant_id, p_collection_uuid, v_coll, v_access);
  IF v_access = 'VIEW' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FORBIDDEN'; END IF;
  IF p_in = 1 THEN
    INSERT IGNORE INTO favorite_collection_talent (collection_id, talent_id, created_by) VALUES (v_coll, v_talent, p_actor_id);
  ELSE
    DELETE FROM favorite_collection_talent WHERE collection_id = v_coll AND talent_id = v_talent;
  END IF;
  UPDATE favorite_collection SET updated_by = p_actor_id WHERE id = v_coll;
END $$

-- Con quién está compartida (solo el dueño lo ve)
DROP PROCEDURE IF EXISTS sp_collection_shares $$
CREATE PROCEDURE sp_collection_shares(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_access VARCHAR(10);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  CALL sp_collection_access(p_actor_id, p_tenant_id, p_uuid, v_id, v_access);
  IF v_access <> 'OWNER' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FORBIDDEN'; END IF;
  SELECT u.uuid, u.full_name_enc, u.email_enc, s.permission, s.created_at
    FROM favorite_collection_share s JOIN app_user u ON u.id = s.user_id
   WHERE s.collection_id = v_id ORDER BY s.created_at;
END $$

DROP PROCEDURE IF EXISTS sp_collection_share $$
CREATE PROCEDURE sp_collection_share(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_user_uuid CHAR(36), IN p_permission VARCHAR(10), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_access VARCHAR(10);
  DECLARE v_user BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  CALL sp_collection_access(p_actor_id, p_tenant_id, p_uuid, v_id, v_access);
  IF v_access <> 'OWNER' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FORBIDDEN'; END IF;
  IF p_permission NOT IN ('VIEW','EDIT') THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  -- destinatario: mismo tenant, activo y con acceso a talentos
  SELECT u.id INTO v_user FROM app_user u
   WHERE u.uuid = p_user_uuid AND u.tenant_id = p_tenant_id AND u.status = 'ACTIVE' AND u.deleted_at IS NULL
     AND EXISTS (SELECT 1 FROM v_user_permission vp WHERE vp.user_id = u.id AND vp.code = 'talent.read');
  IF v_user IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:SHARE_USER_INVALID'; END IF;
  IF v_user = p_actor_id THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:SHARE_USER_INVALID'; END IF;
  INSERT INTO favorite_collection_share (collection_id, user_id, permission, created_by)
  VALUES (v_id, v_user, p_permission, p_actor_id)
  ON DUPLICATE KEY UPDATE permission = VALUES(permission), updated_by = p_actor_id;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'SHARE', 'favorite_collection', p_uuid, JSON_OBJECT('user', p_user_uuid, 'permission', p_permission), p_ip);
  CALL sp_collection_shares(p_actor_id, p_tenant_id, p_uuid);
END $$

DROP PROCEDURE IF EXISTS sp_collection_unshare $$
CREATE PROCEDURE sp_collection_unshare(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_user_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_access VARCHAR(10);
  DECLARE v_user BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  CALL sp_collection_access(p_actor_id, p_tenant_id, p_uuid, v_id, v_access);
  SELECT id INTO v_user FROM app_user WHERE uuid = p_user_uuid AND tenant_id = p_tenant_id;
  -- el dueño quita a cualquiera; los demás solo a sí mismos
  IF v_access <> 'OWNER' AND (v_user IS NULL OR v_user <> p_actor_id) THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FORBIDDEN'; END IF;
  DELETE FROM favorite_collection_share WHERE collection_id = v_id AND user_id = v_user;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'UNSHARE', 'favorite_collection', p_uuid, JSON_OBJECT('user', p_user_uuid), p_ip);
END $$

-- Directorio de compañeros para compartir (usuarios activos del tenant con acceso a talentos)
DROP PROCEDURE IF EXISTS sp_tenant_colleagues $$
CREATE PROCEDURE sp_tenant_colleagues(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  SELECT u.uuid, u.full_name_enc, u.email_enc
    FROM app_user u
   WHERE u.tenant_id = p_tenant_id AND u.status = 'ACTIVE' AND u.deleted_at IS NULL AND u.id <> p_actor_id
     AND EXISTS (SELECT 1 FROM v_user_permission vp WHERE vp.user_id = u.id AND vp.code = 'talent.read')
   ORDER BY u.id LIMIT 500;
END $$

DELIMITER ;
