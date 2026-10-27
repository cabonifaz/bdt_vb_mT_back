-- =====================================================================
-- Disponibilidad del talento como catálogo (antes texto libre)
-- talent.availability (VARCHAR) -> talent.availability_item_id (master_item de TALENT_AVAILABILITY)
-- =====================================================================

INSERT INTO master_table (code, name_i18n, scope, is_system) VALUES
 ('TALENT_AVAILABILITY', JSON_OBJECT('es','Disponibilidad del talento','en','Talent availability'), 'BOTH', 0);

INSERT INTO master_item (uuid, master_table_id, code, label_i18n, sort_order)
SELECT UUID(), t.id, v.code, v.label, v.ord
FROM master_table t
JOIN (
  SELECT 'IMMEDIATE' code, JSON_OBJECT('es','Inmediata','en','Immediate') label, 1 ord UNION ALL
  SELECT 'WEEKS_2', JSON_OBJECT('es','2 semanas','en','2 weeks'), 2 UNION ALL
  SELECT 'MONTH_1', JSON_OBJECT('es','1 mes','en','1 month'), 3 UNION ALL
  SELECT 'TO_AGREE', JSON_OBJECT('es','A convenir','en','To be agreed'), 4 UNION ALL
  SELECT 'UNAVAILABLE', JSON_OBJECT('es','No disponible','en','Not available'), 5
) v
WHERE t.code = 'TALENT_AVAILABILITY';

ALTER TABLE talent
  ADD COLUMN availability_item_id BIGINT UNSIGNED NULL AFTER summary,
  ADD CONSTRAINT fk_talent_availability FOREIGN KEY (availability_item_id) REFERENCES master_item (id);

-- los textos existentes se asocian a la opción equivalente; el resto queda sin disponibilidad
UPDATE talent t
  JOIN master_item i ON i.code = CASE
       WHEN LOWER(t.availability) LIKE 'inmediat%' OR LOWER(t.availability) LIKE 'immediate%' THEN 'IMMEDIATE'
       WHEN LOWER(t.availability) LIKE '%2 semanas%' OR LOWER(t.availability) LIKE '%15 d%' THEN 'WEEKS_2'
       WHEN LOWER(t.availability) LIKE '%1 mes%' OR LOWER(t.availability) LIKE '%30 d%' THEN 'MONTH_1'
       WHEN LOWER(t.availability) LIKE '%convenir%' THEN 'TO_AGREE'
       WHEN LOWER(t.availability) LIKE 'no disponible%' THEN 'UNAVAILABLE' END
  JOIN master_table m ON m.id = i.master_table_id AND m.code = 'TALENT_AVAILABILITY' AND i.tenant_id IS NULL
   SET t.availability_item_id = i.id
 WHERE t.availability IS NOT NULL;

ALTER TABLE talent DROP COLUMN availability;
