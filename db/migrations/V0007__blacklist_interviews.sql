-- =====================================================================
-- Clientes (catálogo del tenant), lista negra y entrevistas
-- Clientes: master_item de la tabla CLIENT (str1 = RUC, str2 = dirección, str3 = URL de Google Maps).
-- =====================================================================

INSERT INTO permission (code, description) VALUES
 ('blacklist.manage', 'Agregar, editar y quitar restricciones de lista negra'),
 ('interview.read',   'Consultar entrevistas'),
 ('interview.write',  'Registrar y editar entrevistas');

INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p
  ON p.code IN ('blacklist.manage','interview.read','interview.write')
 WHERE r.code IN ('TENANT_ADMIN','RECRUITER','COMMERCIAL');
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code = 'interview.read' WHERE r.code = 'VIEWER';

-- ---------------------------------------------------------------------
-- Catálogos
-- ---------------------------------------------------------------------
INSERT INTO master_table (code, name_i18n, scope, is_system) VALUES
 ('CLIENT',              JSON_OBJECT('es','Clientes','en','Clients'), 'TENANT', 0),
 ('INTERVIEW_STATUS',    JSON_OBJECT('es','Estados de entrevista','en','Interview statuses'), 'GLOBAL', 1),
 ('INTERVIEW_STAGE',     JSON_OBJECT('es','Etapas de entrevista','en','Interview stages'), 'BOTH', 0),
 ('INTERVIEW_TYPE',      JSON_OBJECT('es','Tipos de entrevista','en','Interview types'), 'GLOBAL', 1),
 ('INTERVIEW_DURATION',  JSON_OBJECT('es','Duración de entrevista','en','Interview durations'), 'BOTH', 0),
 ('INTERVIEW_FILE_TYPE', JSON_OBJECT('es','Tipos de archivo de entrevista','en','Interview file types'), 'BOTH', 0),
 ('PHONE_QUESTION',      JSON_OBJECT('es','Preguntas de entrevista telefónica','en','Phone screening questions'), 'BOTH', 0);

