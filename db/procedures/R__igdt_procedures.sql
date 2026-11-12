-- =====================================================================
-- Integración con IGDT: IGDT gobierna los catálogos compartidos (master_table.igdt_shared, contactos y tarifario).
-- Sin integración ACTIVE el tenant administra sus maestros en BDT; con integración, BDT los recibe por API y los
-- bloquea (sp_igdt_assert_editable). La copia local se escribe solo desde aquí (sp_igdt_apply_changes).
-- =====================================================================
DELIMITER $$

DROP PROCEDURE IF EXISTS sp_igdt_assert_editable $$
CREATE PROCEDURE sp_igdt_assert_editable(IN p_tenant_id BIGINT UNSIGNED, IN p_table_code VARCHAR(60))
BEGIN
  IF p_tenant_id IS NOT NULL
     AND EXISTS (SELECT 1 FROM tenant_integration WHERE tenant_id = p_tenant_id AND provider = 'IGDT' AND status = 'ACTIVE')
     AND EXISTS (SELECT 1 FROM master_table WHERE code = p_table_code AND igdt_shared = 1) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:MANAGED_BY_IGDT';
  END IF;
END $$

-- ¿El tenant tiene la integración activa? (para /auth/me: la app bloquea la edición de esos catálogos)
DROP PROCEDURE IF EXISTS sp_igdt_active $$
CREATE PROCEDURE sp_igdt_active(IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  SELECT EXISTS (SELECT 1 FROM tenant_integration WHERE tenant_id = p_tenant_id AND provider = 'IGDT' AND status = 'ACTIVE') AS active;
END $$

DROP PROCEDURE IF EXISTS sp_igdt_status $$
CREATE PROCEDURE sp_igdt_status(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.integrations.manage');
  SELECT JSON_OBJECT('connected', IFNULL(ti.status, '') = 'ACTIVE', 'status', IFNULL(ti.status, 'NONE'), 'remoteTenant', ti.remote_tenant,
           'importedAt', ti.imported_at,
           'bdtLinked', EXISTS (SELECT 1 FROM api_key k WHERE k.id = ti.link_key_id AND k.status = 'ENABLED' AND k.deleted_at IS NULL), 'lastSyncAt', ti.last_sync_at, 'lastError', ti.last_error, 'cursor', IFNULL(ti.sync_cursor, 0),
           'tables', (SELECT JSON_ARRAYAGG(JSON_OBJECT('code', mt.code, 'label', mt.name_i18n)) FROM master_table mt WHERE mt.igdt_shared = 1),
           'counts', JSON_OBJECT(
             'clients', (SELECT COUNT(*) FROM master_item i JOIN master_table t ON t.id = i.master_table_id AND t.code = 'CLIENT'
                          WHERE i.tenant_id = p_tenant_id AND i.deleted_at IS NULL),
             'contacts', (SELECT COUNT(*) FROM client_contact WHERE tenant_id = p_tenant_id AND deleted_at IS NULL),
             'tariffs', (SELECT COUNT(*) FROM tariff WHERE tenant_id = p_tenant_id AND deleted_at IS NULL))) AS doc
    FROM (SELECT p_tenant_id AS tid) x
    LEFT JOIN tenant_integration ti ON ti.tenant_id = x.tid AND ti.provider = 'IGDT';
END $$

-- Guarda el token (cifrado por la app) tras verificarlo contra IGDT y deja la integración ACTIVE.
DROP PROCEDURE IF EXISTS sp_igdt_save $$
CREATE PROCEDURE sp_igdt_save(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_key_enc VARBINARY(1024),
  IN p_remote_tenant VARCHAR(60), IN p_ip VARCHAR(45))
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.integrations.manage');
  INSERT INTO tenant_integration (tenant_id, provider, api_key_enc, remote_tenant, status, hires_from, created_by)
  VALUES (p_tenant_id, 'IGDT', p_key_enc, p_remote_tenant, 'ACTIVE', NOW(3) - INTERVAL 30 DAY, p_actor_id)
  ON DUPLICATE KEY UPDATE api_key_enc = VALUES(api_key_enc), remote_tenant = VALUES(remote_tenant), status = 'ACTIVE',
                          last_error = NULL, hires_from = IFNULL(hires_from, VALUES(hires_from)), updated_by = p_actor_id;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'CONNECT', 'tenant_integration', 'IGDT', JSON_OBJECT('remoteTenant', p_remote_tenant), p_ip);
END $$

-- Desconectar: el token se borra (no se reutiliza) y los catálogos vuelven a administrarse en BDT. La copia y el
-- mapeo se conservan para una reconexión.
DROP PROCEDURE IF EXISTS sp_igdt_disable $$
CREATE PROCEDURE sp_igdt_disable(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_ip VARCHAR(45))
BEGIN
  DECLARE v_link BIGINT UNSIGNED;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'tenant.integrations.manage');
  SET v_link = (SELECT link_key_id FROM tenant_integration WHERE tenant_id = p_tenant_id AND provider = 'IGDT');
  UPDATE tenant_integration SET status = 'DISABLED', api_key_enc = '', link_key_id = NULL, updated_by = p_actor_id WHERE tenant_id = p_tenant_id AND provider = 'IGDT';
  IF ROW_COUNT() = 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  -- la llave BDT entregada a IGDT deja de servir
  IF v_link IS NOT NULL THEN
    UPDATE api_key SET status = 'REVOKED', updated_by = p_actor_id WHERE id = v_link AND status <> 'REVOKED';
    CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'SET_STATUS', 'api_key', (SELECT uuid FROM api_key WHERE id = v_link),
                         JSON_OBJECT('status', 'REVOKED', 'reason', 'IGDT_DISCONNECT'), p_ip);
  END IF;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'DISCONNECT', 'tenant_integration', 'IGDT', NULL, p_ip);
