-- Solicitudes de ingreso en IGDT: solo se envían los candidatos confirmados desde hires_from (30 días antes de la
-- primera conexión). Las confirmaciones anteriores son historia: ya se gestionaron fuera de IGDT.
ALTER TABLE tenant_integration ADD COLUMN hires_from DATETIME(3) NULL AFTER rq_synced_at;
UPDATE tenant_integration SET hires_from = NOW(3) - INTERVAL 30 DAY WHERE provider = 'IGDT';
