-- =====================================================================
-- Notificaciones (centro en la app + push del sistema operativo)
-- =====================================================================
DELIMITER $$

-- El actor debe ser un usuario activo del tenant (las notificaciones son personales, no requieren permiso extra)
DROP PROCEDURE IF EXISTS sp_assert_member $$
CREATE PROCEDURE sp_assert_member(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  IF p_actor_id IS NULL OR NOT EXISTS (SELECT 1 FROM app_user WHERE id = p_actor_id AND tenant_id = p_tenant_id
                                         AND status = 'ACTIVE' AND deleted_at IS NULL) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:FORBIDDEN';
  END IF;
END $$

-- Registra un evento y notifica a los usuarios activos del tenant con p_recipient_perm (menos el actor).
-- Agrupa: si el mismo actor generó hace menos de p_group_minutes una notificación del mismo tipo (o de p_group_with,
-- p. ej. un alta seguida de ediciones) sobre la misma entidad, se actualiza esa (vuelve a "no leída") y no se envía push.
-- Devuelve: (1) uuid, is_new, nombre cifrado del actor; (2) si es nueva, las suscripciones push de los destinatarios.
DROP PROCEDURE IF EXISTS sp_notify_entity $$
CREATE PROCEDURE sp_notify_entity(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36),
  IN p_type VARCHAR(40), IN p_group_with VARCHAR(40), IN p_entity_type VARCHAR(30), IN p_entity_uuid CHAR(36),
  IN p_payload JSON, IN p_recipient_perm VARCHAR(80), IN p_group_minutes INT)
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  DECLARE v_uuid CHAR(36);
  DECLARE v_new TINYINT(1) DEFAULT 0;
  CALL sp_assert_member(p_actor_id, p_tenant_id);

  SELECT id, uuid INTO v_id, v_uuid FROM notification
   WHERE tenant_id = p_tenant_id AND entity_uuid = p_entity_uuid AND actor_user_id = p_actor_id
     AND type IN (p_type, IFNULL(p_group_with, p_type))
     AND updated_at >= NOW(3) - INTERVAL IFNULL(p_group_minutes, 0) MINUTE
   ORDER BY id DESC LIMIT 1;

  IF v_id IS NOT NULL THEN
    UPDATE notification SET payload = p_payload, event_count = event_count + 1, updated_at = NOW(3) WHERE id = v_id;
    UPDATE notification_recipient SET read_at = NULL, delivered_at = NOW(3) WHERE notification_id = v_id;
  ELSE
    SET v_new = 1, v_uuid = p_uuid;
    INSERT INTO notification (uuid, tenant_id, type, entity_type, entity_uuid, actor_user_id, payload)
    VALUES (p_uuid, p_tenant_id, p_type, p_entity_type, p_entity_uuid, p_actor_id, p_payload);
    SET v_id = LAST_INSERT_ID();
    INSERT INTO notification_recipient (notification_id, user_id)
    SELECT DISTINCT v_id, u.id
      FROM app_user u
      JOIN v_user_permission vp ON vp.user_id = u.id AND vp.code = p_recipient_perm
     WHERE u.tenant_id = p_tenant_id AND u.status = 'ACTIVE' AND u.deleted_at IS NULL AND u.id <> p_actor_id
       AND NOT EXISTS (SELECT 1 FROM notification_pref np WHERE np.user_id = u.id AND np.type = p_type AND np.in_app = 0);
  END IF;

  SELECT v_uuid AS uuid, v_new AS is_new, a.full_name_enc AS actor_name_enc
    FROM app_user a WHERE a.id = p_actor_id;

  IF v_new = 1 THEN
    SELECT s.endpoint_hash, s.endpoint, s.p256dh, s.auth, u.locale
      FROM notification_recipient r
      JOIN app_user u ON u.id = r.user_id
      JOIN push_subscription s ON s.user_id = r.user_id AND s.tenant_id = p_tenant_id
     WHERE r.notification_id = v_id
       AND NOT EXISTS (SELECT 1 FROM notification_pref np WHERE np.user_id = r.user_id AND np.type = p_type AND np.push = 0);
  END IF;
END $$

