-- =====================================================================
-- Renuncia de un talento ya confirmado en un RQ
-- Al reabrir un RQ por renuncia se indica qué candidatos renunciaron: quedan en el pipeline (historial) marcados
-- como renunciados, dejan de cubrir la vacante y su baja queda PENDING hasta que el FMI confirme que es
-- efectiva (EFFECTIVE) mediante una API de BDT (pendiente de la integración).
-- =====================================================================

ALTER TABLE requirement_candidate
  ADD COLUMN resigned_at      DATETIME(3)     NULL AFTER notes,
  ADD COLUMN resign_reopen_id BIGINT UNSIGNED NULL AFTER resigned_at,
  ADD COLUMN offboard_status  VARCHAR(10)     NULL AFTER resign_reopen_id,
  ADD COLUMN offboard_at      DATETIME(3)     NULL AFTER offboard_status,
  ADD CONSTRAINT ck_candidate_offboard CHECK (offboard_status IS NULL OR offboard_status IN ('PENDING','EFFECTIVE')),
  ADD CONSTRAINT fk_candidate_resign_reopen FOREIGN KEY (resign_reopen_id) REFERENCES requirement_reopen(id);