INSERT INTO master_item (uuid, master_table_id, code, label_i18n, num1, sort_order)
SELECT UUID(), t.id, v.code, v.label, v.n1, v.ord
FROM master_table t
JOIN (
  SELECT 'INTERVIEW_STATUS' tbl, 'REGISTERED' code, JSON_OBJECT('es','Registrado','en','Scheduled') label, NULL n1, 1 ord UNION ALL
  SELECT 'INTERVIEW_STATUS', 'IN_PROGRESS', JSON_OBJECT('es','En proceso','en','In progress'), NULL, 2 UNION ALL
  SELECT 'INTERVIEW_STATUS', 'FINISHED', JSON_OBJECT('es','Finalizado','en','Finished'), NULL, 3 UNION ALL
  SELECT 'INTERVIEW_STATUS', 'CANCELLED', JSON_OBJECT('es','Cancelado','en','Cancelled'), NULL, 4 UNION ALL
  SELECT 'INTERVIEW_STAGE', 'RS_TEAM', JSON_OBJECT('es','Entrevista con el equipo de R&S','en','Interview with the R&S team'), NULL, 1 UNION ALL
  SELECT 'INTERVIEW_STAGE', 'PHONE_SCREEN', JSON_OBJECT('es','Entrevista filtro telefónico R&S','en','R&S phone screening'), NULL, 2 UNION ALL
  SELECT 'INTERVIEW_STAGE', 'TECH_LEAD', JSON_OBJECT('es','Entrevista técnica con líder técnico','en','Technical interview with tech lead'), NULL, 3 UNION ALL
  SELECT 'INTERVIEW_STAGE', 'TECH_CLIENT', JSON_OBJECT('es','Entrevista técnica con cliente','en','Technical interview with client'), NULL, 4 UNION ALL
  SELECT 'INTERVIEW_TYPE', 'VIRTUAL', JSON_OBJECT('es','Virtual','en','Virtual'), NULL, 1 UNION ALL
  SELECT 'INTERVIEW_TYPE', 'IN_PERSON', JSON_OBJECT('es','Presencial','en','In person'), NULL, 2 UNION ALL
  SELECT 'INTERVIEW_TYPE', 'PHONE', JSON_OBJECT('es','Telefónica','en','Phone'), NULL, 3 UNION ALL
  SELECT 'INTERVIEW_DURATION', 'MIN30', JSON_OBJECT('es','30 minutos','en','30 minutes'), 30, 1 UNION ALL
  SELECT 'INTERVIEW_DURATION', 'MIN45', JSON_OBJECT('es','45 minutos','en','45 minutes'), 45, 2 UNION ALL
  SELECT 'INTERVIEW_DURATION', 'MIN60', JSON_OBJECT('es','1 hora','en','1 hour'), 60, 3 UNION ALL
  SELECT 'INTERVIEW_DURATION', 'MIN90', JSON_OBJECT('es','1 hora 30 minutos','en','1 hour 30 minutes'), 90, 4 UNION ALL
  SELECT 'INTERVIEW_FILE_TYPE', 'TECH_TEST', JSON_OBJECT('es','Prueba técnica','en','Technical test'), NULL, 1 UNION ALL
  SELECT 'INTERVIEW_FILE_TYPE', 'EVALUATION', JSON_OBJECT('es','Evaluación','en','Evaluation'), NULL, 2 UNION ALL
  SELECT 'INTERVIEW_FILE_TYPE', 'CV', JSON_OBJECT('es','CV','en','CV'), NULL, 3 UNION ALL
  SELECT 'INTERVIEW_FILE_TYPE', 'OTHER', JSON_OBJECT('es','Otro','en','Other'), NULL, 4 UNION ALL
  -- Preguntas base de la entrevista telefónica (editables por el tenant desde los maestros)
  SELECT 'PHONE_QUESTION', 'Q_AVAILABILITY', JSON_OBJECT('es','¿Cuál es tu disponibilidad para iniciar?','en','When could you start?'), NULL, 1 UNION ALL
  SELECT 'PHONE_QUESTION', 'Q_CURRENT_JOB', JSON_OBJECT('es','¿Trabajas actualmente? ¿Por qué buscas un cambio?','en','Are you currently employed? Why are you looking for a change?'), NULL, 2 UNION ALL
  SELECT 'PHONE_QUESTION', 'Q_SALARY', JSON_OBJECT('es','¿Cuál es tu expectativa salarial y modalidad de contratación?','en','What are your salary expectations and preferred contract type?'), NULL, 3 UNION ALL
  SELECT 'PHONE_QUESTION', 'Q_WORK_MODE', JSON_OBJECT('es','¿Qué modalidad prefieres (remoto, híbrido, presencial)?','en','Which work mode do you prefer (remote, hybrid, on-site)?'), NULL, 4 UNION ALL
  SELECT 'PHONE_QUESTION', 'Q_EQUIPMENT', JSON_OBJECT('es','¿Cuentas con equipo propio?','en','Do you have your own equipment?'), NULL, 5 UNION ALL
  SELECT 'PHONE_QUESTION', 'Q_ENGLISH', JSON_OBJECT('es','¿Cuál es tu nivel de inglés?','en','What is your English level?'), NULL, 6 UNION ALL
  SELECT 'PHONE_QUESTION', 'Q_EXPERIENCE', JSON_OBJECT('es','Resume tu experiencia más relevante para el puesto','en','Summarize your most relevant experience for the role'), NULL, 7
) v ON v.tbl = t.code;

-- ---------------------------------------------------------------------
-- Lista negra: client_item_id NULL = restricción global (todos los clientes)
-- ---------------------------------------------------------------------
CREATE TABLE talent_blacklist (
  id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid           CHAR(36)      NOT NULL,
  tenant_id      BIGINT UNSIGNED NOT NULL,
  talent_id      BIGINT UNSIGNED NOT NULL,
  client_item_id BIGINT UNSIGNED NULL,
  reason         VARCHAR(1000) NOT NULL,
  client_key     BIGINT UNSIGNED AS (IFNULL(client_item_id, 0)) STORED,
  active_key     TINYINT AS (IF(deleted_at IS NULL, 1, NULL)) STORED,
  created_at     DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by     BIGINT UNSIGNED NULL,
  updated_at     DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by     BIGINT UNSIGNED NULL,
  deleted_at     DATETIME(3)   NULL,
  deleted_by     BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_bl_uuid (uuid),
  UNIQUE KEY uq_bl_active (talent_id, client_key, active_key),
  KEY ix_bl_tenant (tenant_id, deleted_at),
  CONSTRAINT fk_bl_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_bl_talent FOREIGN KEY (talent_id) REFERENCES talent(id),
  CONSTRAINT fk_bl_client FOREIGN KEY (client_item_id) REFERENCES master_item(id)
) ENGINE=InnoDB;

