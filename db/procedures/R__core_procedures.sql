-- =====================================================================
-- Procedimientos del núcleo (migración repetible: se reaplica si cambia)
-- Errores de negocio: SIGNAL SQLSTATE '45000' con MESSAGE_TEXT 'BDT:<CODIGO>'
-- La aplicación traduce el código a un mensaje i18n.
-- =====================================================================

DELIMITER $$

-- ---------------------------------------------------------------------
-- Utilidades
-- ---------------------------------------------------------------------
DROP FUNCTION IF EXISTS fn_has_permission $$
DROP FUNCTION IF EXISTS fn_user_tenant $$

-- Exige permiso y que el actor pertenezca al tenant (o sea de plataforma)
DROP PROCEDURE IF EXISTS sp_assert_actor $$
CREATE PROCEDURE sp_assert_actor(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_perm VARCHAR(80))
BEGIN
  DECLARE v_actor_tenant BIGINT UNSIGNED;
  IF p_actor_id IS NULL OR NOT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = p_perm) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FORBIDDEN';
  END IF;
  SET v_actor_tenant = (SELECT tenant_id FROM app_user WHERE id = p_actor_id);
  IF v_actor_tenant IS NOT NULL AND (p_tenant_id IS NULL OR v_actor_tenant <> p_tenant_id) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TENANT_MISMATCH';
  END IF;
END $$

DROP PROCEDURE IF EXISTS sp_audit_insert $$
CREATE PROCEDURE sp_audit_insert(IN p_tenant_id BIGINT UNSIGNED, IN p_user_id BIGINT UNSIGNED,
  IN p_api_key_id BIGINT UNSIGNED, IN p_action VARCHAR(60), IN p_entity VARCHAR(60),
  IN p_entity_id VARCHAR(64), IN p_detail JSON, IN p_ip VARCHAR(45))
BEGIN
  INSERT INTO audit_log (tenant_id, user_id, api_key_id, action, entity, entity_id, detail, ip)
  VALUES (p_tenant_id, p_user_id, p_api_key_id, p_action, p_entity, p_entity_id, p_detail, p_ip);
END $$

-- ---------------------------------------------------------------------
-- Tenants
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_tenant_create $$
CREATE PROCEDURE sp_tenant_create(IN p_actor_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_slug VARCHAR(60),
  IN p_name VARCHAR(150), IN p_legal_name VARCHAR(250), IN p_tax_id VARCHAR(30),
  IN p_locale VARCHAR(10), IN p_timezone VARCHAR(60), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, NULL, 'platform.tenants.manage');
  IF p_slug NOT REGEXP '^[a-z0-9]([a-z0-9-]{1,58}[a-z0-9])$' THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TENANT_SLUG_INVALID';
  END IF;
  IF EXISTS (SELECT 1 FROM tenant WHERE slug = p_slug) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TENANT_SLUG_TAKEN';
  END IF;
  INSERT INTO tenant (uuid, slug, name, legal_name, tax_id, default_locale, timezone, created_by)
  VALUES (p_uuid, p_slug, p_name, p_legal_name, p_tax_id, IFNULL(p_locale,'es'), IFNULL(p_timezone,'America/Lima'), p_actor_id);
  SET v_id = LAST_INSERT_ID();
  INSERT INTO tenant_branding (tenant_id, display_name, created_by) VALUES (v_id, p_name, p_actor_id);
  CALL sp_audit_insert(v_id, p_actor_id, NULL, 'CREATE', 'tenant', p_uuid, JSON_OBJECT('slug', p_slug), p_ip);
  CALL sp_tenant_get(v_id);
END $$