END $$

-- Tenants con la integración activa y su token cifrado (sincronización periódica; sin actor, solo la llama la API).
DROP PROCEDURE IF EXISTS sp_igdt_targets $$
CREATE PROCEDURE sp_igdt_targets(IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  SELECT ti.tenant_id, t.slug, ti.api_key_enc, ti.sync_cursor, ti.imported_at IS NOT NULL AS imported, ti.hires_cursor
    FROM tenant_integration ti JOIN tenant t ON t.id = ti.tenant_id AND t.status = 'ACTIVE' AND t.deleted_at IS NULL
   WHERE ti.provider = 'IGDT' AND ti.status = 'ACTIVE' AND (p_tenant_id IS NULL OR ti.tenant_id = p_tenant_id)
   ORDER BY ti.tenant_id;
END $$

-- Llave BDT automática para IGDT (alcances talent.read, talent.search y masters.read, ya habilitada): se crea solo si la integración está activa y
-- el tenant aún no tiene una vigente. Devuelve created = 1 si la creó (la app la entrega a IGDT; si falla, la descarta
-- con sp_igdt_link_key_drop). Sin actor: la llama la API (conexión, «Sincronizar ahora» o la sincronización periódica).
DROP PROCEDURE IF EXISTS sp_igdt_link_key_create $$
CREATE PROCEDURE sp_igdt_link_key_create(IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_name VARCHAR(120),
  IN p_prefix CHAR(12), IN p_hash CHAR(64))
BEGIN
  DECLARE v_status VARCHAR(10);
  DECLARE v_link BIGINT UNSIGNED;
  DECLARE v_created TINYINT DEFAULT 0;
  DECLARE EXIT HANDLER FOR SQLEXCEPTION BEGIN ROLLBACK; RESIGNAL; END;
  START TRANSACTION;
  SELECT status, link_key_id INTO v_status, v_link FROM tenant_integration
   WHERE tenant_id = p_tenant_id AND provider = 'IGDT' FOR UPDATE;
  IF v_status = 'ACTIVE' AND NOT EXISTS (SELECT 1 FROM api_key WHERE id = v_link AND status = 'ENABLED' AND deleted_at IS NULL)
     AND EXISTS (SELECT 1 FROM tenant WHERE id = p_tenant_id AND status = 'ACTIVE' AND deleted_at IS NULL) THEN
    INSERT INTO api_key (uuid, tenant_id, name, key_prefix, key_hash, scopes, status, enabled_at)
    VALUES (p_uuid, p_tenant_id, p_name, p_prefix, p_hash, JSON_ARRAY('talent.read', 'talent.search', 'masters.read'), 'ENABLED', NOW(3));
    UPDATE tenant_integration SET link_key_id = LAST_INSERT_ID() WHERE tenant_id = p_tenant_id AND provider = 'IGDT';
    CALL sp_audit_insert(p_tenant_id, NULL, NULL, 'CREATE', 'api_key', p_uuid,
                         JSON_OBJECT('prefix', p_prefix, 'reason', 'IGDT_LINK', 'scopes', JSON_ARRAY('talent.read', 'talent.search', 'masters.read')), NULL);
    SET v_created = 1;
  END IF;
  COMMIT;
  SELECT v_created AS created;
END $$

-- Descarta la llave automática que IGDT no recibió (o que IGDT dejó de usar): revocada y eliminada, y el tenant queda
-- sin vincular para reintentar.
DROP PROCEDURE IF EXISTS sp_igdt_link_key_drop $$
CREATE PROCEDURE sp_igdt_link_key_drop(IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36), IN p_reason VARCHAR(60))
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  SET v_id = (SELECT id FROM api_key WHERE uuid = p_uuid AND tenant_id = p_tenant_id);
  IF v_id IS NOT NULL THEN
    UPDATE tenant_integration SET link_key_id = NULL WHERE tenant_id = p_tenant_id AND provider = 'IGDT' AND link_key_id = v_id;
    UPDATE api_key SET status = 'REVOKED', deleted_at = IFNULL(deleted_at, NOW(3)) WHERE id = v_id;
    CALL sp_audit_insert(p_tenant_id, NULL, NULL, 'SET_STATUS', 'api_key', p_uuid, JSON_OBJECT('status', 'REVOKED', 'reason', p_reason), NULL);
  END IF;
