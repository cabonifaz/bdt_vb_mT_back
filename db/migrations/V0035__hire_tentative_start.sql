-- Fecha tentativa de inicio de labores (v1.15): Selección la registra obligatoriamente al confirmar a un candidato.
-- Las condiciones del ingreso (modalidad, duración, contrato, EMO, SCTR, laptop) son del RQ; la fecha es de cada
-- candidato confirmado, por eso va en requirement_candidate. Se puede corregir mientras el ingreso no esté registrado
-- en IGDT (hired_at) y viaja a IGDT en el snapshot de ingreso (conditions.startDate, AAAA-MM-DD). Los confirmados
-- vigentes anteriores quedan sin fecha (NULL): no se inventa.
ALTER TABLE requirement_candidate ADD COLUMN tentative_start_date DATE NULL AFTER stage_at;
