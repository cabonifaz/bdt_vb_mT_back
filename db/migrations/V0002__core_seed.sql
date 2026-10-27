-- =====================================================================
-- Datos base: roles, permisos, maestros de plataforma y tenant Fractal
-- =====================================================================

INSERT INTO role (code, name_i18n, is_platform) VALUES
 ('SUPER_ADMIN',  JSON_OBJECT('es','Super administrador','en','Super administrator'), 1),
 ('TENANT_ADMIN', JSON_OBJECT('es','Administrador','en','Administrator'), 0),
 ('RECRUITER',    JSON_OBJECT('es','Reclutador','en','Recruiter'), 0),
 ('COMMERCIAL',   JSON_OBJECT('es','Comercial','en','Sales'), 0),
 ('VIEWER',       JSON_OBJECT('es','Consulta','en','Viewer'), 0);

INSERT INTO permission (code, description) VALUES
 ('platform.tenants.manage',  'Crear, editar y suspender tenants'),
 ('platform.apikeys.approve', 'Habilitar o revocar API keys de cualquier tenant'),
 ('platform.audit.read',      'Ver auditoría de toda la plataforma'),
 ('platform.masters.manage',  'Administrar maestros globales'),
 ('tenant.users.manage',      'Administrar usuarios del tenant'),
 ('tenant.users.invite',      'Invitar usuarios al tenant'),
 ('tenant.branding.manage',   'Configurar logos y colores del tenant'),
 ('tenant.masters.manage',    'Administrar maestros propios del tenant'),
 ('tenant.reports.manage',    'Configurar formatos y reportes del tenant'),
 ('tenant.translations.manage','Personalizar textos del tenant'),
 ('tenant.apikeys.request',   'Solicitar API keys de integración'),
 ('tenant.audit.read',        'Ver auditoría del tenant'),
 ('talent.read',              'Consultar talentos'),
 ('talent.write',             'Registrar y editar talentos'),
 ('talent.rates.read',        'Ver tarifas y expectativas salariales');

-- Super Admin: todo lo de plataforma
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code LIKE 'platform.%' WHERE r.code = 'SUPER_ADMIN';
-- Admin del tenant: todo lo del tenant y del negocio
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON (p.code LIKE 'tenant.%' OR p.code LIKE 'talent.%') WHERE r.code = 'TENANT_ADMIN';
-- Reclutador: talentos sin tarifas (equivale a la regla RECLUTADOR del sistema anterior)
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code IN ('talent.read','talent.write') WHERE r.code = 'RECRUITER';
-- Comercial: talentos con tarifas
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code IN ('talent.read','talent.write','talent.rates.read') WHERE r.code = 'COMMERCIAL';
-- Consulta
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code = 'talent.read' WHERE r.code = 'VIEWER';

-- ---------------------------------------------------------------------
-- Maestro de maestros: catálogos de plataforma conocidos.
-- El resto de catálogos (y sus legacy_id) se importa desde el sistema
-- anterior con la herramienta de migración.
-- ---------------------------------------------------------------------
INSERT INTO master_table (code, name_i18n, scope, is_system) VALUES
 ('LANGUAGE',       JSON_OBJECT('es','Idiomas','en','Languages'), 'GLOBAL', 1),
 ('LANGUAGE_LEVEL', JSON_OBJECT('es','Niveles de idioma','en','Language levels'), 'GLOBAL', 1),
 ('ACADEMIC_GRADE', JSON_OBJECT('es','Grados académicos','en','Academic degrees'), 'BOTH', 0),
 ('CURRENCY',       JSON_OBJECT('es','Monedas','en','Currencies'), 'GLOBAL', 1),
 ('COUNTRY',        JSON_OBJECT('es','Países','en','Countries'), 'GLOBAL', 1),
 ('DOCUMENT_TYPE',  JSON_OBJECT('es','Tipos de documento','en','Document types'), 'BOTH', 0),
 ('TECH_SKILL',     JSON_OBJECT('es','Habilidades técnicas','en','Technical skills'), 'BOTH', 0),
 ('SOFT_SKILL',     JSON_OBJECT('es','Habilidades blandas','en','Soft skills'), 'BOTH', 0);

INSERT INTO master_table (code, name_i18n, scope, is_system, parent_table_id)
SELECT 'CITY', JSON_OBJECT('es','Ciudades','en','Cities'), 'GLOBAL', 1, id FROM master_table WHERE code = 'COUNTRY';

