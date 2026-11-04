-- Alertas de RQ sin movimiento: cuando pasan muchos días sin que ningún postulante del RQ cambie de etapa.
-- - requirement_candidate.stage_at: cuándo entró el postulante a su etapa actual (alta o último cambio de etapa).
-- - requirement.stale_level: último nivel avisado (0 nada, 1 atención, 2 crítico); evita repetir la notificación
--   y vuelve a 0 cuando hay movimiento.
-- - rq_alert_setting: días de atención y críticos por tenant y si se notifica (por defecto 5 y 10 días, con aviso).
-- - Permiso rq.alerts.manage (admin y jefe de selección) y la funcionalidad rq_stale_alerts en el catálogo.

ALTER TABLE requirement_candidate ADD COLUMN stage_at DATETIME(3) NULL AFTER stage_item_id;
UPDATE requirement_candidate rc
   SET rc.stage_at = IFNULL((SELECT MAX(e.created_at) FROM requirement_candidate_event e
                              WHERE e.candidate_id = rc.id AND e.type IN ('ADDED', 'STAGE')), rc.created_at);
ALTER TABLE requirement_candidate MODIFY stage_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3);

ALTER TABLE requirement ADD COLUMN stale_level TINYINT NOT NULL DEFAULT 0;

CREATE TABLE rq_alert_setting (
  tenant_id     BIGINT UNSIGNED PRIMARY KEY,
  warn_days     SMALLINT        NOT NULL DEFAULT 5,
  critical_days SMALLINT        NOT NULL DEFAULT 10,
  notify        TINYINT(1)      NOT NULL DEFAULT 1,
  updated_at    DATETIME(3)     NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by    BIGINT UNSIGNED NULL,
  CONSTRAINT fk_rq_alert_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT ck_rq_alert_days CHECK (warn_days BETWEEN 1 AND 90 AND critical_days BETWEEN 2 AND 180 AND critical_days > warn_days)
) ENGINE=InnoDB;

INSERT INTO permission (code, description) VALUES ('rq.alerts.manage', 'Configurar las alertas de RQ sin movimiento');
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code = 'rq.alerts.manage' WHERE r.code IN ('TENANT_ADMIN', 'SELECTION_LEAD');

-- sa_notified_at NULL: al desplegar se avisa al Super Admin que hay una funcionalidad nueva
INSERT INTO feature (code, name_i18n, description_i18n, module, impacted_roles, is_core, default_enabled, sort_order) VALUES
 ('rq_stale_alerts', JSON_OBJECT('es','Alertas de RQ sin movimiento','en','Stalled requirement alerts'),
   JSON_OBJECT('es','Avisa en el tablero y por notificación cuando pasan varios días sin que ningún postulante del RQ cambie de etapa.',
               'en','Warns on the board and by notification when several days pass without any requirement candidate changing stage.'),
   'requirements', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER'), 0, 1, 16);

-- Actividad del RQ: último movimiento (cambio de etapa o alta de un postulante, o reapertura) y cuántos postulantes
-- siguen en proceso (sin confirmar ni renunciar). Vista y no función: con binlog no se permite CREATE FUNCTION.
CREATE OR REPLACE SQL SECURITY INVOKER VIEW v_rq_activity AS
SELECT r.id AS requirement_id,
       GREATEST(IFNULL((SELECT MAX(rc.stage_at) FROM requirement_candidate rc WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL), '1970-01-01'),
                IFNULL((SELECT MAX(ro.created_at) FROM requirement_reopen ro WHERE ro.requirement_id = r.id), '1970-01-01')) AS last_move_at,
       (SELECT COUNT(*) FROM requirement_candidate rc JOIN master_item cs ON cs.id = rc.stage_item_id
         WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL AND rc.resigned_at IS NULL AND cs.code <> 'CONFIRMED') AS in_process
  FROM requirement r
 WHERE r.deleted_at IS NULL;
