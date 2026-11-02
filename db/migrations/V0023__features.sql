-- =====================================================================
-- Catálogo de funcionalidades
-- - feature: cada funcionalidad de la plataforma (módulo, roles a los que impacta). status SUSPENDED la apaga para
--   todos los tenants; is_core = no se puede suspender. default_enabled: si los tenants la tienen sin activarla.
--   Las funcionalidades nuevas llegan con una migración (default_enabled 0, sa_notified_at NULL): al iniciar, la API
--   avisa al Super Admin; luego él la activa por tenant.
-- - tenant_feature: activación o suspensión por tenant (sin fila = default_enabled).
-- Al activarla en un tenant se notifica a sus usuarios con los roles impactados.
-- =====================================================================

CREATE TABLE feature (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  code             VARCHAR(40)     NOT NULL,
  name_i18n        JSON            NOT NULL,
  description_i18n JSON            NOT NULL,
  module           VARCHAR(40)     NOT NULL,
  impacted_roles   JSON            NOT NULL,
  is_core          TINYINT(1)      NOT NULL DEFAULT 0,
  default_enabled  TINYINT(1)      NOT NULL DEFAULT 1,
  status           VARCHAR(10)     NOT NULL DEFAULT 'ACTIVE',
  sort_order       INT             NOT NULL DEFAULT 0,
  released_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  sa_notified_at   DATETIME(3)     NULL,
  updated_at       DATETIME(3)     NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by       BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_feature_code (code),
  CONSTRAINT ck_feature_status CHECK (status IN ('ACTIVE','SUSPENDED'))
) ENGINE=InnoDB;

CREATE TABLE tenant_feature (
  tenant_id  BIGINT UNSIGNED NOT NULL,
  feature_id BIGINT UNSIGNED NOT NULL,
  enabled    TINYINT(1)      NOT NULL,
  changed_by BIGINT UNSIGNED NULL,
  changed_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (tenant_id, feature_id),
  CONSTRAINT fk_tf_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_tf_feature FOREIGN KEY (feature_id) REFERENCES feature(id)
) ENGINE=InnoDB;

INSERT INTO permission (code, description) VALUES ('platform.features.manage', 'Activar y suspender funcionalidades de la plataforma y por tenant');
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code = 'platform.features.manage' WHERE r.code = 'SUPER_ADMIN';