END $$

-- Carga inicial: clientes y perfiles que usa el tenant (los suyos y los globales de BDT) y sus contactos.
DROP PROCEDURE IF EXISTS sp_igdt_export $$
CREATE PROCEDURE sp_igdt_export(IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  SELECT JSON_OBJECT(
           'items', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('table', t.code, 'id', i.uuid, 'code', i.code, 'label', i.label_i18n,
                        'str1', i.str1, 'str2', i.str2, 'sortOrder', i.sort_order, 'active', i.is_active))
                      FROM master_item i JOIN master_table t ON t.id = i.master_table_id AND t.code IN ('CLIENT', 'RQ_PROFILE')
                     WHERE i.tenant_key IN (0, p_tenant_id) AND i.deleted_at IS NULL), JSON_ARRAY()),
           'contacts', IFNULL((SELECT JSON_ARRAYAGG(JSON_OBJECT('id', cc.uuid, 'clientId', c.uuid, 'firstName', cc.first_name,
                        'lastName', cc.last_name, 'secondLastName', cc.second_last_name, 'position', cc.position, 'phone', cc.phone,
                        'phone2', cc.phone2, 'email', cc.email, 'email2', cc.email2))
                      FROM client_contact cc JOIN master_item c ON c.id = cc.client_item_id
                     WHERE cc.tenant_id = p_tenant_id AND cc.deleted_at IS NULL), JSON_ARRAY())) AS doc;
END $$

