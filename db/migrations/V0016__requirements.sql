-- =====================================================================
-- Requerimientos (RQ): módulo propio de BDT (multi-tenant), basado en el de AutFMI.
-- RQ -> vacantes (perfil, cantidad, tarifas, skills, carreras) -> candidatos con etapa (pipeline Kanban).
-- Estados del RQ y etapas del candidato son maestros globales (legacy_id = id del sistema anterior).
-- Tarifas y bandas salariales: solo con rq.rates.read (el reclutador no las ve).
-- =====================================================================

INSERT INTO master_table (code, name_i18n, scope, is_system) VALUES
 ('RQ_STATUS',         JSON_OBJECT('es','Estados de requerimiento','en','Requirement statuses'), 'GLOBAL', 1),
 ('RQ_PROFILE',        JSON_OBJECT('es','Perfiles de vacante','en','Vacancy profiles'), 'BOTH', 0),
 ('RQ_FILE_TYPE',      JSON_OBJECT('es','Tipos de archivo de requerimiento','en','Requirement file types'), 'BOTH', 0),
 ('TIME_UNIT',         JSON_OBJECT('es','Unidades de tiempo','en','Time units'), 'GLOBAL', 1),
 ('WORK_MODALITY',     JSON_OBJECT('es','Modalidades de trabajo','en','Work modalities'), 'GLOBAL', 1),
 ('CONTRACT_MODALITY', JSON_OBJECT('es','Modalidades de contrato','en','Contract modalities'), 'BOTH', 0),
 ('CANDIDATE_STAGE',   JSON_OBJECT('es','Etapas del candidato','en','Candidate stages'), 'GLOBAL', 1);

