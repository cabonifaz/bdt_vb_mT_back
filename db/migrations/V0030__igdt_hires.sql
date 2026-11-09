-- Ingresos registrados en IGDT: BDT lee los eventos de ingreso (hires_cursor) y marca al candidato como ingresado
-- (hired_at, hire_start_date). Cuando un RQ abierto tiene tantos ingresos como vacantes, se cierra como Atendido.
ALTER TABLE tenant_integration ADD COLUMN hires_cursor BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER sync_cursor;
ALTER TABLE requirement_candidate ADD COLUMN hired_at DATETIME(3) NULL AFTER stage_at,
  ADD COLUMN hire_start_date DATE NULL AFTER hired_at;
