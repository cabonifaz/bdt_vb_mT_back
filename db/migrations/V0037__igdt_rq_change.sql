-- Cambio de RQ y fin de contrato desde IGDT (v1.17): IGDT puede pedir llevar a un talento ingresado a uno o más RQ
-- (evento RQ_CHANGE_REQUESTED). BDT lo agrega como candidato en cada RQ destino y guarda aquí su origen
-- ({source, operationId, fromRequirementId, fromCode, date, hireId}) para mostrar «Traído desde IGDT». NULL = agregado en BDT.
ALTER TABLE requirement_candidate ADD COLUMN igdt_origin JSON NULL AFTER igdt_manager_id;