-- Catálogo inicial: lo que ya existe (activo en todos los tenants, ya conocido por el Super Admin)
INSERT INTO feature (code, name_i18n, description_i18n, module, impacted_roles, is_core, default_enabled, sort_order, sa_notified_at) VALUES
 ('talents', JSON_OBJECT('es','Banco de talentos','en','Talent bank'),
   JSON_OBJECT('es','Registro, búsqueda y ficha de talentos con experiencia, estudios, habilidades e idiomas.','en','Talent registration, search and profile with experience, education, skills and languages.'),
   'talents', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER','COMMERCIAL','VIEWER'), 1, 1, 1, NOW(3)),
 ('cv_ai', JSON_OBJECT('es','Lectura de CV con IA','en','AI CV reading'),
   JSON_OBJECT('es','Carga rápida con CV y relectura del CV al editar: la IA completa los datos y los años por tecnología.','en','Quick load from a CV and CV re-reading: AI fills in the data and years per technology.'),
   'talents', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER','COMMERCIAL'), 0, 1, 2, NOW(3)),
 ('dni_lookup', JSON_OBJECT('es','Consulta de DNI','en','ID lookup'),
   JSON_OBJECT('es','Completa nombres y apellidos desde el DNI (RENIEC) al registrar un talento.','en','Fills in names from the national ID (RENIEC) when registering a talent.'),
   'talents', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER','COMMERCIAL'), 0, 1, 3, NOW(3)),
 ('cv_branding', JSON_OBJECT('es','CV con la marca de la empresa','en','Branded CV'),
   JSON_OBJECT('es','Genera el CV del talento con la plantilla y la marca de la empresa (español e inglés).','en','Generates the talent CV with the company template and brand (Spanish and English).'),
   'talents', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER','COMMERCIAL'), 0, 1, 4, NOW(3)),
 ('favorites', JSON_OBJECT('es','Favoritos y colecciones','en','Favorites and collections'),
   JSON_OBJECT('es','Colecciones de talentos favoritos que se pueden compartir con colegas.','en','Collections of favorite talents that can be shared with colleagues.'),
   'favorites', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER','COMMERCIAL'), 0, 1, 5, NOW(3)),
 ('interviews', JSON_OBJECT('es','Entrevistas','en','Interviews'),
   JSON_OBJECT('es','Agenda y registro de entrevistas con calificación y respuestas.','en','Interview scheduling and recording with ratings and answers.'),
   'interviews', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER','COMMERCIAL','VIEWER'), 0, 1, 6, NOW(3)),
 ('blacklist', JSON_OBJECT('es','Lista negra','en','Blacklist'),
   JSON_OBJECT('es','Restricciones de talentos para todos los clientes o para algunos, con aviso al usarlos.','en','Talent restrictions for all or some clients, with warnings when used.'),
   'blacklist', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER','COMMERCIAL'), 0, 1, 7, NOW(3)),
 ('requirements', JSON_OBJECT('es','Requerimientos (RQ)','en','Requirements'),
   JSON_OBJECT('es','Tablero Kanban de requerimientos con vistas, filtros y pipeline de candidatos por etapas.','en','Requirements Kanban board with views, filters and a staged candidate pipeline.'),
   'requirements', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER','COMMERCIAL','VIEWER'), 0, 1, 8, NOW(3)),
 ('rq_reopen', JSON_OBJECT('es','Reapertura de RQ','en','Requirement reopening'),
   JSON_OBJECT('es','El jefe de selección reabre un RQ cerrado por renuncia de un talento o por más vacantes.','en','The selection lead reopens a closed requirement due to a resignation or more openings.'),
   'requirements', JSON_ARRAY('SELECTION_LEAD'), 0, 1, 9, NOW(3)),
 ('rq_documents', JSON_OBJECT('es','Documentos de postulación','en','Application documents'),
   JSON_OBJECT('es','Maestro de documentos, documentos pedidos por RQ, carga y validación manual con línea de tiempo del postulante.','en','Document list, documents per requirement, upload and manual check with the applicant timeline.'),
   'requirements', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER'), 0, 1, 10, NOW(3)),
 ('cul_auto', JSON_OBJECT('es','Validación automática del CUL','en','Automatic CUL check'),
   JSON_OBJECT('es','Verifica el Certificado Único Laboral contra el MTPE con el RUC de la empresa (requiere RUC registrado).','en','Checks the Single Labor Certificate with the MTPE using the company tax ID (tax ID required).'),
   'requirements', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER'), 0, 1, 11, NOW(3)),
 ('push', JSON_OBJECT('es','Notificaciones push','en','Push notifications'),
   JSON_OBJECT('es','Avisos del sistema operativo en la app instalada.','en','Operating system notifications in the installed app.'),
   'notifications', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER','COMMERCIAL','VIEWER'), 0, 1, 12, NOW(3)),
 ('support', JSON_OBJECT('es','Soporte y sugerencias','en','Support and suggestions'),
   JSON_OBJECT('es','Reportar incidencias, observaciones y mejoras al equipo de la plataforma desde cualquier pantalla.','en','Report incidents, observations and improvements to the platform team from any screen.'),
   'support', JSON_ARRAY('TENANT_ADMIN','SELECTION_LEAD','RECRUITER','COMMERCIAL','VIEWER'), 0, 1, 13, NOW(3)),
 ('api_keys', JSON_OBJECT('es','API de integración','en','Integration API'),
   JSON_OBJECT('es','API keys para integrar otros sistemas (aprobadas por el Super Admin).','en','API keys to integrate other systems (approved by the Super Admin).'),
   'administration', JSON_ARRAY('TENANT_ADMIN'), 0, 1, 14, NOW(3)),
 ('administration', JSON_OBJECT('es','Administración de la empresa','en','Company administration'),
   JSON_OBJECT('es','Usuarios, marca, clientes y maestros de la empresa.','en','Users, brand, clients and company lists.'),
   'administration', JSON_ARRAY('TENANT_ADMIN'), 1, 1, 15, NOW(3));
