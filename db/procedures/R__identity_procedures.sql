-- =====================================================================
-- Consulta de identidad (DNI) contra un registro público
-- La consulta la hace la app; aquí se valida el permiso y se audita (sin guardar el número consultado).
-- =====================================================================
DELIMITER $$

DROP PROCEDURE IF EXISTS sp_identity_lookup_log $$
CREATE PROCEDURE sp_identity_lookup_log(IN p_actor_id BIGINT UNSIGNED, IN p_tenant_id BIGINT UNSIGNED,
  IN p_provider VARCHAR(30), IN p_result VARCHAR(30), IN p_ip VARCHAR(45))
BEGIN
  CALL sp_assert_actor(p_actor_id, p_tenant_id, 'talent.write');
  CALL sp_audit_insert(p_tenant_id, p_actor_id, NULL, 'IDENTITY_LOOKUP', 'identity', NULL,
                       JSON_OBJECT('provider', p_provider, 'result', p_result), p_ip);
END $$

DELIMITER ;
