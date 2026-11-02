-- =====================================================================
-- Versiones publicadas en el ambiente (cada ambiente tiene su base: staging, preprod, prod).
-- La API registra su versión al iniciar; si es nueva, avisa a todos los usuarios.
-- Estado: TESTING (en pruebas, por defecto en staging), CANDIDATE (candidata, preprod), STABLE (estable, prod) o
-- WITHDRAWN (retirada). El Super Admin lo ajusta (p. ej. marca como estable una versión validada).
-- =====================================================================

CREATE TABLE app_version (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  version      VARCHAR(12)     NOT NULL,
  environment  VARCHAR(12)     NOT NULL,
  status       VARCHAR(10)     NOT NULL,
  commit_sha   VARCHAR(40)     NULL,
  notes        JSON            NOT NULL,
  published_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  status_by    BIGINT UNSIGNED NULL,
  status_at    DATETIME(3)     NULL,
  UNIQUE KEY uq_app_version (environment, version),
  KEY ix_app_version_published (environment, published_at),
  CONSTRAINT ck_app_version_status CHECK (status IN ('TESTING','CANDIDATE','STABLE','WITHDRAWN')),
  CONSTRAINT fk_app_version_user FOREIGN KEY (status_by) REFERENCES app_user(id)
) ENGINE=InnoDB;
