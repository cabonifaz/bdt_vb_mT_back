-- =====================================================================
-- Alertas de RQ sin movimiento: umbrales por tenant y barrido periódico.
-- Un RQ abierto con postulantes en proceso está "sin movimiento" cuando ningún postulante cambió de etapa
-- (ni se agregó uno, ni se reabrió el RQ) en los días hábiles (lunes a viernes) de atención; pasados los críticos, es crítica.
-- =====================================================================
DELIMITER $$

-- Umbrales del tenant (por defecto 5 y 10 días, con notificación)
DROP PROCEDURE IF EXISTS sp_rq_alert_days $$
CREATE PROCEDURE sp_rq_alert_days(IN p_tenant_id BIGINT UNSIGNED, OUT o_warn INT, OUT o_crit INT, OUT o_notify TINYINT(1))
BEGIN
  SET o_warn = 5, o_crit = 10, o_notify = 1;
  SELECT warn_days, critical_days, notify INTO o_warn, o_crit, o_notify FROM rq_alert_setting WHERE tenant_id = p_tenant_id;
END $$

DROP PROCEDURE IF EXISTS sp_rq_alert_get $$
CREATE PROCEDURE sp_rq_alert_get(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED)
BEGIN
  DECLARE v_warn, v_crit INT;
  DECLARE v_notify TINYINT(1);
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.read');
  CALL sp_rq_alert_days(p_tenant_id, v_warn, v_crit, v_notify);
  SELECT JSON_OBJECT('warnDays', v_warn, 'criticalDays', v_crit, 'notify', v_notify = 1) AS doc;
END $$

DROP PROCEDURE IF EXISTS sp_rq_alert_save $$
CREATE PROCEDURE sp_rq_alert_save(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_warn INT, IN p_crit INT,
  IN p_notify TINYINT(1), IN p_ip VARCHAR(45))
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'rq.alerts.manage');
  IF p_warn IS NULL OR p_crit IS NULL OR p_warn NOT BETWEEN 1 AND 90 OR p_crit NOT BETWEEN 2 AND 180 OR p_crit <= p_warn THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION';
  END IF;
  INSERT INTO rq_alert_setting (tenant_id, warn_days, critical_days, notify, updated_by)
  VALUES (p_tenant_id, p_warn, p_crit, IFNULL(p_notify, 1), p_actor_id)
  ON DUPLICATE KEY UPDATE warn_days = VALUES(warn_days), critical_days = VALUES(critical_days), notify = VALUES(notify), updated_by = p_actor_id;
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'UPDATE', 'rq_alert_setting', NULL,
    JSON_OBJECT('warnDays', p_warn, 'criticalDays', p_crit, 'notify', IFNULL(p_notify, 1)), p_ip);
END $$

-- Barrido (lo corre la API cada hora): recalcula el nivel de cada RQ y devuelve los que subieron de nivel
-- (de nada a atención, o a crítico) para notificar a sus responsables y a los jefes de selección. Solo una vez por
-- nivel: si hay movimiento, el nivel vuelve a 0 y la alerta podrá repetirse más adelante.
-- Los días se cuentan con la fecha local de cada tenant (p_today solo si su zona horaria no se reconoce). Tenants suspendidos, con la funcionalidad apagada o sin aviso no se notifican.
DROP PROCEDURE IF EXISTS sp_rq_stale_scan $$
CREATE PROCEDURE sp_rq_stale_scan(IN p_today DATE)
BEGIN
  DROP TEMPORARY TABLE IF EXISTS tmp_rq_stale;
  CREATE TEMPORARY TABLE tmp_rq_stale (id BIGINT UNSIGNED PRIMARY KEY, old_level TINYINT, level TINYINT, idle INT, notify TINYINT(1)) ENGINE=MEMORY;
  INSERT INTO tmp_rq_stale
  SELECT x.id, x.stale_level,
         CASE WHEN x.idle IS NULL THEN 0 WHEN x.idle >= x.crit THEN 2 WHEN x.idle >= x.warn THEN 1 ELSE 0 END,
         x.idle, x.notify
    FROM (SELECT r.id, r.stale_level, IFNULL(a.warn_days, 5) AS warn, IFNULL(a.critical_days, 10) AS crit,
                 IFNULL(a.notify, 1) = 1 AND t.status = 'ACTIVE'
                   AND EXISTS (SELECT 1 FROM feature f LEFT JOIN tenant_feature tf ON tf.feature_id = f.id AND tf.tenant_id = r.tenant_id
                                WHERE f.code = 'rq_stale_alerts' AND f.status = 'ACTIVE' AND IFNULL(tf.enabled, f.default_enabled) = 1) AS notify,
                 IF(s.str1 = 'OPEN' AND act.in_process > 0,
                    (5 * (DATEDIFF(IFNULL(DATE(CONVERT_TZ(UTC_TIMESTAMP(), '+00:00', t.timezone)), p_today), '2000-12-31') DIV 7) + LEAST(DATEDIFF(IFNULL(DATE(CONVERT_TZ(UTC_TIMESTAMP(), '+00:00', t.timezone)), p_today), '2000-12-31') MOD 7, 5)) - (5 * (DATEDIFF(act.last_move_day, '2000-12-31') DIV 7) + LEAST(DATEDIFF(act.last_move_day, '2000-12-31') MOD 7, 5)), NULL) AS idle
            FROM requirement r
            JOIN tenant t ON t.id = r.tenant_id AND t.deleted_at IS NULL
            JOIN master_item s ON s.id = r.status_item_id
            JOIN v_rq_activity act ON act.requirement_id = r.id
            LEFT JOIN rq_alert_setting a ON a.tenant_id = r.tenant_id
           WHERE r.deleted_at IS NULL AND (r.stale_level > 0 OR s.str1 = 'OPEN')) x;

  SELECT r.tenant_id, t.slug, r.uuid, r.code, r.title, JSON_UNQUOTE(JSON_EXTRACT(c.label_i18n, '$.es')) AS client, x.level, x.idle
    FROM tmp_rq_stale x
    JOIN requirement r ON r.id = x.id
    JOIN master_item c ON c.id = r.client_item_id
    JOIN tenant t ON t.id = r.tenant_id
   WHERE x.level > x.old_level AND x.notify = 1
   ORDER BY r.tenant_id, r.id;

  UPDATE requirement r JOIN tmp_rq_stale x ON x.id = r.id SET r.stale_level = x.level, r.updated_at = r.updated_at WHERE r.stale_level <> x.level;
  DROP TEMPORARY TABLE tmp_rq_stale;
END $$

DELIMITER ;