DROP PROCEDURE IF EXISTS sp_tenant_get $$
CREATE PROCEDURE sp_tenant_get(IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  SELECT t.id, t.uuid, t.slug, t.name, t.legal_name, t.tax_id, t.status, t.default_locale, t.timezone,
         t.created_at,
         (SELECT COUNT(*) FROM app_user u WHERE u.tenant_id = t.id AND u.deleted_at IS NULL) AS user_count
  FROM tenant t WHERE t.id = p_tenant_id AND t.deleted_at IS NULL;
END $$

DROP PROCEDURE IF EXISTS sp_tenant_list $$
CREATE PROCEDURE sp_tenant_list(IN p_actor_id BIGINT UNSIGNED, IN p_search VARCHAR(100),
  IN p_status VARCHAR(20), IN p_page INT, IN p_page_size INT)
BEGIN
  DECLARE v_offset INT;
  CALL sp_assert_actor(p_actor_id, NULL, 'platform.tenants.manage');
  SET p_page_size = LEAST(GREATEST(IFNULL(p_page_size, 20), 1), 100);
  SET v_offset = (GREATEST(IFNULL(p_page, 1), 1) - 1) * p_page_size;
  SELECT COUNT(*) AS total FROM tenant t
   WHERE t.deleted_at IS NULL
     AND (p_status IS NULL OR t.status = p_status)
     AND (p_search IS NULL OR t.name LIKE CONCAT('%', p_search, '%') OR t.slug LIKE CONCAT('%', p_search, '%'));
  SELECT t.id, t.uuid, t.slug, t.name, t.legal_name, t.tax_id, t.status, t.default_locale, t.timezone, t.created_at,
         (SELECT COUNT(*) FROM app_user u WHERE u.tenant_id = t.id AND u.deleted_at IS NULL) AS user_count
    FROM tenant t
   WHERE t.deleted_at IS NULL
     AND (p_status IS NULL OR t.status = p_status)
     AND (p_search IS NULL OR t.name LIKE CONCAT('%', p_search, '%') OR t.slug LIKE CONCAT('%', p_search, '%'))
   ORDER BY t.name
   LIMIT v_offset, p_page_size;
END $$

DROP PROCEDURE IF EXISTS sp_tenant_set_status $$
CREATE PROCEDURE sp_tenant_set_status(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_uuid CHAR(36),
  IN p_status VARCHAR(20), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, NULL, 'platform.tenants.manage');
  SELECT id INTO v_id FROM tenant WHERE uuid = p_tenant_uuid AND deleted_at IS NULL;
  IF v_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  IF p_status NOT IN ('ACTIVE','SUSPENDED','CANCELLED') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_STATUS';
  END IF;
  UPDATE tenant SET status = p_status, updated_by = p_actor_id WHERE id = v_id;
  -- Al suspender o cancelar se cierran todas las sesiones y se apagan las API keys
  IF p_status <> 'ACTIVE' THEN
    UPDATE user_session s JOIN app_user u ON u.id = s.user_id
       SET s.revoked_at = NOW(3) WHERE u.tenant_id = v_id AND s.revoked_at IS NULL;
    UPDATE api_key SET status = 'DISABLED', updated_by = p_actor_id WHERE tenant_id = v_id AND status = 'ENABLED';
  END IF;
  CALL sp_audit_insert(v_id, p_actor_id, NULL, 'SET_STATUS', 'tenant', p_tenant_uuid, JSON_OBJECT('status', p_status), p_ip);
  CALL sp_tenant_get(v_id);
END $$

-- Traduce el uuid público de un tenant a su id interno (uso exclusivo de plataforma)
DROP PROCEDURE IF EXISTS sp_tenant_resolve $$
CREATE PROCEDURE sp_tenant_resolve(IN p_actor_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  CALL sp_assert_actor(p_actor_id, NULL, 'platform.tenants.manage');
  SELECT id, slug, name FROM tenant WHERE uuid = p_uuid AND deleted_at IS NULL;
END $$

-- Datos públicos para pintar la pantalla de login de un tenant
DROP PROCEDURE IF EXISTS sp_tenant_public_get $$
CREATE PROCEDURE sp_tenant_public_get(IN p_slug VARCHAR(60))
BEGIN
  SELECT t.id, t.uuid, t.slug, t.name, t.status, t.default_locale,
         b.display_name, b.logo_url, lf.uuid AS logo_file_uuid, ldf.uuid AS logo_dark_file_uuid,
         ff.uuid AS favicon_file_uuid,
         b.color_primary, b.color_primary_hover, b.color_secondary, b.color_accent, b.color_sidebar,
         b.font_family, IFNULL(b.updated_at, b.created_at) AS branding_version
    FROM tenant t
    JOIN tenant_branding b ON b.tenant_id = t.id
    LEFT JOIN file_object lf  ON lf.id = b.logo_file_id AND lf.deleted_at IS NULL
    LEFT JOIN file_object ldf ON ldf.id = b.logo_dark_file_id AND ldf.deleted_at IS NULL
    LEFT JOIN file_object ff  ON ff.id = b.favicon_file_id AND ff.deleted_at IS NULL
   WHERE t.slug = p_slug AND t.deleted_at IS NULL AND t.status = 'ACTIVE';
END $$

-- ---------------------------------------------------------------------
-- Branding
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_branding_update $$
CREATE PROCEDURE sp_branding_update(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_display_name VARCHAR(150), IN p_primary CHAR(7), IN p_primary_hover CHAR(7), IN p_secondary CHAR(7),
  IN p_accent CHAR(7), IN p_sidebar CHAR(7), IN p_font VARCHAR(80), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_slug VARCHAR(60);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.branding.manage');
  IF p_font NOT IN ('Inter','Roboto','Open Sans','Montserrat','Poppins','Lato','Nunito','Source Sans 3') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FONT_NOT_ALLOWED';
  END IF;
  UPDATE tenant_branding
     SET display_name = p_display_name, color_primary = UPPER(p_primary), color_primary_hover = UPPER(p_primary_hover),
         color_secondary = UPPER(p_secondary), color_accent = UPPER(p_accent), color_sidebar = UPPER(p_sidebar),
         font_family = p_font, updated_by = p_actor_id
   WHERE tenant_id = p_tenant_id;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'UPDATE', 'tenant_branding', p_tenant_id,
       JSON_OBJECT('primary', p_primary, 'secondary', p_secondary, 'accent', p_accent), p_ip);
  SELECT slug INTO v_slug FROM tenant WHERE id = p_tenant_id;
  CALL sp_tenant_public_get(v_slug);
END $$

DROP PROCEDURE IF EXISTS sp_branding_set_file $$
CREATE PROCEDURE sp_branding_set_file(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_kind VARCHAR(20), IN p_file_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_file_id BIGINT UNSIGNED;
  DECLARE v_slug VARCHAR(60);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.branding.manage');
  SELECT id INTO v_file_id FROM file_object
   WHERE uuid = p_file_uuid AND tenant_id = p_tenant_id AND owner_type = 'TENANT_BRANDING' AND deleted_at IS NULL;
  IF v_file_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FILE_NOT_FOUND'; END IF;
  CASE p_kind
    WHEN 'LOGO'      THEN UPDATE tenant_branding SET logo_file_id = v_file_id, updated_by = p_actor_id WHERE tenant_id = p_tenant_id;
    WHEN 'LOGO_DARK' THEN UPDATE tenant_branding SET logo_dark_file_id = v_file_id, updated_by = p_actor_id WHERE tenant_id = p_tenant_id;
    WHEN 'FAVICON'   THEN UPDATE tenant_branding SET favicon_file_id = v_file_id, updated_by = p_actor_id WHERE tenant_id = p_tenant_id;
    ELSE SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_KIND';
  END CASE;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'SET_FILE', 'tenant_branding', p_tenant_id,
       JSON_OBJECT('kind', p_kind, 'file', p_file_uuid), p_ip);
  SELECT slug INTO v_slug FROM tenant WHERE id = p_tenant_id;
  CALL sp_tenant_public_get(v_slug);
END $$

-- ---------------------------------------------------------------------
-- Archivos
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_file_create $$
CREATE PROCEDURE sp_file_create(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_owner_type VARCHAR(40), IN p_owner_id BIGINT UNSIGNED, IN p_category VARCHAR(40),
  IN p_storage_key VARCHAR(500), IN p_original_name VARCHAR(255), IN p_mime VARCHAR(120),
  IN p_size BIGINT UNSIGNED, IN p_sha256 CHAR(64), IN p_encrypted TINYINT(1))
BEGIN
  DECLARE v_actor_tenant BIGINT UNSIGNED;
  SET v_actor_tenant = (SELECT tenant_id FROM app_user WHERE id = p_actor_id);
  IF v_actor_tenant IS NOT NULL AND v_actor_tenant <> p_tenant_id THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TENANT_MISMATCH';
  END IF;
  INSERT INTO file_object (uuid, tenant_id, owner_type, owner_id, category, storage_key, original_name,
                           mime_type, size_bytes, sha256, is_encrypted, created_by)
  VALUES (p_uuid, p_tenant_id, p_owner_type, p_owner_id, p_category, p_storage_key, p_original_name,
          p_mime, p_size, p_sha256, p_encrypted, p_actor_id);
  SELECT id, uuid, tenant_id, owner_type, category, original_name, mime_type, size_bytes, created_at
    FROM file_object WHERE id = LAST_INSERT_ID();
END $$

-- p_tenant_id NULL solo para archivos públicos de branding (logo en login)
DROP PROCEDURE IF EXISTS sp_file_get $$
CREATE PROCEDURE sp_file_get(IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_public_only TINYINT(1))
BEGIN
  SELECT f.id, f.uuid, f.tenant_id, f.owner_type, f.category, f.storage_key, f.original_name,
         f.mime_type, f.size_bytes, f.sha256, f.is_encrypted
    FROM file_object f
   WHERE f.uuid = p_uuid AND f.deleted_at IS NULL
     AND (p_public_only = 0 OR f.owner_type = 'TENANT_BRANDING')
     AND (p_tenant_id IS NULL OR f.tenant_id = p_tenant_id);
END $$

-- ---------------------------------------------------------------------
-- Autenticación
-- ---------------------------------------------------------------------
-- p_tenant_slug NULL o '' = login de plataforma (Super Admin)
DROP PROCEDURE IF EXISTS sp_auth_find_user $$
CREATE PROCEDURE sp_auth_find_user(IN p_tenant_slug VARCHAR(60), IN p_email_bidx CHAR(64))
BEGIN
  IF p_tenant_slug IS NULL OR p_tenant_slug = '' THEN
    SELECT u.id, u.uuid, u.tenant_id, NULL AS tenant_uuid, NULL AS tenant_slug, 'ACTIVE' AS tenant_status,
           u.email_enc, u.full_name_enc, u.password_hash, u.locale, u.status, u.failed_logins, u.locked_until
      FROM app_user u
     WHERE u.tenant_id IS NULL AND u.email_bidx = p_email_bidx AND u.deleted_at IS NULL;
  ELSE
    SELECT u.id, u.uuid, u.tenant_id, t.uuid AS tenant_uuid, t.slug AS tenant_slug, t.status AS tenant_status,
           u.email_enc, u.full_name_enc, u.password_hash, u.locale, u.status, u.failed_logins, u.locked_until
      FROM app_user u JOIN tenant t ON t.id = u.tenant_id AND t.deleted_at IS NULL
     WHERE t.slug = p_tenant_slug AND u.email_bidx = p_email_bidx AND u.deleted_at IS NULL;
  END IF;
END $$

-- Registra el resultado del login y aplica la política de bloqueo
DROP PROCEDURE IF EXISTS sp_auth_register_attempt $$
CREATE PROCEDURE sp_auth_register_attempt(IN p_user_id BIGINT UNSIGNED, IN p_success TINYINT(1),
  IN p_max_attempts INT, IN p_lock_minutes INT, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_tenant BIGINT UNSIGNED;
  DECLARE v_failed INT;
  SELECT tenant_id INTO v_tenant FROM app_user WHERE id = p_user_id;
  IF p_success = 1 THEN
    UPDATE app_user SET failed_logins = 0, locked_until = NULL, last_login_at = NOW(3) WHERE id = p_user_id;
    CALL sp_audit_insert(v_tenant, p_user_id, NULL, 'LOGIN_OK', 'app_user', p_user_id, NULL, p_ip);
  ELSE
    UPDATE app_user SET failed_logins = failed_logins + 1 WHERE id = p_user_id;
    SELECT failed_logins INTO v_failed FROM app_user WHERE id = p_user_id;
    IF v_failed >= p_max_attempts THEN
      UPDATE app_user SET locked_until = DATE_ADD(NOW(3), INTERVAL p_lock_minutes MINUTE), failed_logins = 0
       WHERE id = p_user_id;
      CALL sp_audit_insert(v_tenant, p_user_id, NULL, 'LOCKED', 'app_user', p_user_id, JSON_OBJECT('minutes', p_lock_minutes), p_ip);
    ELSE
      CALL sp_audit_insert(v_tenant, p_user_id, NULL, 'LOGIN_FAIL', 'app_user', p_user_id, JSON_OBJECT('attempt', v_failed), p_ip);
    END IF;
  END IF;
END $$

DROP PROCEDURE IF EXISTS sp_user_security_profile $$
CREATE PROCEDURE sp_user_security_profile(IN p_user_id BIGINT UNSIGNED)
BEGIN
  SELECT u.id, u.uuid, u.tenant_id, t.uuid AS tenant_uuid, t.slug AS tenant_slug, IFNULL(t.status,'ACTIVE') AS tenant_status,
         u.email_enc, u.full_name_enc, u.locale, u.status
    FROM app_user u LEFT JOIN tenant t ON t.id = u.tenant_id
   WHERE u.id = p_user_id AND u.deleted_at IS NULL;
  SELECT r.code FROM user_role ur JOIN role r ON r.id = ur.role_id WHERE ur.user_id = p_user_id ORDER BY r.code;
  SELECT DISTINCT p.code FROM user_role ur
    JOIN role_permission rp ON rp.role_id = ur.role_id
    JOIN permission p ON p.id = rp.permission_id AND p.deleted_at IS NULL
   WHERE ur.user_id = p_user_id ORDER BY p.code;
END $$

DROP PROCEDURE IF EXISTS sp_session_create $$
CREATE PROCEDURE sp_session_create(IN p_uuid CHAR(36), IN p_user_id BIGINT UNSIGNED, IN p_token_hash CHAR(64),
  IN p_family_id CHAR(36), IN p_expires_at DATETIME(3), IN p_ip VARCHAR(45), IN p_user_agent VARCHAR(300))
BEGIN
  INSERT INTO user_session (uuid, user_id, token_hash, family_id, expires_at, ip, user_agent)
  VALUES (p_uuid, p_user_id, p_token_hash, p_family_id, p_expires_at, p_ip, LEFT(p_user_agent, 300));
END $$

-- Rotación de refresh token. Si se reutiliza un token ya rotado se revoca toda la familia.
DROP PROCEDURE IF EXISTS sp_session_rotate $$
CREATE PROCEDURE sp_session_rotate(IN p_old_hash CHAR(64), IN p_new_uuid CHAR(36), IN p_new_hash CHAR(64),
  IN p_expires_at DATETIME(3), IN p_ip VARCHAR(45), IN p_user_agent VARCHAR(300))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_user BIGINT UNSIGNED;
  DECLARE v_family CHAR(36);
  DECLARE v_revoked DATETIME(3);
  DECLARE v_expires DATETIME(3);
  DECLARE v_tenant BIGINT UNSIGNED;
  SELECT id, user_id, family_id, revoked_at, expires_at INTO v_id, v_user, v_family, v_revoked, v_expires
    FROM user_session WHERE token_hash = p_old_hash FOR UPDATE;
  IF v_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:SESSION_INVALID'; END IF;
  IF v_revoked IS NOT NULL THEN
    UPDATE user_session SET revoked_at = NOW(3) WHERE family_id = v_family AND revoked_at IS NULL;
    SELECT tenant_id INTO v_tenant FROM app_user WHERE id = v_user;
    CALL sp_audit_insert(v_tenant, v_user, NULL, 'TOKEN_REUSE', 'user_session', v_family, NULL, p_ip);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:SESSION_REUSED';
  END IF;
  IF v_expires < NOW(3) THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:SESSION_EXPIRED'; END IF;
  UPDATE user_session SET revoked_at = NOW(3) WHERE id = v_id;
  CALL sp_session_create(p_new_uuid, v_user, p_new_hash, v_family, p_expires_at, p_ip, p_user_agent);
  SELECT v_user AS user_id;
END $$

DROP PROCEDURE IF EXISTS sp_session_revoke $$
CREATE PROCEDURE sp_session_revoke(IN p_token_hash CHAR(64))
BEGIN
  UPDATE user_session s JOIN user_session f ON f.family_id = s.family_id
     SET f.revoked_at = IFNULL(f.revoked_at, NOW(3))
   WHERE s.token_hash = p_token_hash;
END $$

-- Crea el primer Super Admin (solo si no existe ninguno)
DROP PROCEDURE IF EXISTS sp_bootstrap_superadmin $$
CREATE PROCEDURE sp_bootstrap_superadmin(IN p_uuid CHAR(36), IN p_email_enc VARBINARY(512), IN p_email_bidx CHAR(64),
  IN p_name_enc VARBINARY(512), IN p_password_hash VARCHAR(255))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  IF EXISTS (SELECT 1 FROM app_user u JOIN user_role ur ON ur.user_id = u.id JOIN role r ON r.id = ur.role_id
              WHERE r.code = 'SUPER_ADMIN' AND u.deleted_at IS NULL) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:SUPERADMIN_EXISTS';
  END IF;
  INSERT INTO app_user (uuid, tenant_id, email_enc, email_bidx, full_name_enc, password_hash, status, password_changed_at)
  VALUES (p_uuid, NULL, p_email_enc, p_email_bidx, p_name_enc, p_password_hash, 'ACTIVE', NOW(3));
  SET v_id = LAST_INSERT_ID();
  INSERT INTO user_role (user_id, role_id) SELECT v_id, id FROM role WHERE code = 'SUPER_ADMIN';
  CALL sp_audit_insert(NULL, v_id, NULL, 'BOOTSTRAP', 'app_user', p_uuid, NULL, NULL);
  SELECT v_id AS id, p_uuid AS uuid;
END $$

-- ---------------------------------------------------------------------
-- Usuarios
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_user_list $$
CREATE PROCEDURE sp_user_list(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_status VARCHAR(20), IN p_page INT, IN p_page_size INT)
BEGIN
  DECLARE v_offset INT;
  IF NOT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'platform.tenants.manage') THEN
    CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.users.manage');
  END IF;
  SET p_page_size = LEAST(GREATEST(IFNULL(p_page_size, 20), 1), 100);
  SET v_offset = (GREATEST(IFNULL(p_page, 1), 1) - 1) * p_page_size;
  SELECT COUNT(*) AS total FROM app_user u
   WHERE u.tenant_key = IFNULL(p_tenant_id, 0) AND u.deleted_at IS NULL AND (p_status IS NULL OR u.status = p_status);
  SELECT u.id, u.uuid, u.email_enc, u.full_name_enc, u.locale, u.status, u.last_login_at, u.created_at,
         (SELECT GROUP_CONCAT(r.code ORDER BY r.code) FROM user_role ur JOIN role r ON r.id = ur.role_id WHERE ur.user_id = u.id) AS roles
    FROM app_user u
   WHERE u.tenant_key = IFNULL(p_tenant_id, 0) AND u.deleted_at IS NULL AND (p_status IS NULL OR u.status = p_status)
   ORDER BY u.created_at DESC
   LIMIT v_offset, p_page_size;
END $$

DROP PROCEDURE IF EXISTS sp_user_set_status $$
CREATE PROCEDURE sp_user_set_status(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_user_uuid CHAR(36), IN p_status VARCHAR(20), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  IF NOT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'platform.tenants.manage') THEN
    CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.users.manage');
  END IF;
  SELECT id INTO v_id FROM app_user WHERE uuid = p_user_uuid AND tenant_key = IFNULL(p_tenant_id, 0) AND deleted_at IS NULL;
  IF v_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  IF v_id = p_actor_id THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:CANNOT_CHANGE_SELF'; END IF;
  IF p_status NOT IN ('ACTIVE','DISABLED') THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_STATUS'; END IF;
  -- El tenant nunca puede quedarse sin un administrador activo
  IF p_status = 'DISABLED' AND p_tenant_id IS NOT NULL
     AND EXISTS (SELECT 1 FROM user_role ur JOIN role r ON r.id = ur.role_id WHERE ur.user_id = v_id AND r.code = 'TENANT_ADMIN')
     AND (SELECT COUNT(*) FROM app_user u JOIN user_role ur ON ur.user_id = u.id JOIN role r ON r.id = ur.role_id
           WHERE u.tenant_id = p_tenant_id AND r.code = 'TENANT_ADMIN' AND u.status = 'ACTIVE' AND u.deleted_at IS NULL) <= 1 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:LAST_TENANT_ADMIN';
  END IF;
  UPDATE app_user SET status = p_status, failed_logins = 0, locked_until = NULL, updated_by = p_actor_id WHERE id = v_id;
  IF p_status = 'DISABLED' THEN
    UPDATE user_session SET revoked_at = NOW(3) WHERE user_id = v_id AND revoked_at IS NULL;
  END IF;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'SET_STATUS', 'app_user', p_user_uuid, JSON_OBJECT('status', p_status), p_ip);
END $$

-- ---------------------------------------------------------------------
-- Invitaciones
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_invitation_create $$
CREATE PROCEDURE sp_invitation_create(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_uuid CHAR(36), IN p_email_enc VARBINARY(512), IN p_email_bidx CHAR(64), IN p_role_code VARCHAR(40),
  IN p_token_hash CHAR(64), IN p_expires_at DATETIME(3), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_role_id BIGINT UNSIGNED;
  DECLARE v_is_platform TINYINT(1);
  DECLARE v_tenant_status VARCHAR(20);
  SELECT id, is_platform INTO v_role_id, v_is_platform FROM role WHERE code = p_role_code AND deleted_at IS NULL;
  IF v_role_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:ROLE_NOT_FOUND'; END IF;
  IF p_tenant_id IS NULL THEN
    -- Invitar a otro Super Admin: solo un Super Admin
    CALL sp_assert_actor(p_actor_id, NULL, 'platform.tenants.manage');
    IF v_is_platform = 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:ROLE_NOT_ALLOWED'; END IF;
  ELSE
    IF NOT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'platform.tenants.manage') THEN
      CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.users.invite');
      -- Solo un administrador del tenant puede invitar a otro administrador
      IF p_role_code = 'TENANT_ADMIN' AND NOT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'tenant.users.manage') THEN
        SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:ROLE_NOT_ALLOWED';
      END IF;
    END IF;
    IF v_is_platform = 1 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:ROLE_NOT_ALLOWED'; END IF;
    SELECT status INTO v_tenant_status FROM tenant WHERE id = p_tenant_id AND deleted_at IS NULL;
    IF v_tenant_status IS NULL OR v_tenant_status <> 'ACTIVE' THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TENANT_INACTIVE';
    END IF;
  END IF;
  IF EXISTS (SELECT 1 FROM app_user WHERE tenant_key = IFNULL(p_tenant_id, 0) AND email_bidx = p_email_bidx AND deleted_at IS NULL) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:USER_ALREADY_EXISTS';
  END IF;
  -- Una sola invitación vigente por correo
  UPDATE user_invitation SET status = 'REVOKED', updated_by = p_actor_id
   WHERE IFNULL(tenant_id, 0) = IFNULL(p_tenant_id, 0) AND email_bidx = p_email_bidx AND status = 'PENDING';
  INSERT INTO user_invitation (uuid, tenant_id, email_enc, email_bidx, role_id, token_hash, expires_at, created_by)
  VALUES (p_uuid, p_tenant_id, p_email_enc, p_email_bidx, v_role_id, p_token_hash, p_expires_at, p_actor_id);
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'CREATE', 'user_invitation', p_uuid, JSON_OBJECT('role', p_role_code), p_ip);
  SELECT i.uuid, i.status, i.expires_at, r.code AS role_code FROM user_invitation i JOIN role r ON r.id = i.role_id
   WHERE i.uuid = p_uuid;
