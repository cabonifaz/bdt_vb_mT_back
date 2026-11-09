-- Envíos a IGDT (bandeja de selección): con la integración activa, BDT publica en IGDT el último estado de sus RQ
-- (con el avance de los postulantes) y los candidatos confirmados, que allá se vuelven solicitudes de ingreso.
-- - igdt_outbox: cola con reintentos. Solo hay una fila pendiente por entidad (pending_key); al enviarse se marca
--   sent_at. kind RQ = uuid del requerimiento; HIRE = uuid del candidato.
-- - tenant_integration.rq_synced_at: hasta cuándo se revisaron cambios de RQ y candidatos (la conciliación periódica
--   encola lo que cambió por cualquier vía desde esa marca).

CREATE TABLE igdt_outbox (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  tenant_id   BIGINT UNSIGNED NOT NULL,
  kind        VARCHAR(10)     NOT NULL,
  entity_uuid CHAR(36)        NOT NULL,
  attempts    INT UNSIGNED    NOT NULL DEFAULT 0,
  next_at     DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_error  VARCHAR(200)    NULL,
  created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  sent_at     DATETIME(3)     NULL,
  pending_key TINYINT AS (IF(sent_at IS NULL, 1, NULL)) STORED,
  UNIQUE KEY uq_igdt_outbox_pending (tenant_id, kind, entity_uuid, pending_key),
  KEY ix_igdt_outbox_next (pending_key, next_at),
  CONSTRAINT fk_igdt_outbox_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT ck_igdt_outbox_kind CHECK (kind IN ('RQ', 'HIRE'))
) ENGINE=InnoDB;

ALTER TABLE tenant_integration ADD COLUMN rq_synced_at DATETIME(3) NULL AFTER last_sync_at;