-- str1: grupo. RQ_STATUS: OPEN | CLOSED (columna "Cerrados" del tablero). CONTRACT_MODALITY: PAYROLL | RXH.
INSERT INTO master_item (uuid, master_table_id, code, label_i18n, str1, sort_order, legacy_id)
SELECT UUID(), t.id, v.code, v.label, v.s1, v.ord, v.legacy
FROM master_table t
JOIN (
  SELECT 'RQ_STATUS' tbl, 'REGISTERED' code, JSON_OBJECT('es','Registrado','en','Registered') label, 'OPEN' s1, 1 ord, 1 legacy UNION ALL
  SELECT 'RQ_STATUS', 'ASSIGNED', JSON_OBJECT('es','Asignado','en','Assigned'), 'OPEN', 2, 2 UNION ALL
  SELECT 'RQ_STATUS', 'IN_SELECTION', JSON_OBJECT('es','En selección','en','In selection'), 'OPEN', 3, 4 UNION ALL
  SELECT 'RQ_STATUS', 'IN_PRODUCTION', JSON_OBJECT('es','En producción','en','In production'), 'OPEN', 4, 5 UNION ALL
  SELECT 'RQ_STATUS', 'ATTENDED', JSON_OBJECT('es','Atendido','en','Fulfilled'), 'CLOSED', 5, 3 UNION ALL
  SELECT 'RQ_STATUS', 'FINISHED', JSON_OBJECT('es','Terminado','en','Finished'), 'CLOSED', 6, 7 UNION ALL
  SELECT 'RQ_STATUS', 'LOST', JSON_OBJECT('es','Perdido','en','Lost'), 'CLOSED', 7, 6 UNION ALL
  SELECT 'RQ_STATUS', 'CANCELLED', JSON_OBJECT('es','Cancelado','en','Cancelled'), 'CLOSED', 8, 8 UNION ALL
  SELECT 'TIME_UNIT', 'DAYS', JSON_OBJECT('es','Días','en','Days'), NULL, 1, 1 UNION ALL
  SELECT 'TIME_UNIT', 'WEEKS', JSON_OBJECT('es','Semanas','en','Weeks'), NULL, 2, 2 UNION ALL
  SELECT 'TIME_UNIT', 'MONTHS', JSON_OBJECT('es','Meses','en','Months'), NULL, 3, 3 UNION ALL
  SELECT 'WORK_MODALITY', 'ON_SITE', JSON_OBJECT('es','Presencial','en','On-site'), NULL, 1, 1 UNION ALL
  SELECT 'WORK_MODALITY', 'REMOTE', JSON_OBJECT('es','Remoto','en','Remote'), NULL, 2, 2 UNION ALL
  SELECT 'WORK_MODALITY', 'HYBRID', JSON_OBJECT('es','Híbrido','en','Hybrid'), NULL, 3, 3 UNION ALL
  SELECT 'CONTRACT_MODALITY', 'SERVICES', JSON_OBJECT('es','Locación de servicios','en','Service contract'), 'RXH', 1, 1 UNION ALL
  SELECT 'CONTRACT_MODALITY', 'PAYROLL_FULL', JSON_OBJECT('es','Planilla completa','en','Full payroll'), 'PAYROLL', 2, 2 UNION ALL
  SELECT 'CONTRACT_MODALITY', 'PAYROLL_PARTIAL', JSON_OBJECT('es','Planilla parcial','en','Partial payroll'), 'PAYROLL', 3, 3 UNION ALL
  SELECT 'CONTRACT_MODALITY', 'PRE_INTERNSHIP', JSON_OBJECT('es','Prácticas preprofesionales','en','Pre-professional internship'), 'PAYROLL', 4, 4 UNION ALL
  SELECT 'CONTRACT_MODALITY', 'INTERNSHIP', JSON_OBJECT('es','Prácticas profesionales','en','Professional internship'), 'PAYROLL', 5, 5 UNION ALL
  SELECT 'CANDIDATE_STAGE', 'OBSERVED', JSON_OBJECT('es','Observado','en','Under review'), NULL, 1, 1 UNION ALL
  SELECT 'CANDIDATE_STAGE', 'DATA_COMPLETE', JSON_OBJECT('es','Datos completos','en','Data complete'), NULL, 2, 2 UNION ALL
  SELECT 'CANDIDATE_STAGE', 'INTERVIEW', JSON_OBJECT('es','En entrevista','en','Interviewing'), NULL, 3, 3 UNION ALL
  SELECT 'CANDIDATE_STAGE', 'CONFIRMED', JSON_OBJECT('es','Confirmado','en','Confirmed'), NULL, 4, 4 UNION ALL
  SELECT 'RQ_PROFILE', 'JAVA_DEV', JSON_OBJECT('es','Desarrollador Java','en','Java developer'), NULL, 1, NULL UNION ALL
  SELECT 'RQ_PROFILE', 'FRONTEND_DEV', JSON_OBJECT('es','Desarrollador Frontend','en','Frontend developer'), NULL, 2, NULL UNION ALL
  SELECT 'RQ_PROFILE', 'QA_ANALYST', JSON_OBJECT('es','Analista QA','en','QA analyst'), NULL, 3, NULL UNION ALL
  SELECT 'RQ_PROFILE', 'BUSINESS_ANALYST', JSON_OBJECT('es','Analista de negocio','en','Business analyst'), NULL, 4, NULL UNION ALL
  SELECT 'RQ_PROFILE', 'PROJECT_MANAGER', JSON_OBJECT('es','Jefe de proyecto','en','Project manager'), NULL, 5, NULL UNION ALL
  SELECT 'RQ_FILE_TYPE', 'TOR', JSON_OBJECT('es','Términos de referencia','en','Terms of reference'), NULL, 1, NULL UNION ALL
  SELECT 'RQ_FILE_TYPE', 'PROPOSAL', JSON_OBJECT('es','Propuesta','en','Proposal'), NULL, 2, NULL UNION ALL
  SELECT 'RQ_FILE_TYPE', 'OTHER', JSON_OBJECT('es','Otro','en','Other'), NULL, 3, NULL
) v ON v.tbl = t.code;

-- Contactos del cliente (cliente = master_item de CLIENT)
CREATE TABLE client_contact (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid             CHAR(36)        NOT NULL,
  tenant_id        BIGINT UNSIGNED NOT NULL,
  client_item_id   BIGINT UNSIGNED NOT NULL,
  first_name       VARCHAR(100)    NOT NULL,
  last_name        VARCHAR(100)    NOT NULL,
  second_last_name VARCHAR(100)    NULL,
  position         VARCHAR(150)    NULL,
  phone            VARCHAR(30)     NULL,
  phone2           VARCHAR(30)     NULL,
  email            VARCHAR(254)    NULL,
  email2           VARCHAR(254)    NULL,
  created_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by       BIGINT UNSIGNED NULL,
  updated_at       DATETIME(3)     NULL ON UPDATE CURRENT_TIMESTAMP(3),
  deleted_at       DATETIME(3)     NULL,
  UNIQUE KEY uq_client_contact_uuid (uuid),
  KEY ix_client_contact (tenant_id, client_item_id),
  CONSTRAINT fk_cc_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_cc_client FOREIGN KEY (client_item_id) REFERENCES master_item(id)
) ENGINE=InnoDB;

-- Correlativo del código autogenerado por tenant y año (RQ-2026-0001)
CREATE TABLE requirement_sequence (
  tenant_id BIGINT UNSIGNED NOT NULL,
  year      SMALLINT        NOT NULL,
  last_no   INT UNSIGNED    NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, year)
) ENGINE=InnoDB;