-- Mapeo devuelto por la carga inicial: [{entity, sourceId (uuid en BDT), igdtId}]. Solo si difieren.
DROP PROCEDURE IF EXISTS sp_igdt_apply_map $$
CREATE PROCEDURE sp_igdt_apply_map(IN p_tenant_id BIGINT UNSIGNED, IN p_map JSON)
BEGIN
  INSERT IGNORE INTO igdt_map (tenant_id, entity, remote_uuid, local_id)
  SELECT p_tenant_id, 'MASTER_ITEM', j.igdt_id, i.id
    FROM JSON_TABLE(IFNULL(p_map, JSON_ARRAY()), '$[*]' COLUMNS (entity VARCHAR(20) PATH '$.entity', source_id CHAR(36) PATH '$.sourceId',
           igdt_id CHAR(36) PATH '$.igdtId')) j
    JOIN master_item i ON i.uuid = j.source_id AND i.tenant_key IN (0, p_tenant_id)
   WHERE j.entity = 'MASTER_ITEM' AND j.igdt_id <> j.source_id;
  INSERT IGNORE INTO igdt_map (tenant_id, entity, remote_uuid, local_id)
  SELECT p_tenant_id, 'CLIENT_CONTACT', j.igdt_id, cc.id
    FROM JSON_TABLE(IFNULL(p_map, JSON_ARRAY()), '$[*]' COLUMNS (entity VARCHAR(20) PATH '$.entity', source_id CHAR(36) PATH '$.sourceId',
           igdt_id CHAR(36) PATH '$.igdtId')) j
    JOIN client_contact cc ON cc.uuid = j.source_id AND cc.tenant_id = p_tenant_id
   WHERE j.entity = 'CLIENT_CONTACT' AND j.igdt_id <> j.source_id;
  UPDATE tenant_integration SET imported_at = NOW(3) WHERE tenant_id = p_tenant_id AND provider = 'IGDT';
END $$

-- Ítem local que corresponde a un id de IGDT: por el mapeo, por el mismo uuid o por el código en la misma tabla.
DROP PROCEDURE IF EXISTS sp_igdt_local_item $$
CREATE PROCEDURE sp_igdt_local_item(IN p_tenant_id BIGINT UNSIGNED, IN p_table VARCHAR(60), IN p_remote CHAR(36), IN p_code VARCHAR(60),
  OUT o_id BIGINT UNSIGNED)
BEGIN
  SET o_id = (SELECT local_id FROM igdt_map WHERE tenant_id = p_tenant_id AND entity = 'MASTER_ITEM' AND remote_uuid = p_remote);
  IF o_id IS NULL THEN
    SET o_id = (SELECT i.id FROM master_item i JOIN master_table t ON t.id = i.master_table_id
                 WHERE i.uuid = p_remote AND i.tenant_key IN (0, p_tenant_id) AND (p_table IS NULL OR t.code = p_table));
  END IF;
  IF o_id IS NULL AND p_code IS NOT NULL AND p_table IS NOT NULL THEN
    SET o_id = (SELECT i.id FROM master_item i JOIN master_table t ON t.id = i.master_table_id AND t.code = p_table
                 WHERE i.code = p_code AND i.tenant_key IN (0, p_tenant_id) AND i.deleted_at IS NULL ORDER BY i.tenant_key DESC LIMIT 1);
    IF o_id IS NOT NULL THEN
      INSERT IGNORE INTO igdt_map (tenant_id, entity, remote_uuid, local_id) VALUES (p_tenant_id, 'MASTER_ITEM', p_remote, o_id);
    END IF;
  END IF;
END $$

