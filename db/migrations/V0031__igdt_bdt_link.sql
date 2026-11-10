-- Llave BDT automática para IGDT: al conectar la empresa con IGDT (y en cada sincronización mientras falte), BDT crea
-- una API key propia del tenant («IGDT (automática)», alcance talent.read, ya habilitada) y se la entrega a IGDT
-- (PUT /integration/v1/bdt-link). Con ella IGDT lee la ficha, los CV y las postulaciones de los candidatos.
-- - tenant_integration.link_key_id: la key entregada. NULL = aún no vinculada (se reintenta); al desconectar se revoca.
-- Eventos de ingreso que llegan de IGDT además de HIRED: START_CHANGED (cambió la fecha de inicio) y REVOKED (el cliente
-- revocó el ingreso antes de empezar). Se registran en requirement_candidate_event como HIRE_START_CHANGED y
-- HIRE_REVOKED (type VARCHAR(20) sin CHECK: no requiere cambio de esquema).
ALTER TABLE tenant_integration ADD COLUMN link_key_id BIGINT UNSIGNED NULL AFTER hires_cursor,
  ADD CONSTRAINT fk_tenant_integration_link_key FOREIGN KEY (link_key_id) REFERENCES api_key(id);
