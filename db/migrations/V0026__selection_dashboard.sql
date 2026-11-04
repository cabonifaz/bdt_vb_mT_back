-- Indicadores de selección y días hábiles en las alertas de RQ sin movimiento.
-- - requirement.closed_at: cuándo se cerró el RQ (para el tiempo hasta el cierre y si se cerró a tiempo). Se completa
--   desde la auditoría (último cambio a un estado de cierre); al reabrir vuelve a NULL.
-- - v_rq_activity: además del último movimiento (UTC), su día en la hora local del tenant (last_move_day), con el que
--   se cuentan los días hábiles sin movimiento.
-- - Permiso dashboard.selection (admin y jefe de selección) y la funcionalidad selection_dashboard.

ALTER TABLE requirement ADD COLUMN closed_at DATETIME(3) NULL AFTER due_date;

UPDATE requirement r
  JOIN master_item s ON s.id = r.status_item_id AND s.str1 = 'CLOSED'
   SET r.closed_at = IFNULL((SELECT MAX(a.created_at) FROM audit_log a
                              WHERE a.entity = 'requirement' AND a.entity_id = r.uuid AND a.action IN ('STATUS', 'UPDATE', 'CREATE')
                                AND JSON_UNQUOTE(JSON_EXTRACT(a.detail, '$.to')) IN (SELECT code FROM master_item WHERE str1 = 'CLOSED')),
                            IFNULL(r.updated_at, r.created_at)),
       r.updated_at = r.updated_at
 WHERE r.deleted_at IS NULL;

CREATE OR REPLACE SQL SECURITY INVOKER VIEW v_rq_activity AS
SELECT x.requirement_id, x.last_move_at, x.in_process,
       DATE(IFNULL(CONVERT_TZ(x.last_move_at, '+00:00', x.tz), x.last_move_at)) AS last_move_day
  FROM (SELECT r.id AS requirement_id, t.timezone AS tz,
               GREATEST(IFNULL((SELECT MAX(rc.stage_at) FROM requirement_candidate rc WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL), '1970-01-01'),
                        IFNULL((SELECT MAX(ro.created_at) FROM requirement_reopen ro WHERE ro.requirement_id = r.id), '1970-01-01')) AS last_move_at,
               (SELECT COUNT(*) FROM requirement_candidate rc JOIN master_item cs ON cs.id = rc.stage_item_id
                 WHERE rc.requirement_id = r.id AND rc.deleted_at IS NULL AND rc.resigned_at IS NULL AND cs.code <> 'CONFIRMED') AS in_process
          FROM requirement r JOIN tenant t ON t.id = r.tenant_id
         WHERE r.deleted_at IS NULL) x;

INSERT INTO permission (code, description) VALUES ('dashboard.selection', 'Ver los indicadores de selección');
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code = 'dashboard.selection' WHERE r.code IN ('TENANT_ADMIN', 'SELECTION_LEAD');

INSERT INTO feature (code, name_i18n, description_i18n, module, impacted_roles, is_core, default_enabled, sort_order) VALUES
 ('selection_dashboard', JSON_OBJECT('es','Indicadores de selección','en','Selection dashboard'),
   JSON_OBJECT('es','Dashboard para el jefe de selección y el administrador: vencidos, tiempos y postulantes por ingreso, altas y entrevistas por analista, embudo, clientes y perfiles.',
               'en','Dashboard for the selection lead and the administrator: overdue requirements, time and candidates per hire, hires and interviews per analyst, funnel, clients and profiles.'),
   'requirements', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD'), 0, 1, 17);

-- entrevistas ya registradas: vínculo con su RQ por el código
UPDATE interview iv JOIN requirement r ON r.tenant_id = iv.tenant_id AND r.code = iv.requirement_ref AND r.deleted_at IS NULL
   SET iv.requirement_id = r.id, iv.updated_at = iv.updated_at
 WHERE iv.requirement_id IS NULL;