END $$

DROP PROCEDURE IF EXISTS sp_invitation_get_by_token $$
CREATE PROCEDURE sp_invitation_get_by_token(IN p_token_hash CHAR(64))
BEGIN
  UPDATE user_invitation SET status = 'EXPIRED' WHERE token_hash = p_token_hash AND status = 'PENDING' AND expires_at < NOW(3);
  SELECT i.uuid, i.tenant_id, t.slug AS tenant_slug, t.name AS tenant_name, i.email_enc, r.code AS role_code,
         i.status, i.expires_at
    FROM user_invitation i
    JOIN role r ON r.id = i.role_id
    LEFT JOIN tenant t ON t.id = i.tenant_id
   WHERE i.token_hash = p_token_hash AND i.deleted_at IS NULL;
END $$

DROP PROCEDURE IF EXISTS sp_invitation_accept $$
CREATE PROCEDURE sp_invitation_accept(IN p_token_hash CHAR(64), IN p_user_uuid CHAR(36),
  IN p_name_enc VARBINARY(512), IN p_password_hash VARCHAR(255), IN p_locale VARCHAR(10), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_inv_id BIGINT UNSIGNED;
  DECLARE v_tenant BIGINT UNSIGNED;
  DECLARE v_role BIGINT UNSIGNED;
  DECLARE v_email_enc VARBINARY(512);
  DECLARE v_bidx CHAR(64);
  DECLARE v_status VARCHAR(20);
  DECLARE v_expires DATETIME(3);
  DECLARE v_user_id BIGINT UNSIGNED;
  SELECT id, tenant_id, role_id, email_enc, email_bidx, status, expires_at
    INTO v_inv_id, v_tenant, v_role, v_email_enc, v_bidx, v_status, v_expires
    FROM user_invitation WHERE token_hash = p_token_hash AND deleted_at IS NULL FOR UPDATE;
  IF v_inv_id IS NULL OR v_status <> 'PENDING' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVITATION_INVALID'; END IF;
  IF v_expires < NOW(3) THEN
    UPDATE user_invitation SET status = 'EXPIRED' WHERE id = v_inv_id;
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVITATION_EXPIRED';
  END IF;
  IF EXISTS (SELECT 1 FROM app_user WHERE tenant_key = IFNULL(v_tenant, 0) AND email_bidx = v_bidx AND deleted_at IS NULL) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:USER_ALREADY_EXISTS';
  END IF;
  INSERT INTO app_user (uuid, tenant_id, email_enc, email_bidx, full_name_enc, password_hash, locale, status, password_changed_at)
  VALUES (p_user_uuid, v_tenant, v_email_enc, v_bidx, p_name_enc, p_password_hash, IFNULL(p_locale,'es'), 'ACTIVE', NOW(3));
  SET v_user_id = LAST_INSERT_ID();
  INSERT INTO user_role (user_id, role_id) VALUES (v_user_id, v_role);
  UPDATE user_invitation SET status = 'ACCEPTED', accepted_at = NOW(3), accepted_user_id = v_user_id WHERE id = v_inv_id;
  CALL sp_audit_insert(v_tenant, v_user_id, NULL, 'ACCEPT', 'user_invitation', v_inv_id, NULL, p_ip);
  SELECT v_user_id AS user_id;
END $$

DROP PROCEDURE IF EXISTS sp_invitation_list $$
CREATE PROCEDURE sp_invitation_list(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  IF NOT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'platform.tenants.manage') THEN
    CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.users.invite');
  END IF;
  UPDATE user_invitation SET status = 'EXPIRED'
   WHERE IFNULL(tenant_id, 0) = IFNULL(p_tenant_id, 0) AND status = 'PENDING' AND expires_at < NOW(3);
  SELECT i.uuid, i.email_enc, r.code AS role_code, i.status, i.expires_at, i.accepted_at, i.created_at
    FROM user_invitation i JOIN role r ON r.id = i.role_id
   WHERE IFNULL(i.tenant_id, 0) = IFNULL(p_tenant_id, 0) AND i.deleted_at IS NULL
   ORDER BY i.created_at DESC LIMIT 200;
END $$

DROP PROCEDURE IF EXISTS sp_invitation_revoke $$
CREATE PROCEDURE sp_invitation_revoke(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_uuid CHAR(36), IN p_ip VARCHAR(45))
BEGIN
  IF NOT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'platform.tenants.manage') THEN
    CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.users.invite');
  END IF;
  UPDATE user_invitation SET status = 'REVOKED', updated_by = p_actor_id
   WHERE uuid = p_uuid AND IFNULL(tenant_id, 0) = IFNULL(p_tenant_id, 0) AND status = 'PENDING';
  IF ROW_COUNT() = 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'REVOKE', 'user_invitation', p_uuid, NULL, p_ip);