CREATE TABLE talent_blacklist_history (
  id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  tenant_id      BIGINT UNSIGNED NOT NULL,
  blacklist_id   BIGINT UNSIGNED NOT NULL,
  talent_id      BIGINT UNSIGNED NOT NULL,
  client_item_id BIGINT UNSIGNED NULL,
  movement       VARCHAR(10)   NOT NULL,   -- CREATE | UPDATE | DELETE
  reason         VARCHAR(1000) NOT NULL,
  user_id        BIGINT UNSIGNED NULL,
  created_at     DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY ix_blh_talent (talent_id, id),
  CONSTRAINT fk_blh_bl FOREIGN KEY (blacklist_id) REFERENCES talent_blacklist(id),
  CONSTRAINT ck_blh_mov CHECK (movement IN ('CREATE','UPDATE','DELETE'))
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- Entrevistas
-- ---------------------------------------------------------------------
CREATE TABLE interview (
  id                BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid              CHAR(36)     NOT NULL,
  tenant_id         BIGINT UNSIGNED NOT NULL,
  talent_id         BIGINT UNSIGNED NOT NULL,
  type_item_id      BIGINT UNSIGNED NOT NULL,
  stage_item_id     BIGINT UNSIGNED NOT NULL,
  status_item_id    BIGINT UNSIGNED NOT NULL,
  duration_item_id  BIGINT UNSIGNED NULL,
  client_item_id    BIGINT UNSIGNED NULL,
  requirement_ref   VARCHAR(100) NULL,         -- código del RQ (hasta integrar AutFMI)
  profile           VARCHAR(200) NOT NULL,     -- perfil / puesto
  scheduled_date    DATE         NOT NULL,
  scheduled_time    TIME         NOT NULL,
  meeting_link      VARCHAR(500) NULL,
  location_url      VARCHAR(500) NULL,
  address           VARCHAR(500) NULL,
  rating_general    TINYINT      NOT NULL DEFAULT 0,
  rating_personal   TINYINT      NOT NULL DEFAULT 0,
  rating_experience TINYINT      NOT NULL DEFAULT 0,
  rating_languages  TINYINT      NOT NULL DEFAULT 0,
  rating_education  TINYINT      NOT NULL DEFAULT 0,
  notes_personal    TEXT         NULL,
  notes_experience  TEXT         NULL,
  notes_languages   TEXT         NULL,
  notes_education   TEXT         NULL,
  cancel_reason     TEXT         NULL,
  version           INT          NOT NULL DEFAULT 1,
  created_at        DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by        BIGINT UNSIGNED NULL,
  updated_at        DATETIME(3)  NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by        BIGINT UNSIGNED NULL,
  deleted_at        DATETIME(3)  NULL,
  deleted_by        BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_iv_uuid (uuid),
  KEY ix_iv_tenant (tenant_id, scheduled_date, deleted_at),
  KEY ix_iv_talent (talent_id, type_item_id),
  CONSTRAINT fk_iv_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_iv_talent FOREIGN KEY (talent_id) REFERENCES talent(id),
  CONSTRAINT fk_iv_type FOREIGN KEY (type_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_iv_stage FOREIGN KEY (stage_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_iv_status FOREIGN KEY (status_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_iv_duration FOREIGN KEY (duration_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_iv_client FOREIGN KEY (client_item_id) REFERENCES master_item(id),
  CONSTRAINT ck_iv_ratings CHECK (rating_general BETWEEN 0 AND 5 AND rating_personal BETWEEN 0 AND 5 AND
    rating_experience BETWEEN 0 AND 5 AND rating_languages BETWEEN 0 AND 5 AND rating_education BETWEEN 0 AND 5)
) ENGINE=InnoDB;

-- Entrevistadores: personas (internas o del cliente), no necesariamente usuarios del sistema
CREATE TABLE interview_interviewer (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  interview_id BIGINT UNSIGNED NOT NULL,
  full_name    VARCHAR(150)  NOT NULL,
  email_enc    VARBINARY(512) NULL,
  notify       TINYINT(1)    NOT NULL DEFAULT 0,
  sort_order   INT           NOT NULL DEFAULT 0,
  KEY ix_ivi (interview_id),
  CONSTRAINT fk_ivi_iv FOREIGN KEY (interview_id) REFERENCES interview(id)
) ENGINE=InnoDB;

CREATE TABLE interview_recording (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  interview_id BIGINT UNSIGNED NOT NULL,
  url          VARCHAR(500)  NOT NULL,
  recorded_on  DATE          NOT NULL,
  KEY ix_ivr (interview_id),
  CONSTRAINT fk_ivr_iv FOREIGN KEY (interview_id) REFERENCES interview(id)
) ENGINE=InnoDB;

-- Respuestas a las preguntas de la entrevista telefónica (una por pregunta)
CREATE TABLE interview_answer (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  interview_id     BIGINT UNSIGNED NOT NULL,
  question_item_id BIGINT UNSIGNED NOT NULL,
  answer           TEXT          NOT NULL,
  created_at       DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by       BIGINT UNSIGNED NULL,
  updated_at       DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by       BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_iva (interview_id, question_item_id),
  CONSTRAINT fk_iva_iv FOREIGN KEY (interview_id) REFERENCES interview(id),
  CONSTRAINT fk_iva_q FOREIGN KEY (question_item_id) REFERENCES master_item(id)
) ENGINE=InnoDB;
