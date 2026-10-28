-- =====================================================================
-- Rol Jefe de selección y reapertura de RQ
-- - Jefe de selección: lo del reclutador más crear, editar y cerrar RQ (rq.write), ver tarifas y bandas
--   salariales (de talentos y de RQ) y reabrir RQ cerrados (rq.reopen). No administra el tenant.
-- - Reabrir un RQ cerrado (renunció un talento o se amplían las vacantes) solo con rq.reopen y con motivo;
--   queda en el historial requirement_reopen.
-- - El estado "En producción" pasa a llamarse "A procesar" (postulantes completos, listos para su ingreso).
-- =====================================================================

INSERT INTO role (code, name_i18n, is_platform) VALUES
 ('SELECTION_LEAD', JSON_OBJECT('es','Jefe de selección','en','Selection lead'), 0);

INSERT INTO permission (code, description) VALUES
 ('rq.reopen', 'Reabrir requerimientos cerrados (renuncia de un talento o más vacantes)');

INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code IN (
  'talent.read','talent.write','talent.rates.read','talent.cv.generate',
  'blacklist.manage','interview.read','interview.write',
  'rq.read','rq.write','rq.candidates','rq.rates.read','rq.reopen')
 WHERE r.code = 'SELECTION_LEAD';

CREATE TABLE requirement_reopen (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  requirement_id  BIGINT UNSIGNED NOT NULL,
  reason          VARCHAR(20)     NOT NULL,
  notes           VARCHAR(500)    NULL,
  from_status_id  BIGINT UNSIGNED NOT NULL,
  to_status_id    BIGINT UNSIGNED NOT NULL,
  created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by      BIGINT UNSIGNED NOT NULL,
  KEY ix_reopen_rq (requirement_id, created_at),
  CONSTRAINT ck_reopen_reason CHECK (reason IN ('RESIGNATION','MORE_VACANCIES')),
  CONSTRAINT fk_reopen_rq FOREIGN KEY (requirement_id) REFERENCES requirement(id),
  CONSTRAINT fk_reopen_from FOREIGN KEY (from_status_id) REFERENCES master_item(id),
  CONSTRAINT fk_reopen_to FOREIGN KEY (to_status_id) REFERENCES master_item(id),
  CONSTRAINT fk_reopen_user FOREIGN KEY (created_by) REFERENCES app_user(id)
) ENGINE=InnoDB;

UPDATE master_item i JOIN master_table t ON t.id = i.master_table_id
   SET i.label_i18n = JSON_OBJECT('es','A procesar','en','To process')
 WHERE t.code = 'RQ_STATUS' AND i.code = 'IN_PRODUCTION' AND i.tenant_id IS NULL;
