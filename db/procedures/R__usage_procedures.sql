-- =====================================================================
-- Consumo de servicios externos por tenant (DNI e IA)
-- =====================================================================
DELIMITER $$

-- Registra una llamada al proveedor (la hace la app después de llamar; no requiere actor)
DROP PROCEDURE IF EXISTS sp_usage_record $$
CREATE PROCEDURE sp_usage_record(IN p_tenant_id BIGINT UNSIGNED, IN p_kind VARCHAR(20), IN p_provider VARCHAR(60),
  IN p_ok TINYINT(1), IN p_prompt BIGINT UNSIGNED, IN p_completion BIGINT UNSIGNED)
BEGIN
  INSERT INTO tenant_usage_daily (tenant_id, usage_date, kind, provider, calls, errors, prompt_tokens, completion_tokens)
  VALUES (p_tenant_id, UTC_DATE(), p_kind, IFNULL(NULLIF(p_provider, ''), '-'), 1, IF(p_ok = 1, 0, 1), IFNULL(p_prompt, 0), IFNULL(p_completion, 0))
  ON DUPLICATE KEY UPDATE calls = calls + 1, errors = errors + IF(p_ok = 1, 0, 1),
    prompt_tokens = prompt_tokens + IFNULL(p_prompt, 0), completion_tokens = completion_tokens + IFNULL(p_completion, 0);
END $$

-- Reporte para el Super Admin: consumo por tenant, mes, tipo y proveedor en un rango de fechas (máx. 366 días)
DROP PROCEDURE IF EXISTS sp_usage_report $$
CREATE PROCEDURE sp_usage_report(IN p_actor_id BIGINT UNSIGNED, IN p_from DATE, IN p_to DATE)
BEGIN
  CALL sp_assert_actor(p_actor_id, NULL, 'platform.usage.read');
  IF p_from IS NULL OR p_to IS NULL OR p_to < p_from OR DATEDIFF(p_to, p_from) > 366 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION';
  END IF;
  SELECT t.uuid AS tenant_uuid, t.slug, t.name, DATE_FORMAT(u.usage_date, '%Y-%m') AS month, u.kind, u.provider,
         SUM(u.calls) AS calls, SUM(u.errors) AS errors, SUM(u.prompt_tokens) AS prompt_tokens, SUM(u.completion_tokens) AS completion_tokens
    FROM tenant_usage_daily u
    JOIN tenant t ON t.id = u.tenant_id
   WHERE u.usage_date BETWEEN p_from AND p_to
   GROUP BY t.uuid, t.slug, t.name, month, u.kind, u.provider
   ORDER BY t.name, month, u.kind, u.provider;
END $$

DELIMITER ;
