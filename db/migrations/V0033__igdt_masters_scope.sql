-- Alcance masters.read para la llave BDT automática de IGDT (v1.11): IGDT replica la pantalla de talentos de BDT y
-- necesita los catálogos de sus filtros (TECH_SKILL, LANGUAGE_LEVEL, ACADEMIC_GRADE) por GET /integration/v1/masters.
-- La llave se crea desde ahora con talent.read, talent.search y masters.read (sp_igdt_link_key_create); aquí se agrega
-- masters.read a las llaves automáticas vigentes (tenant_integration.link_key_id), sin duplicarlo. La API no guarda las
-- keys en caché: el cambio rige en la siguiente petición.
UPDATE api_key k
  JOIN tenant_integration ti ON ti.link_key_id = k.id AND ti.provider = 'IGDT'
   SET k.scopes = JSON_ARRAY_APPEND(k.scopes, '$', 'masters.read')
 WHERE k.deleted_at IS NULL AND k.status <> 'REVOKED'
   AND NOT JSON_CONTAINS(k.scopes, JSON_QUOTE('masters.read'));