END $$

-- ---------------------------------------------------------------------
-- API keys de integración
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_apikey_create $$
CREATE PROCEDURE sp_apikey_create(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_name VARCHAR(120), IN p_prefix CHAR(12), IN p_hash CHAR(64), IN p_scopes JSON, IN p_expires_at DATETIME(3),
  IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_is_sa TINYINT(1);
  SET v_is_sa = (SELECT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'platform.apikeys.approve'));
  IF v_is_sa = 0 THEN CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.apikeys.request'); END IF;
  IF NOT EXISTS (SELECT 1 FROM tenant WHERE id = p_tenant_id AND status = 'ACTIVE' AND deleted_at IS NULL) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TENANT_INACTIVE';
  END IF;
  -- Creada por el tenant: queda PENDING hasta que el Super Admin la habilite
  INSERT INTO api_key (uuid, tenant_id, name, key_prefix, key_hash, scopes, status, enabled_by, enabled_at, expires_at, created_by)
  VALUES (p_uuid, p_tenant_id, p_name, p_prefix, p_hash, p_scopes,
          IF(v_is_sa = 1, 'ENABLED', 'PENDING'), IF(v_is_sa = 1, p_actor_id, NULL), IF(v_is_sa = 1, NOW(3), NULL),
          p_expires_at, p_actor_id);
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'CREATE', 'api_key', p_uuid, JSON_OBJECT('prefix', p_prefix), p_ip);
  CALL sp_apikey_list(p_actor_id, p_tenant_id, p_uuid);