-- Bandeja del usuario: (1) no leídas; (2) notificaciones (más recientes primero; p_before para paginar)
DROP PROCEDURE IF EXISTS sp_notification_list $$
CREATE PROCEDURE sp_notification_list(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_unread_only TINYINT(1), IN p_before DATETIME(3), IN p_limit INT)
BEGIN
  DECLARE v_limit INT DEFAULT LEAST(GREATEST(IFNULL(p_limit, 20), 1), 100);
  CALL sp_assert_member(p_actor_id, p_tenant_id);
  SELECT COUNT(*) AS unread FROM notification_recipient r JOIN notification n ON n.id = r.notification_id
   WHERE r.user_id = p_actor_id AND r.read_at IS NULL AND n.tenant_id = p_tenant_id;
  SELECT n.uuid, n.type, n.entity_type, n.entity_uuid, n.payload, n.event_count, n.created_at, r.delivered_at, r.read_at,
         a.uuid AS actor_uuid, a.full_name_enc AS actor_name_enc
    FROM notification_recipient r
    JOIN notification n ON n.id = r.notification_id
    LEFT JOIN app_user a ON a.id = n.actor_user_id
   WHERE r.user_id = p_actor_id AND n.tenant_id = p_tenant_id
     AND (IFNULL(p_unread_only, 0) = 0 OR r.read_at IS NULL)
     AND (p_before IS NULL OR r.delivered_at < p_before)
   ORDER BY r.delivered_at DESC
   LIMIT v_limit;
END $$

-- Marca como leída una notificación (p_uuid) o todas (p_uuid NULL)
DROP PROCEDURE IF EXISTS sp_notification_mark_read $$
CREATE PROCEDURE sp_notification_mark_read(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_uuid CHAR(36))
BEGIN
  CALL sp_assert_member(p_actor_id, p_tenant_id);
  UPDATE notification_recipient r JOIN notification n ON n.id = r.notification_id
     SET r.read_at = NOW(3)
   WHERE r.user_id = p_actor_id AND n.tenant_id = p_tenant_id AND r.read_at IS NULL AND (p_uuid IS NULL OR n.uuid = p_uuid);
END $$

-- Suscripción push del dispositivo (si el endpoint ya existía, pasa al usuario actual)
DROP PROCEDURE IF EXISTS sp_push_subscribe $$
CREATE PROCEDURE sp_push_subscribe(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_hash CHAR(64),
  IN p_endpoint VARCHAR(1000), IN p_p256dh VARCHAR(200), IN p_auth VARCHAR(100), IN p_ua VARCHAR(300))
BEGIN
  CALL sp_assert_member(p_actor_id, p_tenant_id);
  INSERT INTO push_subscription (user_id, tenant_id, endpoint_hash, endpoint, p256dh, auth, user_agent)
  VALUES (p_actor_id, p_tenant_id, p_hash, p_endpoint, p_p256dh, p_auth, p_ua)
  ON DUPLICATE KEY UPDATE user_id = p_actor_id, tenant_id = p_tenant_id, endpoint = p_endpoint, p256dh = p_p256dh,
    auth = p_auth, user_agent = p_ua, failures = 0;
  SELECT COUNT(*) AS devices FROM push_subscription WHERE user_id = p_actor_id;
END $$

DROP PROCEDURE IF EXISTS sp_push_unsubscribe $$
CREATE PROCEDURE sp_push_unsubscribe(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_hash CHAR(64))
BEGIN
  CALL sp_assert_member(p_actor_id, p_tenant_id);
  DELETE FROM push_subscription WHERE endpoint_hash = p_hash AND user_id = p_actor_id;
END $$

-- Resultado del envío: ok, o caducada (404/410: se borra), o error (se borra tras 5 fallos seguidos)
DROP PROCEDURE IF EXISTS sp_push_result $$
CREATE PROCEDURE sp_push_result(IN p_hash CHAR(64), IN p_ok TINYINT(1), IN p_gone TINYINT(1))
BEGIN
  IF p_ok = 1 THEN
    UPDATE push_subscription SET last_ok_at = NOW(3), failures = 0 WHERE endpoint_hash = p_hash;
  ELSEIF p_gone = 1 THEN
    DELETE FROM push_subscription WHERE endpoint_hash = p_hash;
  ELSE
    UPDATE push_subscription SET failures = failures + 1 WHERE endpoint_hash = p_hash;
    DELETE FROM push_subscription WHERE endpoint_hash = p_hash AND failures >= 5;
  END IF;
END $$

-- Preferencias: tipos con su configuración (sin fila = todo activo) y dispositivos suscritos
DROP PROCEDURE IF EXISTS sp_notification_prefs_get $$
CREATE PROCEDURE sp_notification_prefs_get(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  CALL sp_assert_member(p_actor_id, p_tenant_id);
  SELECT type, in_app, push FROM notification_pref WHERE user_id = p_actor_id;
  SELECT COUNT(*) AS devices FROM push_subscription WHERE user_id = p_actor_id;
END $$

DROP PROCEDURE IF EXISTS sp_notification_pref_set $$
CREATE PROCEDURE sp_notification_pref_set(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_type VARCHAR(40), IN p_in_app TINYINT(1), IN p_push TINYINT(1))
BEGIN
  CALL sp_assert_member(p_actor_id, p_tenant_id);
  INSERT INTO notification_pref (user_id, type, in_app, push) VALUES (p_actor_id, p_type, p_in_app, p_push)
  ON DUPLICATE KEY UPDATE in_app = p_in_app, push = p_push;
END $$

DELIMITER ;
