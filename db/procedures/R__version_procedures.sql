-- =====================================================================
-- Versiones de la plataforma por ambiente: registro al desplegar, versión vigente (pública), historial y estado.
-- =====================================================================
DELIMITER $$

-- Registra la versión del despliegue. Devuelve is_new = 1 si es la primera vez que se publica en el ambiente
-- (un redespliegue de la misma versión solo actualiza el commit).
DROP PROCEDURE IF EXISTS sp_app_version_register $$
CREATE PROCEDURE sp_app_version_register(IN p_version VARCHAR(12), IN p_env VARCHAR(12), IN p_status VARCHAR(10),
  IN p_commit VARCHAR(40), IN p_notes JSON)
BEGIN
  DECLARE v_id BIGINT UNSIGNED;
  SELECT id INTO v_id FROM app_version WHERE environment = p_env AND version = p_version;
  IF v_id IS NULL THEN
    INSERT INTO app_version (version, environment, status, commit_sha, notes) VALUES (p_version, p_env, p_status, p_commit, p_notes);
    SELECT 1 AS is_new;
  ELSE
    UPDATE app_version SET commit_sha = IFNULL(p_commit, commit_sha), notes = p_notes WHERE id = v_id;
    SELECT 0 AS is_new;
  END IF;
END $$

-- Versión vigente del ambiente (la publicada más reciente)
DROP PROCEDURE IF EXISTS sp_app_version_current $$
CREATE PROCEDURE sp_app_version_current(IN p_env VARCHAR(12), IN p_version VARCHAR(12))
BEGIN
  SELECT JSON_OBJECT('version', version, 'environment', environment, 'status', status, 'commit', commit_sha,
           'notes', notes, 'publishedAt', published_at) AS doc
    FROM app_version WHERE environment = p_env AND version = p_version;
END $$

-- Historial del ambiente (cualquier usuario autenticado)
DROP PROCEDURE IF EXISTS sp_app_version_list $$
CREATE PROCEDURE sp_app_version_list(IN p_env VARCHAR(12))
BEGIN
  SELECT JSON_OBJECT('version', version, 'environment', environment, 'status', status, 'commit', commit_sha,
           'notes', notes, 'publishedAt', published_at, 'statusAt', status_at) AS doc
    FROM app_version WHERE environment = p_env ORDER BY published_at DESC LIMIT 100;
END $$

-- El Super Admin cambia el estado de una versión (estable, candidata, en pruebas o retirada)
DROP PROCEDURE IF EXISTS sp_app_version_set_status $$
CREATE PROCEDURE sp_app_version_set_status(IN p_actor_id BIGINT UNSIGNED, IN p_env VARCHAR(12), IN p_version VARCHAR(12),
  IN p_status VARCHAR(10), IN p_ip VARCHAR(45))
BEGIN
  CALL sp_assert_actor(p_actor_id, NULL, 'platform.releases.manage');
  IF p_status NOT IN ('TESTING','CANDIDATE','STABLE','WITHDRAWN') THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:VALIDATION'; END IF;
  UPDATE app_version SET status = p_status, status_by = p_actor_id, status_at = NOW(3) WHERE environment = p_env AND version = p_version;
  IF ROW_COUNT() = 0 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'BDT:NOT_FOUND'; END IF;
  CALL sp_audit_insert(NULL, p_actor_id, NULL, 'STATUS', 'app_version', CONCAT(p_env, ':', p_version), JSON_OBJECT('status', p_status), p_ip);
END $$

DELIMITER ;