END $$

DROP PROCEDURE IF EXISTS sp_apikey_list $$
CREATE PROCEDURE sp_apikey_list(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  IF NOT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'platform.apikeys.approve') THEN
    CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.apikeys.request');
  END IF;
  SELECT k.uuid, k.tenant_id, t.slug AS tenant_slug, t.name AS tenant_name, k.name, k.key_prefix, k.scopes, k.status,
         k.enabled_at, k.expires_at, k.last_used_at, k.created_at
    FROM api_key k JOIN tenant t ON t.id = k.tenant_id
   WHERE k.deleted_at IS NULL
     AND (p_tenant_id IS NULL OR k.tenant_id = p_tenant_id)
     AND (p_uuid IS NULL OR k.uuid = p_uuid)
   ORDER BY k.created_at DESC LIMIT 500;
END $$

DROP PROCEDURE IF EXISTS sp_apikey_set_status $$
CREATE PROCEDURE sp_apikey_set_status(IN p_actor_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_status VARCHAR(20),
  IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_tenant BIGINT UNSIGNED;
  DECLARE v_current VARCHAR(20);
  SELECT id, tenant_id, status INTO v_id, v_tenant, v_current FROM api_key WHERE uuid = p_uuid AND deleted_at IS NULL;
  IF v_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  IF p_status = 'REVOKED' AND NOT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'platform.apikeys.approve') THEN
    -- El tenant puede revocar sus propias keys, pero nunca habilitarlas
    CALL sp_assert_actor(p_actor_id, v_tenant, 'tenant.apikeys.request');
  ELSE
    CALL sp_assert_actor(p_actor_id, NULL, 'platform.apikeys.approve');
  END IF;
  IF v_current = 'REVOKED' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:APIKEY_REVOKED'; END IF;
  IF p_status NOT IN ('ENABLED','DISABLED','REVOKED') THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:INVALID_STATUS'; END IF;
  UPDATE api_key
     SET status = p_status, updated_by = p_actor_id,
         enabled_by = IF(p_status = 'ENABLED', p_actor_id, enabled_by),
         enabled_at = IF(p_status = 'ENABLED', NOW(3), enabled_at)
   WHERE id = v_id;
  CALL sp_audit_insert(v_tenant, p_actor_id, NULL, 'SET_STATUS', 'api_key', p_uuid, JSON_OBJECT('status', p_status), p_ip);
  CALL sp_apikey_list(p_actor_id, v_tenant, p_uuid);
