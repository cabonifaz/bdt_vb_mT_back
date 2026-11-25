-- Solicitantes de la ficha de ingreso (v1.14): el snapshot de ingreso que BDT envía a IGDT incluye el correo de quien
-- confirmó (confirmedByEmail) y el analista responsable del RQ (rqAnalystName, rqAnalystEmail; null si no hay), para que
-- IGDT les envíe las fichas. Se re-encolan las solicitudes vigentes de los tenants con la integración activa (candidatos
-- confirmados desde hires_from, sin renuncia, en RQ no eliminados) para que IGDT las reciba con el dato nuevo. Si ya había
-- un envío pendiente, solo se adelanta.
INSERT INTO igdt_outbox (tenant_id, kind, entity_uuid)
SELECT r.tenant_id, 'HIRE', rc.uuid
  FROM tenant_integration ti
  JOIN requirement r ON r.tenant_id = ti.tenant_id AND r.deleted_at IS NULL
  JOIN requirement_candidate rc ON rc.requirement_id = r.id AND rc.deleted_at IS NULL AND rc.resigned_at IS NULL
  JOIN master_item s ON s.id = rc.stage_item_id AND s.code = 'CONFIRMED'
 WHERE ti.provider = 'IGDT' AND ti.status = 'ACTIVE'
   AND IFNULL(rc.stage_at, rc.created_at) >= IFNULL(ti.hires_from, '2000-01-01')
ON DUPLICATE KEY UPDATE next_at = NOW(3), attempts = 0;
