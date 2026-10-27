-- =====================================================================
-- Módulo de talentos
-- Sensibles cifrados en la app: documento, correo, teléfono y expectativas
-- salariales (rates_enc). Búsqueda exacta por índice ciego (*_bidx).
-- active_key = 1 si no está borrado (NULL si lo está) para unicidad con borrado lógico.
-- version: control de concurrencia optimista (ediciones offline).
-- =====================================================================

CREATE TABLE talent (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid             CHAR(36)      NOT NULL,
  tenant_id        BIGINT UNSIGNED NOT NULL,
  first_name       VARCHAR(100)  NOT NULL,
  last_name        VARCHAR(100)  NOT NULL,
  second_last_name VARCHAR(100)  NULL,
  headline         VARCHAR(150)  NULL,            -- puesto / perfil
  doc_type_item_id BIGINT UNSIGNED NULL,
  doc_number_enc   VARBINARY(256) NULL,
  doc_number_bidx  CHAR(64)      NULL,
  email_enc        VARBINARY(512) NULL,
  email_bidx       CHAR(64)      NULL,
  phone_enc        VARBINARY(256) NULL,
  country_item_id  BIGINT UNSIGNED NULL,
  city             VARCHAR(120)  NULL,
  linkedin_url     VARCHAR(300)  NULL,
  github_url       VARCHAR(300)  NULL,
  summary          TEXT          NULL,
  availability     VARCHAR(150)  NULL,
  has_equipment    TINYINT(1)    NULL,
  rates_enc        VARBINARY(2048) NULL,          -- JSON cifrado: modalidad, montos y monedas
  photo_file_id    BIGINT UNSIGNED NULL,
  cv_file_id       BIGINT UNSIGNED NULL,
  avg_rating       DECIMAL(3,2)  NULL,
  status           VARCHAR(20)   NOT NULL DEFAULT 'ACTIVE',  -- ACTIVE | ARCHIVED
  source           VARCHAR(20)   NOT NULL DEFAULT 'MANUAL',  -- MANUAL | LINK | IMPORT | API
  legacy_id        INT           NULL,
  version          INT           NOT NULL DEFAULT 1,
  search_name      VARCHAR(320) AS (LOWER(CONCAT_WS(' ', first_name, last_name, second_last_name))) STORED,
  active_key       TINYINT AS (IF(deleted_at IS NULL, 1, NULL)) STORED,
  created_at       DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by       BIGINT UNSIGNED NULL,
  updated_at       DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by       BIGINT UNSIGNED NULL,
  deleted_at       DATETIME(3)   NULL,
  deleted_by       BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_talent_uuid (uuid),
  UNIQUE KEY uq_talent_email (tenant_id, email_bidx, active_key),
  UNIQUE KEY uq_talent_doc (tenant_id, doc_number_bidx, active_key),
  KEY ix_talent_tenant (tenant_id, status, deleted_at),
  KEY ix_talent_name (tenant_id, search_name),
  KEY ix_talent_legacy (tenant_id, legacy_id),
  CONSTRAINT fk_talent_tenant  FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_talent_doctype FOREIGN KEY (doc_type_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_talent_country FOREIGN KEY (country_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_talent_photo   FOREIGN KEY (photo_file_id) REFERENCES file_object(id),
  CONSTRAINT fk_talent_cv      FOREIGN KEY (cv_file_id) REFERENCES file_object(id),
  CONSTRAINT ck_talent_status  CHECK (status IN ('ACTIVE','ARCHIVED'))
) ENGINE=InnoDB;

CREATE TABLE talent_tech_skill (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid          CHAR(36)      NOT NULL,
  tenant_id     BIGINT UNSIGNED NOT NULL,
  talent_id     BIGINT UNSIGNED NOT NULL,
  skill_item_id BIGINT UNSIGNED NOT NULL,
  years         DECIMAL(4,1)  NOT NULL DEFAULT 0,
  active_key    TINYINT AS (IF(deleted_at IS NULL, 1, NULL)) STORED,
  created_at    DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by    BIGINT UNSIGNED NULL,
  updated_at    DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by    BIGINT UNSIGNED NULL,
  deleted_at    DATETIME(3)   NULL,
  deleted_by    BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_tts_uuid (uuid),
  UNIQUE KEY uq_tts_skill (talent_id, skill_item_id, active_key),
  KEY ix_tts_skill (tenant_id, skill_item_id),
  CONSTRAINT fk_tts_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_tts_talent FOREIGN KEY (talent_id) REFERENCES talent(id),
  CONSTRAINT fk_tts_skill  FOREIGN KEY (skill_item_id) REFERENCES master_item(id),
  CONSTRAINT ck_tts_years  CHECK (years >= 0 AND years <= 60)
) ENGINE=InnoDB;

CREATE TABLE talent_soft_skill (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid          CHAR(36)      NOT NULL,
  tenant_id     BIGINT UNSIGNED NOT NULL,
  talent_id     BIGINT UNSIGNED NOT NULL,
  skill_item_id BIGINT UNSIGNED NOT NULL,
  active_key    TINYINT AS (IF(deleted_at IS NULL, 1, NULL)) STORED,
  created_at    DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by    BIGINT UNSIGNED NULL,
  updated_at    DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by    BIGINT UNSIGNED NULL,
  deleted_at    DATETIME(3)   NULL,
  deleted_by    BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_tss_uuid (uuid),
  UNIQUE KEY uq_tss_skill (talent_id, skill_item_id, active_key),
  CONSTRAINT fk_tss_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_tss_talent FOREIGN KEY (talent_id) REFERENCES talent(id),
  CONSTRAINT fk_tss_skill  FOREIGN KEY (skill_item_id) REFERENCES master_item(id)
) ENGINE=InnoDB;

CREATE TABLE talent_experience (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid          CHAR(36)      NOT NULL,
  tenant_id     BIGINT UNSIGNED NOT NULL,
  talent_id     BIGINT UNSIGNED NOT NULL,
  company       VARCHAR(150)  NOT NULL,
  position      VARCHAR(150)  NOT NULL,
  start_date    DATE          NOT NULL,
  end_date      DATE          NULL,
  is_current    TINYINT(1)    NOT NULL DEFAULT 0,
  functions     TEXT          NULL,
  created_at    DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by    BIGINT UNSIGNED NULL,
  updated_at    DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by    BIGINT UNSIGNED NULL,
  deleted_at    DATETIME(3)   NULL,
  deleted_by    BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_texp_uuid (uuid),
  KEY ix_texp_talent (talent_id, deleted_at),
  CONSTRAINT fk_texp_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_texp_talent FOREIGN KEY (talent_id) REFERENCES talent(id),
  CONSTRAINT ck_texp_dates  CHECK (end_date IS NULL OR end_date >= start_date)
) ENGINE=InnoDB;

CREATE TABLE talent_education (
  id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid           CHAR(36)      NOT NULL,
  tenant_id      BIGINT UNSIGNED NOT NULL,
  talent_id      BIGINT UNSIGNED NOT NULL,
  institution    VARCHAR(150)  NOT NULL,
  career         VARCHAR(150)  NOT NULL,
  grade_item_id  BIGINT UNSIGNED NULL,
  start_date     DATE          NULL,
  end_date       DATE          NULL,
  is_current     TINYINT(1)    NOT NULL DEFAULT 0,
  date_precision VARCHAR(5)    NOT NULL DEFAULT 'MONTH', -- YEAR | MONTH
  created_at     DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by     BIGINT UNSIGNED NULL,
  updated_at     DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by     BIGINT UNSIGNED NULL,
  deleted_at     DATETIME(3)   NULL,
  deleted_by     BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_tedu_uuid (uuid),
  KEY ix_tedu_talent (talent_id, deleted_at),
  CONSTRAINT fk_tedu_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_tedu_talent FOREIGN KEY (talent_id) REFERENCES talent(id),
  CONSTRAINT fk_tedu_grade  FOREIGN KEY (grade_item_id) REFERENCES master_item(id),
  CONSTRAINT ck_tedu_prec   CHECK (date_precision IN ('YEAR','MONTH'))
) ENGINE=InnoDB;

CREATE TABLE talent_language (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid             CHAR(36)      NOT NULL,
  tenant_id        BIGINT UNSIGNED NOT NULL,
  talent_id        BIGINT UNSIGNED NOT NULL,
  language_item_id BIGINT UNSIGNED NOT NULL,
  level_item_id    BIGINT UNSIGNED NOT NULL,
  stars            TINYINT       NOT NULL DEFAULT 0,
  active_key       TINYINT AS (IF(deleted_at IS NULL, 1, NULL)) STORED,
  created_at       DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by       BIGINT UNSIGNED NULL,
  updated_at       DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by       BIGINT UNSIGNED NULL,
  deleted_at       DATETIME(3)   NULL,
  deleted_by       BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_tlang_uuid (uuid),
  UNIQUE KEY uq_tlang_lang (talent_id, language_item_id, active_key),
  CONSTRAINT fk_tlang_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_tlang_talent FOREIGN KEY (talent_id) REFERENCES talent(id),
  CONSTRAINT fk_tlang_lang   FOREIGN KEY (language_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_tlang_level  FOREIGN KEY (level_item_id) REFERENCES master_item(id),
  CONSTRAINT ck_tlang_stars  CHECK (stars BETWEEN 0 AND 5)
) ENGINE=InnoDB;

CREATE TABLE talent_feedback (
  id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid           CHAR(36)      NOT NULL,
  tenant_id      BIGINT UNSIGNED NOT NULL,
  talent_id      BIGINT UNSIGNED NOT NULL,
  author_user_id BIGINT UNSIGNED NOT NULL,
  stars          TINYINT       NOT NULL,
  comment        TEXT          NOT NULL,
  created_at     DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by     BIGINT UNSIGNED NULL,
  updated_at     DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by     BIGINT UNSIGNED NULL,
  deleted_at     DATETIME(3)   NULL,
  deleted_by     BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_tfb_uuid (uuid),
  KEY ix_tfb_talent (talent_id, deleted_at),
  CONSTRAINT fk_tfb_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_tfb_talent FOREIGN KEY (talent_id) REFERENCES talent(id),
  CONSTRAINT fk_tfb_author FOREIGN KEY (author_user_id) REFERENCES app_user(id),
  CONSTRAINT ck_tfb_stars  CHECK (stars BETWEEN 1 AND 5)
) ENGINE=InnoDB;

-- Colecciones de favoritos (por usuario)
CREATE TABLE favorite_collection (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid        CHAR(36)     NOT NULL,
  tenant_id   BIGINT UNSIGNED NOT NULL,
  user_id     BIGINT UNSIGNED NOT NULL,
  name        VARCHAR(100) NOT NULL,
  active_key  TINYINT AS (IF(deleted_at IS NULL, 1, NULL)) STORED,
  created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by  BIGINT UNSIGNED NULL,
  updated_at  DATETIME(3)  NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by  BIGINT UNSIGNED NULL,
  deleted_at  DATETIME(3)  NULL,
  deleted_by  BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_fc_uuid (uuid),
  UNIQUE KEY uq_fc_name (user_id, name, active_key),
  CONSTRAINT fk_fc_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_fc_user   FOREIGN KEY (user_id) REFERENCES app_user(id)
) ENGINE=InnoDB;

CREATE TABLE favorite_collection_talent (
  collection_id BIGINT UNSIGNED NOT NULL,
  talent_id     BIGINT UNSIGNED NOT NULL,
  created_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by    BIGINT UNSIGNED NULL,
  PRIMARY KEY (collection_id, talent_id),
  CONSTRAINT fk_fct_coll   FOREIGN KEY (collection_id) REFERENCES favorite_collection(id),
  CONSTRAINT fk_fct_talent FOREIGN KEY (talent_id) REFERENCES talent(id)
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- Catálogos que usa el módulo
-- ---------------------------------------------------------------------
INSERT INTO master_table (code, name_i18n, scope, is_system) VALUES
 ('DOC_IDENTITY_TYPE', JSON_OBJECT('es','Tipos de documento de identidad','en','ID document types'), 'GLOBAL', 1),
 ('BILLING_MODALITY',  JSON_OBJECT('es','Modalidades de contratación','en','Hiring modalities'), 'BOTH', 0);

INSERT INTO master_item (uuid, master_table_id, code, label_i18n, sort_order)
SELECT UUID(), t.id, v.code, v.label, v.ord
FROM master_table t
JOIN (
  SELECT 'DOC_IDENTITY_TYPE' tbl, 'DNI' code, JSON_OBJECT('es','DNI','en','National ID') label, 1 ord UNION ALL
  SELECT 'DOC_IDENTITY_TYPE', 'CE', JSON_OBJECT('es','Carné de extranjería','en','Foreigner ID'), 2 UNION ALL
  SELECT 'DOC_IDENTITY_TYPE', 'PASSPORT', JSON_OBJECT('es','Pasaporte','en','Passport'), 3 UNION ALL
  SELECT 'BILLING_MODALITY', 'PAYROLL', JSON_OBJECT('es','Planilla','en','Payroll'), 1 UNION ALL
  SELECT 'BILLING_MODALITY', 'RXH', JSON_OBJECT('es','Recibo por honorarios','en','Freelance invoice'), 2 UNION ALL
  SELECT 'COUNTRY', 'CO', JSON_OBJECT('es','Colombia','en','Colombia'), 2 UNION ALL
  SELECT 'COUNTRY', 'CL', JSON_OBJECT('es','Chile','en','Chile'), 3 UNION ALL
  SELECT 'COUNTRY', 'MX', JSON_OBJECT('es','México','en','Mexico'), 4 UNION ALL
  SELECT 'COUNTRY', 'AR', JSON_OBJECT('es','Argentina','en','Argentina'), 5 UNION ALL
  SELECT 'COUNTRY', 'EC', JSON_OBJECT('es','Ecuador','en','Ecuador'), 6 UNION ALL
  SELECT 'COUNTRY', 'VE', JSON_OBJECT('es','Venezuela','en','Venezuela'), 7 UNION ALL
  SELECT 'COUNTRY', 'ES', JSON_OBJECT('es','España','en','Spain'), 8 UNION ALL
  SELECT 'COUNTRY', 'US', JSON_OBJECT('es','Estados Unidos','en','United States'), 9 UNION ALL
  SELECT 'TECH_SKILL', 'JAVA', JSON_OBJECT('es','Java','en','Java'), 1 UNION ALL
  SELECT 'TECH_SKILL', 'SPRING', JSON_OBJECT('es','Spring Boot','en','Spring Boot'), 2 UNION ALL
  SELECT 'TECH_SKILL', 'GO', JSON_OBJECT('es','Go','en','Go'), 3 UNION ALL
  SELECT 'TECH_SKILL', 'PYTHON', JSON_OBJECT('es','Python','en','Python'), 4 UNION ALL
  SELECT 'TECH_SKILL', 'JAVASCRIPT', JSON_OBJECT('es','JavaScript','en','JavaScript'), 5 UNION ALL
  SELECT 'TECH_SKILL', 'TYPESCRIPT', JSON_OBJECT('es','TypeScript','en','TypeScript'), 6 UNION ALL
  SELECT 'TECH_SKILL', 'REACT', JSON_OBJECT('es','React','en','React'), 7 UNION ALL
  SELECT 'TECH_SKILL', 'ANGULAR', JSON_OBJECT('es','Angular','en','Angular'), 8 UNION ALL
  SELECT 'TECH_SKILL', 'NODE', JSON_OBJECT('es','Node.js','en','Node.js'), 9 UNION ALL
  SELECT 'TECH_SKILL', 'DOTNET', JSON_OBJECT('es','.NET','en','.NET'), 10 UNION ALL
  SELECT 'TECH_SKILL', 'SQL', JSON_OBJECT('es','SQL','en','SQL'), 11 UNION ALL
  SELECT 'TECH_SKILL', 'SQLSERVER', JSON_OBJECT('es','SQL Server','en','SQL Server'), 12 UNION ALL
  SELECT 'TECH_SKILL', 'MYSQL', JSON_OBJECT('es','MySQL','en','MySQL'), 13 UNION ALL
  SELECT 'TECH_SKILL', 'AWS', JSON_OBJECT('es','AWS','en','AWS'), 14 UNION ALL
  SELECT 'TECH_SKILL', 'AZURE', JSON_OBJECT('es','Azure','en','Azure'), 15 UNION ALL
  SELECT 'TECH_SKILL', 'DOCKER', JSON_OBJECT('es','Docker','en','Docker'), 16 UNION ALL
  SELECT 'TECH_SKILL', 'KUBERNETES', JSON_OBJECT('es','Kubernetes','en','Kubernetes'), 17 UNION ALL
  SELECT 'TECH_SKILL', 'SCRUM', JSON_OBJECT('es','Scrum','en','Scrum'), 18 UNION ALL
  SELECT 'TECH_SKILL', 'QA', JSON_OBJECT('es','QA / Testing','en','QA / Testing'), 19 UNION ALL
  SELECT 'SOFT_SKILL', 'COMMUNICATION', JSON_OBJECT('es','Comunicación','en','Communication'), 1 UNION ALL
  SELECT 'SOFT_SKILL', 'TEAMWORK', JSON_OBJECT('es','Trabajo en equipo','en','Teamwork'), 2 UNION ALL
  SELECT 'SOFT_SKILL', 'LEADERSHIP', JSON_OBJECT('es','Liderazgo','en','Leadership'), 3 UNION ALL
  SELECT 'SOFT_SKILL', 'PROBLEM_SOLVING', JSON_OBJECT('es','Resolución de problemas','en','Problem solving'), 4 UNION ALL
  SELECT 'SOFT_SKILL', 'ADAPTABILITY', JSON_OBJECT('es','Adaptabilidad','en','Adaptability'), 5 UNION ALL
  SELECT 'SOFT_SKILL', 'PROACTIVITY', JSON_OBJECT('es','Proactividad','en','Proactivity'), 6 UNION ALL
  SELECT 'SOFT_SKILL', 'TIME_MANAGEMENT', JSON_OBJECT('es','Gestión del tiempo','en','Time management'), 7
) v ON v.tbl = t.code;