END $$

-- Usado por el middleware de la API pública (el hash se compara en la app en tiempo constante)
DROP PROCEDURE IF EXISTS sp_apikey_lookup $$
CREATE PROCEDURE sp_apikey_lookup(IN p_prefix CHAR(12))
BEGIN
  SELECT k.id, k.uuid, k.tenant_id, t.slug AS tenant_slug, k.key_hash, k.scopes, k.status, k.expires_at,
         t.status AS tenant_status
    FROM api_key k JOIN tenant t ON t.id = k.tenant_id
   WHERE k.key_prefix = p_prefix AND k.deleted_at IS NULL;
END $$

DROP PROCEDURE IF EXISTS sp_apikey_touch $$
CREATE PROCEDURE sp_apikey_touch(IN p_id BIGINT UNSIGNED, IN p_ip VARCHAR(45))
BEGIN
  UPDATE api_key SET last_used_at = NOW(3), last_used_ip = p_ip WHERE id = p_id;
END $$

-- ---------------------------------------------------------------------
-- Maestro de maestros
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_master_table_list $$
CREATE PROCEDURE sp_master_table_list()
BEGIN
  SELECT m.id, m.code, m.name_i18n, m.description, m.scope, m.is_system, p.code AS parent_code
    FROM master_table m LEFT JOIN master_table p ON p.id = m.parent_table_id
   WHERE m.deleted_at IS NULL ORDER BY m.code;
