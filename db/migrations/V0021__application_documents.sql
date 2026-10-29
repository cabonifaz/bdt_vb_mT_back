-- =====================================================================
-- Documentos de postulación
-- - document_rule: maestro del tenant. Por tipo de documento (DOCUMENT_TYPE): si se pide por defecto como
--   obligatorio u opcional, si es reutilizable entre RQ, su vigencia en días y cómo se valida
--   (MANUAL o AUTO_CUL: contra el servicio público del MTPE con el RUC del tenant).
-- - requirement_document: qué documentos pide cada RQ (parte del maestro; se ajusta por RQ).
-- - talent_document: documento presentado por un postulante. Si el tipo no es reutilizable queda atado a la
--   postulación (candidate_id). Estado: PENDING, VERIFIED, REJECTED (vencido se calcula con expires_at).
-- - requirement_candidate_event: línea de tiempo del postulante dentro del RQ.
-- Faltantes o sin verificar solo se avisan (no bloquean etapas).
-- =====================================================================

INSERT INTO master_item (uuid, master_table_id, code, label_i18n, sort_order)
SELECT UUID(), t.id, 'CUL', JSON_OBJECT('es','Certificado Único Laboral (CUL)','en','Single Labor Certificate (CUL)'), 8
  FROM master_table t WHERE t.code = 'DOCUMENT_TYPE';

CREATE TABLE document_rule (
  id                    BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  tenant_id             BIGINT UNSIGNED NOT NULL,
  document_type_item_id BIGINT UNSIGNED NOT NULL,
  default_mode          VARCHAR(10)     NOT NULL DEFAULT 'REQUIRED',
  reusable              TINYINT(1)      NOT NULL DEFAULT 1,
  validity_days         INT             NULL,
  verification          VARCHAR(10)     NOT NULL DEFAULT 'MANUAL',
  is_active             TINYINT(1)      NOT NULL DEFAULT 1,
  sort_order            INT             NOT NULL DEFAULT 0,
  created_at            DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by            BIGINT UNSIGNED NULL,
  updated_at            DATETIME(3)     NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by            BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_doc_rule (tenant_id, document_type_item_id),
  CONSTRAINT ck_doc_rule_mode CHECK (default_mode IN ('REQUIRED','OPTIONAL')),
  CONSTRAINT ck_doc_rule_verif CHECK (verification IN ('MANUAL','AUTO_CUL')),
  CONSTRAINT ck_doc_rule_validity CHECK (validity_days IS NULL OR validity_days BETWEEN 1 AND 3650),
  CONSTRAINT fk_doc_rule_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_doc_rule_type FOREIGN KEY (document_type_item_id) REFERENCES master_item(id)
) ENGINE=InnoDB;

CREATE TABLE requirement_document (
  requirement_id        BIGINT UNSIGNED NOT NULL,
  document_type_item_id BIGINT UNSIGNED NOT NULL,
  mode                  VARCHAR(10)     NOT NULL,
  PRIMARY KEY (requirement_id, document_type_item_id),
  CONSTRAINT ck_rq_doc_mode CHECK (mode IN ('REQUIRED','OPTIONAL')),
  CONSTRAINT fk_rq_doc_rq FOREIGN KEY (requirement_id) REFERENCES requirement(id) ON DELETE CASCADE,
  CONSTRAINT fk_rq_doc_type FOREIGN KEY (document_type_item_id) REFERENCES master_item(id)
) ENGINE=InnoDB;

CREATE TABLE talent_document (
  id                    BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid                  CHAR(36)        NOT NULL,
  tenant_id             BIGINT UNSIGNED NOT NULL,
  talent_id             BIGINT UNSIGNED NOT NULL,
  document_type_item_id BIGINT UNSIGNED NOT NULL,
  file_id               BIGINT UNSIGNED NOT NULL,
  candidate_id          BIGINT UNSIGNED NULL,
  issued_at             DATE            NULL,
  expires_at            DATE            NULL,
  status                VARCHAR(10)     NOT NULL DEFAULT 'PENDING',
  method                VARCHAR(10)     NULL,
  reason                VARCHAR(500)    NULL,
  auto_result           JSON            NULL,
  verified_by           BIGINT UNSIGNED NULL,
  verified_at           DATETIME(3)     NULL,
  created_at            DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by            BIGINT UNSIGNED NULL,
  deleted_at            DATETIME(3)     NULL,
  deleted_by            BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_talent_document_uuid (uuid),
  KEY ix_talent_document (talent_id, document_type_item_id, deleted_at),
  CONSTRAINT ck_talent_doc_status CHECK (status IN ('PENDING','VERIFIED','REJECTED')),
  CONSTRAINT ck_talent_doc_method CHECK (method IS NULL OR method IN ('MANUAL','AUTO')),
  CONSTRAINT fk_tdoc_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_tdoc_talent FOREIGN KEY (talent_id) REFERENCES talent(id),
  CONSTRAINT fk_tdoc_type FOREIGN KEY (document_type_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_tdoc_file FOREIGN KEY (file_id) REFERENCES file_object(id),
  CONSTRAINT fk_tdoc_candidate FOREIGN KEY (candidate_id) REFERENCES requirement_candidate(id),
  CONSTRAINT fk_tdoc_verifier FOREIGN KEY (verified_by) REFERENCES app_user(id)
) ENGINE=InnoDB;

CREATE TABLE requirement_candidate_event (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  candidate_id BIGINT UNSIGNED NOT NULL,
  type         VARCHAR(20)     NOT NULL,
  data         JSON            NULL,
  created_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by   BIGINT UNSIGNED NULL,
  KEY ix_cand_event (candidate_id, created_at),
  CONSTRAINT fk_cand_event_candidate FOREIGN KEY (candidate_id) REFERENCES requirement_candidate(id),
  CONSTRAINT fk_cand_event_user FOREIGN KEY (created_by) REFERENCES app_user(id)
) ENGINE=InnoDB;

-- historial inicial: alta en el RQ, etapa actual (si ya avanzó) y renuncias registradas
INSERT INTO requirement_candidate_event (candidate_id, type, data, created_at, created_by)
SELECT rc.id, 'ADDED', JSON_OBJECT('stage', s.code), rc.created_at, rc.created_by
  FROM requirement_candidate rc JOIN master_item s ON s.id = rc.stage_item_id WHERE rc.deleted_at IS NULL;
INSERT INTO requirement_candidate_event (candidate_id, type, data, created_at, created_by)
SELECT rc.id, 'STAGE', JSON_OBJECT('from', 'OBSERVED', 'to', s.code), IFNULL(rc.updated_at, rc.created_at), rc.updated_by
  FROM requirement_candidate rc JOIN master_item s ON s.id = rc.stage_item_id
 WHERE rc.deleted_at IS NULL AND s.code <> 'OBSERVED';
INSERT INTO requirement_candidate_event (candidate_id, type, data, created_at, created_by)
SELECT rc.id, 'RESIGNED', NULL, rc.resigned_at, rc.updated_by FROM requirement_candidate rc WHERE rc.resigned_at IS NOT NULL;
