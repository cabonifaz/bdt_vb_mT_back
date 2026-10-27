-- =====================================================================
-- Inicio del usuario: agenda, últimos talentos y carga del mes (mía y del área)
-- =====================================================================
DELIMITER $$

-- p_today: fecha local del tenant (la calcula la app con su zona horaria)
-- 3 result sets: próximas entrevistas (hoy + 7 días), últimos 10 talentos, resumen por mes (mes actual y 2 anteriores)
DROP PROCEDURE IF EXISTS sp_home_summary $$
CREATE PROCEDURE sp_home_summary(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED, IN p_today DATE, IN p_now DATETIME)
BEGIN
  DECLARE v_iv TINYINT(1);
  DECLARE v_from DATE;
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.read');
  SET v_iv = EXISTS (SELECT 1 FROM v_user_permission WHERE user_id = p_actor_id AND code = 'interview.read');
  SET v_from = DATE_SUB(DATE_FORMAT(p_today, '%Y-%m-01'), INTERVAL 2 MONTH);

  SELECT iv.uuid, t.uuid AS talent_uuid, t.first_name, t.last_name, t.second_last_name,
         ti.code AS type_code, st.label_i18n AS stage_label, ss.code AS status_code, ss.label_i18n AS status_label,
         cl.label_i18n AS client_label, iv.requirement_ref, iv.profile, iv.scheduled_date, iv.scheduled_time,
         iv.created_by = p_actor_id AS is_mine
    FROM interview iv
    JOIN talent t ON t.id = iv.talent_id
    JOIN master_item ti ON ti.id = iv.type_item_id
    JOIN master_item st ON st.id = iv.stage_item_id
    JOIN master_item ss ON ss.id = iv.status_item_id
    LEFT JOIN master_item cl ON cl.id = iv.client_item_id
   WHERE v_iv = 1 AND iv.tenant_id = p_tenant_id AND iv.deleted_at IS NULL
     AND iv.scheduled_date BETWEEN p_today AND DATE_ADD(p_today, INTERVAL 7 DAY)
     AND ss.code IN ('REGISTERED','IN_PROGRESS')
   ORDER BY iv.scheduled_date, iv.scheduled_time, iv.id
   LIMIT 30;

  SELECT t.uuid, t.first_name, t.last_name, t.second_last_name, t.headline, pf.uuid AS photo_uuid, t.created_at,
         t.created_at >= DATE_SUB(p_now, INTERVAL 24 HOUR) AS is_new
    FROM talent t
    LEFT JOIN file_object pf ON pf.id = t.photo_file_id AND pf.deleted_at IS NULL
   WHERE t.tenant_id = p_tenant_id AND t.deleted_at IS NULL AND t.status = 'ACTIVE'
   ORDER BY t.created_at DESC, t.id DESC
   LIMIT 10;

  -- planificadas = no canceladas; realizadas = finalizadas. "Mías" = registradas por el usuario.
  SELECT m.ym,
         IFNULL(SUM(iv.id IS NOT NULL AND iv.created_by = p_actor_id AND ss.code <> 'CANCELLED'), 0) AS my_planned,
         IFNULL(SUM(iv.id IS NOT NULL AND iv.created_by = p_actor_id AND ss.code = 'FINISHED'), 0) AS my_done,
         IFNULL(SUM(iv.id IS NOT NULL AND ss.code <> 'CANCELLED'), 0) AS area_planned,
         IFNULL(SUM(iv.id IS NOT NULL AND ss.code = 'FINISHED'), 0) AS area_done,
         (SELECT COUNT(*) FROM talent t WHERE t.tenant_id = p_tenant_id AND t.deleted_at IS NULL
             AND DATE_FORMAT(t.created_at, '%Y-%m') = m.ym) AS talents_new,
         (SELECT COUNT(*) FROM talent t WHERE t.tenant_id = p_tenant_id AND t.deleted_at IS NULL AND t.created_by = p_actor_id
             AND DATE_FORMAT(t.created_at, '%Y-%m') = m.ym) AS my_talents_new
    FROM (SELECT DATE_FORMAT(v_from, '%Y-%m') AS ym UNION ALL
          SELECT DATE_FORMAT(DATE_ADD(v_from, INTERVAL 1 MONTH), '%Y-%m') UNION ALL
          SELECT DATE_FORMAT(DATE_ADD(v_from, INTERVAL 2 MONTH), '%Y-%m')) m
    LEFT JOIN interview iv ON v_iv = 1 AND iv.tenant_id = p_tenant_id AND iv.deleted_at IS NULL
         AND DATE_FORMAT(iv.scheduled_date, '%Y-%m') = m.ym
    LEFT JOIN master_item ss ON ss.id = iv.status_item_id
   GROUP BY m.ym
   ORDER BY m.ym;
END $$

DELIMITER ;