END $$

-- Valores visibles para un tenant: globales + propios. p_codes = 'LANGUAGE,CURRENCY'
DROP PROCEDURE IF EXISTS sp_master_item_list $$
CREATE PROCEDURE sp_master_item_list(IN p_tenant_id BIGINT UNSIGNED, IN p_codes VARCHAR(1000), IN p_include_inactive TINYINT(1))
BEGIN
  SELECT i.uuid, m.code AS table_code, i.code, i.label_i18n, p.uuid AS parent_uuid,
         i.num1, i.num2, i.num3, i.str1, i.str2, i.str3, i.date1, i.date2, i.date3,
         i.sort_order, i.is_active, IF(i.tenant_id IS NULL, 0, 1) AS is_tenant_owned, i.legacy_id,
         IFNULL(i.updated_at, i.created_at) AS version
    FROM master_item i
    JOIN master_table m ON m.id = i.master_table_id AND m.deleted_at IS NULL
    LEFT JOIN master_item p ON p.id = i.parent_item_id
   WHERE i.deleted_at IS NULL
     AND (i.tenant_id IS NULL OR i.tenant_id = p_tenant_id)
     AND (p_include_inactive = 1 OR i.is_active = 1)
     AND (p_codes IS NULL OR p_codes = '' OR FIND_IN_SET(m.code, p_codes) > 0)
   ORDER BY m.code, i.sort_order, i.code;
END $$

DROP PROCEDURE IF EXISTS sp_master_item_upsert $$
CREATE PROCEDURE sp_master_item_upsert(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_table_code VARCHAR(60), IN p_uuid CHAR(36), IN p_code VARCHAR(60), IN p_label_i18n JSON,
  IN p_parent_uuid CHAR(36), IN p_num1 DECIMAL(18,4), IN p_num2 DECIMAL(18,4), IN p_num3 DECIMAL(18,4),
  IN p_str1 VARCHAR(250), IN p_str2 VARCHAR(250), IN p_str3 VARCHAR(250), IN p_sort INT, IN p_active TINYINT(1),
  IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_table_id BIGINT UNSIGNED;
  DECLARE v_scope VARCHAR(10);
  DECLARE v_system TINYINT(1);
  DECLARE v_parent BIGINT UNSIGNED;
  DECLARE v_item_id BIGINT UNSIGNED;
  DECLARE v_item_tenant BIGINT UNSIGNED;
  SELECT id, scope, is_system INTO v_table_id, v_scope, v_system FROM master_table WHERE code = p_table_code AND deleted_at IS NULL;
  IF v_table_id IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  IF p_tenant_id IS NULL THEN
    CALL sp_assert_actor(p_actor_id, NULL, 'platform.masters.manage');
    IF v_scope = 'TENANT' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:MASTER_SCOPE'; END IF;
  ELSE
    CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.masters.manage');
    IF v_scope = 'GLOBAL' OR v_system = 1 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:MASTER_SCOPE'; END IF;
  END IF;
  IF JSON_TYPE(JSON_EXTRACT(p_label_i18n, '$.es')) IS NULL THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:LABEL_ES_REQUIRED';
  END IF;
  IF p_parent_uuid IS NOT NULL THEN
    SELECT id INTO v_parent FROM master_item WHERE uuid = p_parent_uuid AND deleted_at IS NULL;
    IF v_parent IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:PARENT_NOT_FOUND'; END IF;
  END IF;
  SELECT id, tenant_id INTO v_item_id, v_item_tenant FROM master_item WHERE uuid = p_uuid;
  IF v_item_id IS NULL THEN
    IF EXISTS (SELECT 1 FROM master_item WHERE master_table_id = v_table_id AND tenant_key IN (0, IFNULL(p_tenant_id,0))
                 AND code = p_code AND deleted_at IS NULL) THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:CODE_TAKEN';
    END IF;
    INSERT INTO master_item (uuid, master_table_id, tenant_id, code, label_i18n, parent_item_id,
                             num1, num2, num3, str1, str2, str3, sort_order, is_active, created_by)
    VALUES (p_uuid, v_table_id, p_tenant_id, p_code, p_label_i18n, v_parent,
            p_num1, p_num2, p_num3, p_str1, p_str2, p_str3, IFNULL(p_sort,0), IFNULL(p_active,1), p_actor_id);
  ELSE
    IF IFNULL(v_item_tenant, 0) <> IFNULL(p_tenant_id, 0) THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TENANT_MISMATCH';
    END IF;
    UPDATE master_item SET code = p_code, label_i18n = p_label_i18n, parent_item_id = v_parent,
           num1 = p_num1, num2 = p_num2, num3 = p_num3, str1 = p_str1, str2 = p_str2, str3 = p_str3,
           sort_order = IFNULL(p_sort,0), is_active = IFNULL(p_active,1), updated_by = p_actor_id
     WHERE id = v_item_id;
  END IF;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'UPSERT', 'master_item', p_uuid, JSON_OBJECT('table', p_table_code, 'code', p_code), p_ip);
  SELECT i.uuid, m.code AS table_code, i.code, i.label_i18n, i.sort_order, i.is_active
    FROM master_item i JOIN master_table m ON m.id = i.master_table_id WHERE i.uuid = p_uuid;