INSERT INTO master_item (uuid, master_table_id, code, label_i18n, sort_order, legacy_id)
SELECT UUID(), t.id, v.code, v.label, v.ord, v.legacy
FROM master_table t
JOIN (
  SELECT 'LANGUAGE' tbl, 'ES' code, JSON_OBJECT('es','Español','en','Spanish') label, 1 ord, 1 legacy UNION ALL
  SELECT 'LANGUAGE', 'EN', JSON_OBJECT('es','Inglés','en','English'), 2, 2 UNION ALL
  SELECT 'LANGUAGE', 'FR', JSON_OBJECT('es','Francés','en','French'), 3, 3 UNION ALL
  SELECT 'LANGUAGE', 'DE', JSON_OBJECT('es','Alemán','en','German'), 4, 4 UNION ALL
  SELECT 'LANGUAGE', 'ZH', JSON_OBJECT('es','Chino','en','Chinese'), 5, 5 UNION ALL
  SELECT 'LANGUAGE_LEVEL', 'BASIC', JSON_OBJECT('es','Básico','en','Basic'), 1, 1 UNION ALL
  SELECT 'LANGUAGE_LEVEL', 'INTERMEDIATE', JSON_OBJECT('es','Intermedio','en','Intermediate'), 2, 2 UNION ALL
  SELECT 'LANGUAGE_LEVEL', 'ADVANCED', JSON_OBJECT('es','Avanzado','en','Advanced'), 3, 3 UNION ALL
  SELECT 'LANGUAGE_LEVEL', 'NATIVE', JSON_OBJECT('es','Nativo','en','Native'), 4, 4 UNION ALL
  SELECT 'ACADEMIC_GRADE', 'BACHELOR', JSON_OBJECT('es','Bachiller','en','Bachelor'), 1, 1 UNION ALL
  SELECT 'ACADEMIC_GRADE', 'DEGREE', JSON_OBJECT('es','Título','en','Degree'), 2, 2 UNION ALL
  SELECT 'ACADEMIC_GRADE', 'COURSE', JSON_OBJECT('es','Curso','en','Course'), 3, 3 UNION ALL
  SELECT 'ACADEMIC_GRADE', 'TECHNICAL', JSON_OBJECT('es','Técnico','en','Technical'), 4, 4 UNION ALL
  SELECT 'ACADEMIC_GRADE', 'GRADUATE', JSON_OBJECT('es','Egresado','en','Graduate'), 5, 5 UNION ALL
  SELECT 'ACADEMIC_GRADE', 'STUDENT', JSON_OBJECT('es','Estudiante','en','Student'), 6, 6 UNION ALL
  SELECT 'CURRENCY', 'PEN', JSON_OBJECT('es','Soles','en','Peruvian sol'), 1, NULL UNION ALL
  SELECT 'CURRENCY', 'USD', JSON_OBJECT('es','Dólares','en','US dollar'), 2, NULL UNION ALL
  SELECT 'COUNTRY', 'PE', JSON_OBJECT('es','Perú','en','Peru'), 1, NULL UNION ALL
  SELECT 'DOCUMENT_TYPE', 'CV', JSON_OBJECT('es','CV original','en','Original CV'), 1, 1 UNION ALL
  SELECT 'DOCUMENT_TYPE', 'CV_TENANT_ES', JSON_OBJECT('es','CV formato empresa (ES)','en','Company CV (ES)'), 5, 5 UNION ALL
  SELECT 'DOCUMENT_TYPE', 'CV_TENANT_EN', JSON_OBJECT('es','CV formato empresa (EN)','en','Company CV (EN)'), 6, 6 UNION ALL
  SELECT 'DOCUMENT_TYPE', 'CERTIFICATE', JSON_OBJECT('es','Certificado o diploma','en','Certificate'), 7, NULL
) v ON v.tbl = t.code;

-- ---------------------------------------------------------------------
-- Tenant Fractal (primera versión: mismo logo y colores que producción)
-- ---------------------------------------------------------------------
INSERT INTO tenant (uuid, slug, name, legal_name, default_locale, timezone)
VALUES (UUID(), 'fractal', 'Fractal', 'Fractal Soluciones TI', 'es', 'America/Lima');

INSERT INTO tenant_branding (tenant_id, display_name, logo_url, color_primary, color_primary_hover,
                             color_secondary, color_accent, color_sidebar, font_family)
SELECT id, 'Fractal - Banco de talentos', '/tenants/fractal/logo.png',
       '#009688', '#2D8D8D', '#0B85C3', '#FAAB34', '#FFFFFF', 'Inter'
FROM tenant WHERE slug = 'fractal';
