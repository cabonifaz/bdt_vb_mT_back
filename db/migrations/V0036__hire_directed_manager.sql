-- Gestor dirigido (v1.16): al confirmar a un candidato, Selección puede dirigir la solicitud de ingreso a un gestor de
-- IGDT concreto (uuid del gestor en IGDT; NULL = los gestores del cliente, como hasta ahora). Va junto a la fecha
-- tentativa de inicio, es editable mientras IGDT no registre el ingreso y viaja en el snapshot de ingreso como
-- igdtManagerId. Los confirmados existentes quedan en NULL (gestores del cliente).
ALTER TABLE requirement_candidate ADD COLUMN igdt_manager_id CHAR(36) NULL AFTER tentative_start_date;