END $$

-- ---------------------------------------------------------------------
-- Formatos/reportes y traducciones por tenant
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_report_template_get $$
CREATE PROCEDURE sp_report_template_get(IN p_tenant_id BIGINT UNSIGNED, IN p_code VARCHAR(60), IN p_locale VARCHAR(10))
BEGIN
  -- Primero la del tenant; si no existe, la base de plataforma
  SELECT uuid, tenant_id, code, locale, name, engine, definition, version
    FROM report_template
   WHERE code = p_code AND locale = p_locale AND is_active = 1 AND deleted_at IS NULL
     AND (tenant_id = p_tenant_id OR tenant_id IS NULL)
   ORDER BY tenant_id IS NULL, version DESC LIMIT 1;
END $$

DROP PROCEDURE IF EXISTS sp_report_template_upsert $$
CREATE PROCEDURE sp_report_template_upsert(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_uuid CHAR(36), IN p_code VARCHAR(60), IN p_locale VARCHAR(10), IN p_name VARCHAR(150),
  IN p_definition JSON, IN p_ip VARCHAR(45))
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.reports.manage');
  INSERT INTO report_template (uuid, tenant_id, code, locale, name, definition, created_by)
  VALUES (p_uuid, p_tenant_id, p_code, p_locale, p_name, p_definition, p_actor_id)
  ON DUPLICATE KEY UPDATE name = VALUES(name), definition = VALUES(definition), version = version + 1,
                          updated_by = p_actor_id, is_active = 1;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'UPSERT', 'report_template', p_code, JSON_OBJECT('locale', p_locale), p_ip);
  CALL sp_report_template_get(p_tenant_id, p_code, p_locale);
END $$

DROP PROCEDURE IF EXISTS sp_translation_list $$
CREATE PROCEDURE sp_translation_list(IN p_tenant_id BIGINT UNSIGNED, IN p_locale VARCHAR(10))
BEGIN
  SELECT tkey, tvalue FROM tenant_translation
   WHERE tenant_id = p_tenant_id AND locale = p_locale AND deleted_at IS NULL;
END $$

DROP PROCEDURE IF EXISTS sp_translation_upsert $$
CREATE PROCEDURE sp_translation_upsert(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_locale VARCHAR(10), IN p_key VARCHAR(150), IN p_value VARCHAR(1000), IN p_ip VARCHAR(45))
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.translations.manage');
  INSERT INTO tenant_translation (tenant_id, locale, tkey, tvalue, created_by)
  VALUES (p_tenant_id, p_locale, p_key, p_value, p_actor_id)
  ON DUPLICATE KEY UPDATE tvalue = VALUES(tvalue), updated_by = p_actor_id, deleted_at = NULL;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'UPSERT', 'tenant_translation', p_key, JSON_OBJECT('locale', p_locale), p_ip);
END $$

-- ---------------------------------------------------------------------
-- Sincronización offline (idempotencia de operaciones encoladas)
-- Devuelve is_duplicate = 1 si la operación ya se había aplicado.
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_sync_register $$
CREATE PROCEDURE sp_sync_register(IN p_tenant_id BIGINT UNSIGNED, IN p_user_id BIGINT UNSIGNED,
  IN p_client_op_id CHAR(36), IN p_entity VARCHAR(60), IN p_operation VARCHAR(20),
  IN p_entity_uuid CHAR(36), IN p_client_ts DATETIME(3))
BEGIN
  IF IFNULL((SELECT tenant_id FROM app_user WHERE id = p_user_id), 0) <> p_tenant_id THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:TENANT_MISMATCH'; END IF;
  IF EXISTS (SELECT 1 FROM sync_operation WHERE tenant_id = p_tenant_id AND client_op_id = p_client_op_id) THEN
    SELECT 1 AS is_duplicate, status, result FROM sync_operation WHERE tenant_id = p_tenant_id AND client_op_id = p_client_op_id;
  ELSE
    INSERT INTO sync_operation (tenant_id, user_id, client_op_id, entity, operation, entity_uuid, client_ts)
    VALUES (p_tenant_id, p_user_id, p_client_op_id, p_entity, p_operation, p_entity_uuid, p_client_ts);
    SELECT 0 AS is_duplicate, 'APPLIED' AS status, NULL AS result;
  END IF;
END $$

DROP PROCEDURE IF EXISTS sp_sync_complete $$
CREATE PROCEDURE sp_sync_complete(IN p_tenant_id BIGINT UNSIGNED, IN p_client_op_id CHAR(36),
  IN p_status VARCHAR(20), IN p_result JSON)
BEGIN
  UPDATE sync_operation SET status = p_status, result = p_result
   WHERE tenant_id = p_tenant_id AND client_op_id = p_client_op_id;
END $$

-- ---------------------------------------------------------------------
-- Auditoría
-- ---------------------------------------------------------------------
DROP PROCEDURE IF EXISTS sp_audit_list $$
CREATE PROCEDURE sp_audit_list(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_page INT, IN p_page_size INT)
BEGIN
  DECLARE v_offset INT;
  IF NOT EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'platform.audit.read') THEN
    CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.audit.read');
  END IF;
  SET p_page_size = LEAST(GREATEST(IFNULL(p_page_size, 50), 1), 200);
  SET v_offset = (GREATEST(IFNULL(p_page, 1), 1) - 1) * p_page_size;
  SELECT a.id, a.tenant_id, u.uuid AS user_uuid, a.action, a.entity, a.entity_id, a.detail, a.ip, a.created_at
    FROM audit_log a LEFT JOIN app_user u ON u.id = a.user_id
   WHERE (p_tenant_id IS NULL OR a.tenant_id = p_tenant_id)
   ORDER BY a.id DESC LIMIT v_offset, p_page_size;
END $$

DELIMITER ;
