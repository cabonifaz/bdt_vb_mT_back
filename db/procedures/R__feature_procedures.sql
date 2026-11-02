-- =====================================================================
-- Catálogo de funcionalidades: el Super Admin las ve todas, las suspende globalmente o por tenant y las activa por
-- tenant. Habilitada para un tenant = status ACTIVE y (activación del tenant, o default_enabled si no tiene).
-- =====================================================================
DELIMITER $$

-- Funcionalidades habilitadas para un tenant (las usa la API para permitir o no cada módulo)
DROP PROCEDURE IF EXISTS sp_tenant_features $$
CREATE PROCEDURE sp_tenant_features(IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  SELECT f.code FROM feature f
    LEFT JOIN tenant_feature tf ON tf.feature_id = f.id AND tf.tenant_id = p_tenant_id
   WHERE f.status = 'ACTIVE' AND (f.is_core = 1 OR IFNULL(tf.enabled, f.default_enabled) = 1);
END $$

DROP PROCEDURE IF EXISTS sp_features_list $$
CREATE PROCEDURE sp_features_list(IN p_actor_id BIGINT UNSIGNED)
BEGIN
  DECLARE v_total INT;
  CALL sp_assert_actor(p_actor_id, NULL, 'platform.features.manage');
  SELECT COUNT(*) INTO v_total FROM tenant WHERE deleted_at IS NULL;
  SELECT JSON_OBJECT('code', f.code, 'name', f.name_i18n, 'description', f.description_i18n, 'module', f.module,
           'roles', f.impacted_roles, 'isCore', f.is_core = 1, 'defaultEnabled', f.default_enabled = 1, 'status', f.status,
           'sortOrder', f.sort_order, 'releasedAt', f.released_at, 'tenantsTotal', v_total,
           'tenantsEnabled', (SELECT COUNT(*) FROM tenant t LEFT JOIN tenant_feature tf ON tf.tenant_id = t.id AND tf.feature_id = f.id
                               WHERE t.deleted_at IS NULL AND (f.is_core = 1 OR IFNULL(tf.enabled, f.default_enabled) = 1))) AS doc
    FROM feature f ORDER BY f.sort_order, f.id;
END $$

-- Suspensión o reactivación global (las de núcleo no se suspenden)
DROP PROCEDURE IF EXISTS sp_feature_set_status $$
CREATE PROCEDURE sp_feature_set_status(IN p_actor_id BIGINT UNSIGNED, IN p_code VARCHAR(40), IN p_status VARCHAR(10), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_core TINYINT(1);
  CALL sp_assert_actor(p_actor_id, NULL, 'platform.features.manage');
  SELECT is_core INTO v_core FROM feature WHERE code = p_code;
  IF v_core IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  IF p_status NOT IN ('ACTIVE','SUSPENDED') THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  IF v_core = 1 AND p_status = 'SUSPENDED' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FEATURE_CORE'; END IF;
  UPDATE feature SET status = p_status, updated_by = p_actor_id WHERE code = p_code;
  CALL sp_audit_insert(NULL, p_actor_id, NULL, 'STATUS', 'feature', p_code, JSON_OBJECT('status', p_status), p_ip);
END $$

-- Estado de una funcionalidad en cada tenant
DROP PROCEDURE IF EXISTS sp_feature_tenants $$
CREATE PROCEDURE sp_feature_tenants(IN p_actor_id BIGINT UNSIGNED, IN p_code VARCHAR(40))
BEGIN
  CALL sp_assert_actor(p_actor_id, NULL, 'platform.features.manage');
  SELECT JSON_OBJECT('tenantId', t.uuid, 'slug', t.slug, 'name', t.name, 'tenantStatus', t.status,
           'enabled', f.is_core = 1 OR IFNULL(tf.enabled, f.default_enabled) = 1, 'explicit', tf.tenant_id IS NOT NULL,
           'changedAt', tf.changed_at) AS doc
    FROM tenant t CROSS JOIN feature f
    LEFT JOIN tenant_feature tf ON tf.tenant_id = t.id AND tf.feature_id = f.id
   WHERE f.code = p_code AND t.deleted_at IS NULL
   ORDER BY t.name;
END $$

-- Activa o suspende una funcionalidad en un tenant. Devuelve si quedó recién habilitada (para avisar a los usuarios
-- con los roles impactados), el tenant y los datos de la funcionalidad.
DROP PROCEDURE IF EXISTS sp_tenant_feature_set $$
CREATE PROCEDURE sp_tenant_feature_set(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_uuid CHAR(36), IN p_code VARCHAR(40),
  IN p_enabled TINYINT(1), IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_tenant, v_feature BIGINT UNSIGNED;
  DECLARE v_core, v_was TINYINT(1);
  CALL sp_assert_actor(p_actor_id, NULL, 'platform.features.manage');
  SELECT id INTO v_tenant FROM tenant WHERE uuid = p_tenant_uuid AND deleted_at IS NULL;
  SELECT f.id, f.is_core, IFNULL(tf.enabled, f.default_enabled) INTO v_feature, v_core, v_was
    FROM feature f LEFT JOIN tenant_feature tf ON tf.feature_id = f.id AND tf.tenant_id = v_tenant WHERE f.code = p_code;
  IF v_tenant IS NULL OR v_feature IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  IF v_core = 1 AND p_enabled = 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FEATURE_CORE'; END IF;
  INSERT INTO tenant_feature (tenant_id, feature_id, enabled, changed_by) VALUES (v_tenant, v_feature, p_enabled, p_actor_id)
  ON DUPLICATE KEY UPDATE enabled = VALUES(enabled), changed_by = p_actor_id;
  CALL sp_audit_insert(v_tenant, p_actor_id, NULL, IF(p_enabled = 1, 'ENABLE', 'SUSPEND'), 'feature', p_code, NULL, p_ip);
  SELECT (v_was = 0 AND p_enabled = 1) AS newly_enabled, v_tenant AS tenant_id, t.slug, f.name_i18n, f.description_i18n, f.module, f.impacted_roles, f.status
    FROM feature f JOIN tenant t ON t.id = v_tenant WHERE f.id = v_feature;
END $$

-- Funcionalidades nuevas que el Super Admin aún no conoce (llegaron con un despliegue)
DROP PROCEDURE IF EXISTS sp_features_unnoticed $$
CREATE PROCEDURE sp_features_unnoticed()
BEGIN
  SELECT code, name_i18n, description_i18n, module FROM feature WHERE sa_notified_at IS NULL ORDER BY sort_order;
END $$

DROP PROCEDURE IF EXISTS sp_feature_noticed $$
CREATE PROCEDURE sp_feature_noticed(IN p_code VARCHAR(40))
BEGIN
  UPDATE feature SET sa_notified_at = NOW(3) WHERE code = p_code AND sa_notified_at IS NULL;
  SELECT ROW_COUNT() AS marked;
END $$

DELIMITER ;