-- Aplica una página de cambios de IGDT (GET /integration/v1/catalog/changes) y avanza el cursor.
-- Los ítems globales de BDT solo se mapean (su texto lo administra la plataforma); los del tenant se actualizan.
-- Una baja en IGDT desactiva el ítem en BDT (no se borra: los RQ lo siguen referenciando).
DROP PROCEDURE IF EXISTS sp_igdt_apply_changes $$
CREATE PROCEDURE sp_igdt_apply_changes(IN p_tenant_id BIGINT UNSIGNED, IN p_changes JSON, IN p_cursor BIGINT UNSIGNED)
BEGIN
  DECLARE v_done INT DEFAULT 0;
  DECLARE v_entity VARCHAR(20);
  DECLARE v_id CHAR(36);
  DECLARE v_data JSON;
  DECLARE v_table VARCHAR(60);
  DECLARE v_code VARCHAR(60);
  DECLARE v_table_id, v_local, v_local_tenant, v_client, v_profile, v_currency, v_rate_type BIGINT UNSIGNED;
  DECLARE v_deleted, v_active, v_scoped TINYINT(1);
  DECLARE v_applied INT DEFAULT 0;
  DECLARE v_new CHAR(36);
  DECLARE cur CURSOR FOR
    SELECT j.entity, j.id, j.data FROM JSON_TABLE(IFNULL(p_changes, JSON_ARRAY()), '$[*]' COLUMNS (
             entity VARCHAR(20) PATH '$.entity', id CHAR(36) PATH '$.id', data JSON PATH '$.data')) j;
  DECLARE CONTINUE HANDLER FOR NOT FOUND SET v_done = 1;
  IF NOT EXISTS (SELECT 1 FROM tenant_integration WHERE tenant_id = p_tenant_id AND provider = 'IGDT' AND status = 'ACTIVE') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:IGDT_NOT_CONNECTED';
  END IF;
  OPEN cur;
  changes: LOOP
    FETCH cur INTO v_entity, v_id, v_data;
    IF v_done = 1 THEN LEAVE changes; END IF;
    IF v_data IS NULL OR v_id IS NULL THEN ITERATE changes; END IF;
    SET v_deleted = IFNULL(JSON_EXTRACT(v_data, '$.deleted') = TRUE, 0);

    IF v_entity = 'MASTER_ITEM' THEN
      SET v_table = JSON_UNQUOTE(JSON_EXTRACT(v_data, '$.table'));
      SET v_code = JSON_UNQUOTE(JSON_EXTRACT(v_data, '$.code'));
      SET v_table_id = (SELECT id FROM master_table WHERE code = v_table AND igdt_shared = 1 AND deleted_at IS NULL);
      IF v_table_id IS NULL OR v_code IS NULL THEN ITERATE changes; END IF;
      SET v_active = IFNULL(JSON_EXTRACT(v_data, '$.active') = TRUE, 1) AND NOT v_deleted;
      SET v_scoped = IFNULL(JSON_EXTRACT(v_data, '$.tenantScoped') = TRUE, 0);
      CALL sp_igdt_local_item(p_tenant_id, v_table, v_id, v_code, v_local);
      IF v_local IS NOT NULL THEN
        SET v_local_tenant = (SELECT tenant_id FROM master_item WHERE id = v_local);
        IF v_local_tenant = p_tenant_id THEN
          UPDATE master_item SET code = v_code, label_i18n = JSON_EXTRACT(v_data, '$.label'),
                 str1 = JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.str1'), CAST('null' AS JSON))),
                 str2 = JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.str2'), CAST('null' AS JSON))),
                 str3 = JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.str3'), CAST('null' AS JSON))),
                 sort_order = IFNULL(JSON_EXTRACT(v_data, '$.sortOrder'), sort_order), is_active = v_active
           WHERE id = v_local;
        END IF;
      ELSEIF v_deleted = 0 THEN
        -- uuid de IGDT salvo que ya exista en BDT (no se puede consultar master_item dentro de su propio INSERT)
        SET v_new = IF(EXISTS (SELECT 1 FROM master_item WHERE uuid = v_id), UUID(), v_id);
        INSERT INTO master_item (uuid, master_table_id, tenant_id, code, label_i18n, str1, str2, str3, sort_order, is_active)
        VALUES (v_new, v_table_id, IF(v_scoped, p_tenant_id, NULL), v_code,
                JSON_EXTRACT(v_data, '$.label'),
                JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.str1'), CAST('null' AS JSON))),
                JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.str2'), CAST('null' AS JSON))),
                JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.str3'), CAST('null' AS JSON))),
                IFNULL(JSON_EXTRACT(v_data, '$.sortOrder'), 0), v_active);
        INSERT IGNORE INTO igdt_map (tenant_id, entity, remote_uuid, local_id) VALUES (p_tenant_id, 'MASTER_ITEM', v_id, LAST_INSERT_ID());
      END IF;
      SET v_applied = v_applied + 1;

    ELSEIF v_entity = 'CLIENT_CONTACT' THEN
      CALL sp_igdt_local_item(p_tenant_id, 'CLIENT', JSON_UNQUOTE(JSON_EXTRACT(v_data, '$.clientId')), NULL, v_client);
      SET v_local = (SELECT local_id FROM igdt_map WHERE tenant_id = p_tenant_id AND entity = 'CLIENT_CONTACT' AND remote_uuid = v_id);
      IF v_local IS NULL THEN SET v_local = (SELECT id FROM client_contact WHERE uuid = v_id AND tenant_id = p_tenant_id); END IF;
      IF v_deleted = 1 THEN
        UPDATE client_contact SET deleted_at = IFNULL(deleted_at, NOW(3)) WHERE id = v_local;
      ELSEIF v_client IS NOT NULL THEN
        IF v_local IS NULL THEN
          SET v_new = IF(EXISTS (SELECT 1 FROM client_contact WHERE uuid = v_id), UUID(), v_id);
          INSERT INTO client_contact (uuid, tenant_id, client_item_id, first_name, last_name)
          VALUES (v_new, p_tenant_id, v_client, '-', '-');
          SET v_local = LAST_INSERT_ID();
          INSERT IGNORE INTO igdt_map (tenant_id, entity, remote_uuid, local_id) VALUES (p_tenant_id, 'CLIENT_CONTACT', v_id, v_local);
        END IF;
        UPDATE client_contact SET client_item_id = v_client,
               first_name = JSON_UNQUOTE(JSON_EXTRACT(v_data, '$.firstName')), last_name = JSON_UNQUOTE(JSON_EXTRACT(v_data, '$.lastName')),
               second_last_name = JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.secondLastName'), CAST('null' AS JSON))),
               position = JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.position'), CAST('null' AS JSON))),
               phone = JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.phone'), CAST('null' AS JSON))),
               phone2 = JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.phone2'), CAST('null' AS JSON))),
               email = JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.email'), CAST('null' AS JSON))),
               email2 = JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.email2'), CAST('null' AS JSON))), deleted_at = NULL
         WHERE id = v_local;
      END IF;
      SET v_applied = v_applied + 1;

    ELSEIF v_entity = 'TARIFF' THEN
      IF v_deleted = 1 THEN
        UPDATE tariff SET deleted_at = IFNULL(deleted_at, NOW(3)), synced_at = NOW(3) WHERE uuid = v_id AND tenant_id = p_tenant_id;
      ELSE
        SET v_client = NULL;
        IF JSON_TYPE(JSON_EXTRACT(v_data, '$.clientId')) = 'STRING' THEN
          CALL sp_igdt_local_item(p_tenant_id, 'CLIENT', JSON_UNQUOTE(JSON_EXTRACT(v_data, '$.clientId')), NULL, v_client);
        END IF;
        CALL sp_igdt_local_item(p_tenant_id, 'RQ_PROFILE', JSON_UNQUOTE(JSON_EXTRACT(v_data, '$.profileId')), NULL, v_profile);
        CALL sp_igdt_local_item(p_tenant_id, 'CURRENCY', JSON_UNQUOTE(JSON_EXTRACT(v_data, '$.currencyId')), NULL, v_currency);
        CALL sp_igdt_local_item(p_tenant_id, 'RATE_TYPE', JSON_UNQUOTE(JSON_EXTRACT(v_data, '$.rateTypeId')), NULL, v_rate_type);
        IF v_profile IS NOT NULL AND v_currency IS NOT NULL AND v_rate_type IS NOT NULL
           AND (v_client IS NOT NULL OR JSON_TYPE(JSON_EXTRACT(v_data, '$.clientId')) <> 'STRING') THEN
          INSERT INTO tariff (uuid, tenant_id, client_item_id, profile_item_id, currency_item_id, rate_type_item_id, rate, valid_from, valid_to)
          VALUES (v_id, p_tenant_id, v_client, v_profile, v_currency, v_rate_type, JSON_EXTRACT(v_data, '$.rate'),
                  JSON_UNQUOTE(JSON_EXTRACT(v_data, '$.validFrom')), JSON_UNQUOTE(NULLIF(JSON_EXTRACT(v_data, '$.validTo'), CAST('null' AS JSON))))
          ON DUPLICATE KEY UPDATE client_item_id = VALUES(client_item_id), profile_item_id = VALUES(profile_item_id),
                 currency_item_id = VALUES(currency_item_id), rate_type_item_id = VALUES(rate_type_item_id), rate = VALUES(rate),
                 valid_from = VALUES(valid_from), valid_to = VALUES(valid_to), deleted_at = NULL, synced_at = NOW(3);
        END IF;
      END IF;
      SET v_applied = v_applied + 1;
    END IF;
  END LOOP;
  CLOSE cur;
  UPDATE tenant_integration SET sync_cursor = GREATEST(sync_cursor, IFNULL(p_cursor, sync_cursor)), last_sync_at = NOW(3), last_error = NULL
   WHERE tenant_id = p_tenant_id AND provider = 'IGDT';
  SELECT v_applied AS applied;
