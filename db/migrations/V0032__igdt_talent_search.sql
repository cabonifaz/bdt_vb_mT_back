-- Alcance talent.search (v1.9): búsqueda en todo el banco de talentos del tenant y ficha, CV y documentos de cualquier
-- talento (no solo candidatos) por la API de integración. La llave BDT automática de IGDT se crea desde ahora con
-- talent.read y talent.search (sp_igdt_link_key_create); aquí se agrega talent.search a las llaves automáticas vigentes
-- (tenant_integration.link_key_id), sin duplicarlo. La API no guarda las keys en caché: el cambio rige en la siguiente
-- petición.
UPDATE api_key k
  JOIN tenant_integration ti ON ti.link_key_id = k.id AND ti.provider = 'IGDT'
   SET k.scopes = JSON_ARRAY_APPEND(k.scopes, '$', 'talent.search')
 WHERE k.deleted_at IS NULL AND k.status <> 'REVOKED'
   AND NOT JSON_CONTAINS(k.scopes, JSON_QUOTE('talent.search'));