CREATE TABLE requirement (
  id                        BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid                      CHAR(36)        NOT NULL,
  tenant_id                 BIGINT UNSIGNED NOT NULL,
  code                      VARCHAR(40)     NOT NULL,
  client_item_id            BIGINT UNSIGNED NOT NULL,
  title                     VARCHAR(200)    NOT NULL,
  description               VARCHAR(2000)   NULL,
  status_item_id            BIGINT UNSIGNED NOT NULL,
  request_date              DATE            NOT NULL,
  due_date                  DATE            NOT NULL,
  duration_value            DECIMAL(8,2)    NULL,      -- duración del servicio (opcional)
  duration_unit_item_id     BIGINT UNSIGNED NULL,
  contract_value            DECIMAL(8,2)    NULL,      -- duración del contrato
  contract_unit_item_id     BIGINT UNSIGNED NULL,
  work_modality_item_id     BIGINT UNSIGNED NULL,
  needs_emo                 TINYINT(1)      NOT NULL DEFAULT 0,
  needs_sctr                TINYINT(1)      NOT NULL DEFAULT 0,
  needs_laptop              TINYINT(1)      NOT NULL DEFAULT 0,
  owner_user_id             BIGINT UNSIGNED NULL,      -- responsable (gestor) del RQ
  board_order               INT             NOT NULL DEFAULT 0, -- orden dentro de la columna del tablero
  version                   INT             NOT NULL DEFAULT 1,
  active_key                TINYINT AS (IF(deleted_at IS NULL, 1, NULL)) STORED,
  created_at                DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by                BIGINT UNSIGNED NULL,
  updated_at                DATETIME(3)     NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by                BIGINT UNSIGNED NULL,
  deleted_at                DATETIME(3)     NULL,
  deleted_by                BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_requirement_uuid (uuid),
  UNIQUE KEY uq_requirement_code (tenant_id, code, active_key),
  KEY ix_requirement_board (tenant_id, status_item_id, due_date),
  CONSTRAINT fk_rq_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_rq_client FOREIGN KEY (client_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_rq_status FOREIGN KEY (status_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_rq_owner FOREIGN KEY (owner_user_id) REFERENCES app_user(id),
  CONSTRAINT ck_rq_dates CHECK (due_date >= request_date)
) ENGINE=InnoDB;

CREATE TABLE requirement_contact (
  requirement_id BIGINT UNSIGNED NOT NULL,
  contact_id     BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (requirement_id, contact_id),
  CONSTRAINT fk_rqc_rq FOREIGN KEY (requirement_id) REFERENCES requirement(id) ON DELETE CASCADE,
  CONSTRAINT fk_rqc_contact FOREIGN KEY (contact_id) REFERENCES client_contact(id)
) ENGINE=InnoDB;

-- Banda salarial por modalidad de contrato permitida (montos mínimos y máximos)
CREATE TABLE requirement_billing (
  requirement_id        BIGINT UNSIGNED NOT NULL,
  contract_item_id      BIGINT UNSIGNED NOT NULL,
  currency_item_id      BIGINT UNSIGNED NOT NULL,
  base_min DECIMAL(12,2) NULL, base_max DECIMAL(12,2) NULL,
  mobility_min DECIMAL(12,2) NULL, mobility_max DECIMAL(12,2) NULL,
  monthly_min DECIMAL(12,2) NULL, monthly_max DECIMAL(12,2) NULL,
  quarterly_min DECIMAL(12,2) NULL, quarterly_max DECIMAL(12,2) NULL,
  semiannual_min DECIMAL(12,2) NULL, semiannual_max DECIMAL(12,2) NULL,
  PRIMARY KEY (requirement_id, contract_item_id),
  CONSTRAINT fk_rqb_rq FOREIGN KEY (requirement_id) REFERENCES requirement(id) ON DELETE CASCADE,
  CONSTRAINT fk_rqb_contract FOREIGN KEY (contract_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_rqb_currency FOREIGN KEY (currency_item_id) REFERENCES master_item(id)
) ENGINE=InnoDB;

CREATE TABLE requirement_vacancy (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid             CHAR(36)        NOT NULL,
  requirement_id   BIGINT UNSIGNED NOT NULL,
  profile_item_id  BIGINT UNSIGNED NOT NULL,
  quantity         INT UNSIGNED    NOT NULL DEFAULT 1,
  rate_initial     DECIMAL(12,2)   NULL,
  rate_final       DECIMAL(12,2)   NULL,
  sort_order       INT             NOT NULL DEFAULT 0,
  created_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  deleted_at       DATETIME(3)     NULL,
  UNIQUE KEY uq_vacancy_uuid (uuid),
  KEY ix_vacancy_rq (requirement_id),
  CONSTRAINT fk_vac_rq FOREIGN KEY (requirement_id) REFERENCES requirement(id),
  CONSTRAINT fk_vac_profile FOREIGN KEY (profile_item_id) REFERENCES master_item(id),
  CONSTRAINT ck_vac_qty CHECK (quantity >= 1)
) ENGINE=InnoDB;

CREATE TABLE vacancy_skill (
  vacancy_id    BIGINT UNSIGNED NOT NULL,
  skill_item_id BIGINT UNSIGNED NOT NULL,
  years         DECIMAL(4,1)    NULL,
  optional      TINYINT(1)      NOT NULL DEFAULT 0,
  PRIMARY KEY (vacancy_id, skill_item_id),
  CONSTRAINT fk_vs_vacancy FOREIGN KEY (vacancy_id) REFERENCES requirement_vacancy(id) ON DELETE CASCADE,
  CONSTRAINT fk_vs_skill FOREIGN KEY (skill_item_id) REFERENCES master_item(id)
) ENGINE=InnoDB;

CREATE TABLE vacancy_career (
  id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  vacancy_id     BIGINT UNSIGNED NOT NULL,
  career         VARCHAR(150)    NOT NULL,
  grade_item_id  BIGINT UNSIGNED NULL,
  optional       TINYINT(1)      NOT NULL DEFAULT 0,
  CONSTRAINT fk_vcar_vacancy FOREIGN KEY (vacancy_id) REFERENCES requirement_vacancy(id) ON DELETE CASCADE,
  CONSTRAINT fk_vcar_grade FOREIGN KEY (grade_item_id) REFERENCES master_item(id)
) ENGINE=InnoDB;

-- Candidatos del RQ (pipeline): talento postulado a una vacante, con su etapa
CREATE TABLE requirement_candidate (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid            CHAR(36)        NOT NULL,
  requirement_id  BIGINT UNSIGNED NOT NULL,
  vacancy_id      BIGINT UNSIGNED NOT NULL,
  talent_id       BIGINT UNSIGNED NOT NULL,
  stage_item_id   BIGINT UNSIGNED NOT NULL,
  board_order     INT             NOT NULL DEFAULT 0,
  notes           VARCHAR(500)    NULL,
  created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by      BIGINT UNSIGNED NULL,
  updated_at      DATETIME(3)     NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by      BIGINT UNSIGNED NULL,
  deleted_at      DATETIME(3)     NULL,
  active_key      TINYINT AS (IF(deleted_at IS NULL, 1, NULL)) STORED,
  UNIQUE KEY uq_candidate_uuid (uuid),
  UNIQUE KEY uq_candidate_talent (requirement_id, talent_id, active_key),
  KEY ix_candidate_talent (talent_id),
  CONSTRAINT fk_cand_rq FOREIGN KEY (requirement_id) REFERENCES requirement(id),
  CONSTRAINT fk_cand_vacancy FOREIGN KEY (vacancy_id) REFERENCES requirement_vacancy(id),
  CONSTRAINT fk_cand_talent FOREIGN KEY (talent_id) REFERENCES talent(id),
  CONSTRAINT fk_cand_stage FOREIGN KEY (stage_item_id) REFERENCES master_item(id)
) ENGINE=InnoDB;

-- Entrevistas vinculadas al RQ (requirement_ref queda para registros anteriores)
ALTER TABLE interview ADD COLUMN requirement_id BIGINT UNSIGNED NULL AFTER requirement_ref,
  ADD CONSTRAINT fk_interview_rq FOREIGN KEY (requirement_id) REFERENCES requirement(id);

INSERT INTO permission (code, description) VALUES
 ('rq.read',       'Consultar requerimientos y su tablero'),
 ('rq.write',      'Crear y editar requerimientos, vacantes, contactos y estados'),
 ('rq.candidates', 'Asignar talentos a requerimientos y moverlos entre etapas'),
 ('rq.rates.read', 'Ver tarifas y bandas salariales de los requerimientos');
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p
 ON (p.code = 'rq.read' AND r.code IN ('TENANT_ADMIN','RECRUITER','COMMERCIAL','VIEWER'))
 OR (p.code IN ('rq.write','rq.rates.read') AND r.code IN ('TENANT_ADMIN','COMMERCIAL'))
 OR (p.code = 'rq.candidates' AND r.code IN ('TENANT_ADMIN','RECRUITER','COMMERCIAL'));