END $$

DROP PROCEDURE IF EXISTS sp_igdt_sync_error $$
CREATE PROCEDURE sp_igdt_sync_error(IN p_tenant_id BIGINT UNSIGNED, IN p_error VARCHAR(500))
BEGIN
  UPDATE tenant_integration SET last_error = LEFT(p_error, 500), last_sync_at = NOW(3) WHERE tenant_id = p_tenant_id AND provider = 'IGDT';
END $$

-- Tarifa sugerida para una vacante: la propia del cliente si hay, si no la general del perfil, vigente en p_date.
DROP PROCEDURE IF EXISTS sp_tariff_lookup $$
CREATE PROCEDURE sp_tariff_lookup(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_profile_uuid CHAR(36),
  IN p_client_uuid CHAR(36), IN p_date DATE)
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.rates.read');
  SELECT JSON_OBJECT('id', t.uuid, 'rate', t.rate, 'currency', cu.code, 'rateType', JSON_OBJECT('code', rt.code, 'label', rt.label_i18n),
           'scope', IF(t.client_item_id IS NULL, 'GENERAL', 'CLIENT'), 'validFrom', t.valid_from, 'validTo', t.valid_to) AS doc
    FROM tariff t
    JOIN master_item p ON p.id = t.profile_item_id AND p.uuid = p_profile_uuid
    JOIN master_item cu ON cu.id = t.currency_item_id
    JOIN master_item rt ON rt.id = t.rate_type_item_id
    LEFT JOIN master_item c ON c.id = t.client_item_id
   WHERE t.tenant_id = p_tenant_id AND t.deleted_at IS NULL
     AND (t.client_item_id IS NULL OR c.uuid = p_client_uuid)
     AND t.valid_from <= IFNULL(p_date, CURDATE()) AND IFNULL(t.valid_to, '9999-12-31') >= IFNULL(p_date, CURDATE())
   ORDER BY t.client_item_id IS NULL, t.valid_from DESC
   LIMIT 1;
END $$

DELIMITER ;
