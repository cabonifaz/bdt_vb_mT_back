-- =====================================================================
-- Responsables de un RQ: varios del equipo de selección, con historial de asignación
-- (assigned_at / unassigned_at) para medir la carga de trabajo y los indicadores por analista en
-- cualquier periodo. Reemplaza a requirement.owner_user_id (los responsables actuales se copian).
-- =====================================================================

CREATE TABLE requirement_owner (
  id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  requirement_id BIGINT UNSIGNED NOT NULL,
  user_id        BIGINT UNSIGNED NOT NULL,
  assigned_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  assigned_by    BIGINT UNSIGNED NULL,
  unassigned_at  DATETIME(3)     NULL,
  unassigned_by  BIGINT UNSIGNED NULL,
  active_key     TINYINT AS (IF(unassigned_at IS NULL, 1, NULL)) STORED,
  UNIQUE KEY uq_rq_owner_active (requirement_id, user_id, active_key),
  KEY ix_rq_owner_user (user_id, unassigned_at),
  CONSTRAINT fk_rqo_rq FOREIGN KEY (requirement_id) REFERENCES requirement(id),
  CONSTRAINT fk_rqo_user FOREIGN KEY (user_id) REFERENCES app_user(id)
) ENGINE=InnoDB;

INSERT INTO requirement_owner (requirement_id, user_id, assigned_at, assigned_by)
SELECT id, owner_user_id, created_at, created_by FROM requirement WHERE owner_user_id IS NOT NULL;

ALTER TABLE requirement DROP FOREIGN KEY fk_rq_owner;
ALTER TABLE requirement DROP COLUMN owner_user_id;
