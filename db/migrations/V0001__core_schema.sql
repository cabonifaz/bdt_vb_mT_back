-- =====================================================================
-- Banco de Talentos MT - Núcleo de plataforma
-- Multi-tenant, maestro de maestros, seguridad, auditoría, sincronización
-- Convención: todas las tablas llevan campos de auditoría y borrado lógico.
-- Datos sensibles (*_enc) se cifran en la aplicación (AES-256-GCM) y se
-- buscan por índice ciego (*_bidx = HMAC-SHA256). La BD nunca ve texto plano.
-- =====================================================================

-- ---------------------------------------------------------------------
-- Tenants
-- ---------------------------------------------------------------------
CREATE TABLE tenant (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid            CHAR(36)      NOT NULL,
  slug            VARCHAR(60)   NOT NULL,
  name            VARCHAR(150)  NOT NULL,
  legal_name      VARCHAR(250)  NULL,
  tax_id          VARCHAR(30)   NULL,
  status          VARCHAR(20)   NOT NULL DEFAULT 'ACTIVE',  -- ACTIVE | SUSPENDED | CANCELLED
  default_locale  VARCHAR(10)   NOT NULL DEFAULT 'es',
  timezone        VARCHAR(60)   NOT NULL DEFAULT 'America/Lima',
  settings        JSON          NULL,
  created_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by      BIGINT UNSIGNED NULL,
  updated_at      DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by      BIGINT UNSIGNED NULL,
  deleted_at      DATETIME(3)   NULL,
  deleted_by      BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_tenant_uuid (uuid),
  UNIQUE KEY uq_tenant_slug (slug),
  CONSTRAINT ck_tenant_status CHECK (status IN ('ACTIVE','SUSPENDED','CANCELLED'))
) ENGINE=InnoDB;

-- Marca por tenant: logos y colores los configura el administrador del tenant
CREATE TABLE tenant_branding (
  tenant_id        BIGINT UNSIGNED PRIMARY KEY,
  display_name     VARCHAR(150)  NOT NULL,
  logo_file_id     BIGINT UNSIGNED NULL,
  logo_dark_file_id BIGINT UNSIGNED NULL,
  favicon_file_id  BIGINT UNSIGNED NULL,
  logo_url         VARCHAR(500)  NULL,          -- recurso estático por defecto (seed)
  color_primary    CHAR(7)       NOT NULL DEFAULT '#009688',
  color_primary_hover CHAR(7)    NOT NULL DEFAULT '#2D8D8D',
  color_secondary  CHAR(7)       NOT NULL DEFAULT '#0B85C3',
  color_accent     CHAR(7)       NOT NULL DEFAULT '#FAAB34',
  color_sidebar    CHAR(7)       NOT NULL DEFAULT '#FFFFFF',
  font_family      VARCHAR(80)   NOT NULL DEFAULT 'Inter',
  extra            JSON          NULL,
  created_at       DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by       BIGINT UNSIGNED NULL,
  updated_at       DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by       BIGINT UNSIGNED NULL,
  deleted_at       DATETIME(3)   NULL,
  deleted_by       BIGINT UNSIGNED NULL,
  CONSTRAINT fk_branding_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT ck_branding_colors CHECK (
    color_primary REGEXP '^#[0-9A-Fa-f]{6}$' AND color_primary_hover REGEXP '^#[0-9A-Fa-f]{6}$' AND
    color_secondary REGEXP '^#[0-9A-Fa-f]{6}$' AND color_accent REGEXP '^#[0-9A-Fa-f]{6}$' AND
    color_sidebar REGEXP '^#[0-9A-Fa-f]{6}$')
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- Maestro de maestros
-- master_table = catálogo de catálogos; master_item = sus valores.
-- tenant_id NULL = valor global de plataforma; con tenant_id = propio del tenant.
-- label_i18n = {"es":"...","en":"..."} para multi idioma.
-- ---------------------------------------------------------------------
CREATE TABLE master_table (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  code            VARCHAR(60)   NOT NULL,
  name_i18n       JSON          NOT NULL,
  description     VARCHAR(500)  NULL,
  parent_table_id BIGINT UNSIGNED NULL,          -- jerarquías (país -> ciudad)
  scope           VARCHAR(10)   NOT NULL DEFAULT 'GLOBAL', -- GLOBAL | TENANT | BOTH
  is_system       TINYINT(1)    NOT NULL DEFAULT 0,        -- no editable por tenants
  legacy_id       INT           NULL,            -- ID_MAESTRO del sistema anterior
  created_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by      BIGINT UNSIGNED NULL,
  updated_at      DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by      BIGINT UNSIGNED NULL,
  deleted_at      DATETIME(3)   NULL,
  deleted_by      BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_master_table_code (code),
  CONSTRAINT fk_master_table_parent FOREIGN KEY (parent_table_id) REFERENCES master_table(id),
  CONSTRAINT ck_master_table_scope CHECK (scope IN ('GLOBAL','TENANT','BOTH'))
) ENGINE=InnoDB;

CREATE TABLE master_item (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid            CHAR(36)      NOT NULL,
  master_table_id BIGINT UNSIGNED NOT NULL,
  tenant_id       BIGINT UNSIGNED NULL,
  code            VARCHAR(60)   NOT NULL,
  label_i18n      JSON          NOT NULL,
  parent_item_id  BIGINT UNSIGNED NULL,
  num1 DECIMAL(18,4) NULL, num2 DECIMAL(18,4) NULL, num3 DECIMAL(18,4) NULL,
  str1 VARCHAR(250)  NULL, str2 VARCHAR(250)  NULL, str3 VARCHAR(250)  NULL,
  date1 DATE NULL, date2 DATE NULL, date3 DATE NULL,
  sort_order      INT           NOT NULL DEFAULT 0,
  is_active       TINYINT(1)    NOT NULL DEFAULT 1,
  legacy_id       INT           NULL,            -- ID_PARAMETRO del sistema anterior
  tenant_key      BIGINT UNSIGNED AS (IFNULL(tenant_id, 0)) STORED,
  created_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by      BIGINT UNSIGNED NULL,
  updated_at      DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by      BIGINT UNSIGNED NULL,
  deleted_at      DATETIME(3)   NULL,
  deleted_by      BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_master_item_uuid (uuid),
  UNIQUE KEY uq_master_item_code (master_table_id, tenant_key, code),
  KEY ix_master_item_tenant (tenant_id, master_table_id),
  KEY ix_master_item_parent (parent_item_id),
  CONSTRAINT fk_master_item_table  FOREIGN KEY (master_table_id) REFERENCES master_table(id),
  CONSTRAINT fk_master_item_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_master_item_parent FOREIGN KEY (parent_item_id) REFERENCES master_item(id)
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- Seguridad: roles, permisos, usuarios
-- ---------------------------------------------------------------------
CREATE TABLE role (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  code        VARCHAR(40)  NOT NULL,          -- SUPER_ADMIN, TENANT_ADMIN, RECRUITER, ...
  name_i18n   JSON         NOT NULL,
  is_platform TINYINT(1)   NOT NULL DEFAULT 0, -- 1 = rol de plataforma (no pertenece a tenant)
  created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by  BIGINT UNSIGNED NULL,
  updated_at  DATETIME(3)  NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by  BIGINT UNSIGNED NULL,
  deleted_at  DATETIME(3)  NULL,
  deleted_by  BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_role_code (code)
) ENGINE=InnoDB;

CREATE TABLE permission (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  code        VARCHAR(80)  NOT NULL,          -- p.ej. talent.read, tenant.manage
  description VARCHAR(250) NULL,
  created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by  BIGINT UNSIGNED NULL,
  updated_at  DATETIME(3)  NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by  BIGINT UNSIGNED NULL,
  deleted_at  DATETIME(3)  NULL,
  deleted_by  BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_permission_code (code)
) ENGINE=InnoDB;

CREATE TABLE role_permission (
  role_id       BIGINT UNSIGNED NOT NULL,
  permission_id BIGINT UNSIGNED NOT NULL,
  created_at    DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by    BIGINT UNSIGNED NULL,
  PRIMARY KEY (role_id, permission_id),
  CONSTRAINT fk_rp_role FOREIGN KEY (role_id) REFERENCES role(id),
  CONSTRAINT fk_rp_perm FOREIGN KEY (permission_id) REFERENCES permission(id)
) ENGINE=InnoDB;

CREATE TABLE app_user (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid             CHAR(36)      NOT NULL,
  tenant_id        BIGINT UNSIGNED NULL,       -- NULL = usuario de plataforma (Super Admin)
  email_enc        VARBINARY(512) NOT NULL,
  email_bidx       CHAR(64)      NOT NULL,
  full_name_enc    VARBINARY(512) NOT NULL,
  phone_enc        VARBINARY(256) NULL,
  password_hash    VARCHAR(255)  NULL,         -- argon2id (NULL hasta aceptar invitación)
  locale           VARCHAR(10)   NOT NULL DEFAULT 'es',
  status           VARCHAR(20)   NOT NULL DEFAULT 'ACTIVE', -- INVITED | ACTIVE | LOCKED | DISABLED
  failed_logins    INT           NOT NULL DEFAULT 0,
  locked_until     DATETIME(3)   NULL,
  mfa_secret_enc   VARBINARY(256) NULL,
  last_login_at    DATETIME(3)   NULL,
  password_changed_at DATETIME(3) NULL,
  tenant_key       BIGINT UNSIGNED AS (IFNULL(tenant_id, 0)) STORED,
  created_at       DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by       BIGINT UNSIGNED NULL,
  updated_at       DATETIME(3)   NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by       BIGINT UNSIGNED NULL,
  deleted_at       DATETIME(3)   NULL,
  deleted_by       BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_user_uuid (uuid),
  UNIQUE KEY uq_user_email_tenant (tenant_key, email_bidx),
  KEY ix_user_email (email_bidx),
  CONSTRAINT fk_user_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT ck_user_status CHECK (status IN ('INVITED','ACTIVE','LOCKED','DISABLED'))
) ENGINE=InnoDB;

CREATE TABLE user_role (
  user_id    BIGINT UNSIGNED NOT NULL,
  role_id    BIGINT UNSIGNED NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by BIGINT UNSIGNED NULL,
  PRIMARY KEY (user_id, role_id),
  CONSTRAINT fk_ur_user FOREIGN KEY (user_id) REFERENCES app_user(id),
  CONSTRAINT fk_ur_role FOREIGN KEY (role_id) REFERENCES role(id)
) ENGINE=InnoDB;

-- Sesiones (refresh tokens rotativos, solo se guarda el hash)
CREATE TABLE user_session (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid          CHAR(36)     NOT NULL,
  user_id       BIGINT UNSIGNED NOT NULL,
  token_hash    CHAR(64)     NOT NULL,
  family_id     CHAR(36)     NOT NULL,         -- detecta reuso de refresh token
  expires_at    DATETIME(3)  NOT NULL,
  revoked_at    DATETIME(3)  NULL,
  ip            VARCHAR(45)  NULL,
  user_agent    VARCHAR(300) NULL,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uq_session_uuid (uuid),
  UNIQUE KEY uq_session_token (token_hash),
  KEY ix_session_user (user_id),
  KEY ix_session_family (family_id),
  CONSTRAINT fk_session_user FOREIGN KEY (user_id) REFERENCES app_user(id)
) ENGINE=InnoDB;

-- Invitaciones de usuario (token de un solo uso, se guarda solo el hash)
CREATE TABLE user_invitation (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid          CHAR(36)     NOT NULL,
  tenant_id     BIGINT UNSIGNED NULL,
  email_enc     VARBINARY(512) NOT NULL,
  email_bidx    CHAR(64)     NOT NULL,
  role_id       BIGINT UNSIGNED NOT NULL,
  token_hash    CHAR(64)     NOT NULL,
  status        VARCHAR(20)  NOT NULL DEFAULT 'PENDING', -- PENDING | ACCEPTED | REVOKED | EXPIRED
  expires_at    DATETIME(3)  NOT NULL,
  accepted_at   DATETIME(3)  NULL,
  accepted_user_id BIGINT UNSIGNED NULL,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by    BIGINT UNSIGNED NULL,
  updated_at    DATETIME(3)  NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by    BIGINT UNSIGNED NULL,
  deleted_at    DATETIME(3)  NULL,
  deleted_by    BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_invitation_uuid (uuid),
  UNIQUE KEY uq_invitation_token (token_hash),
  KEY ix_invitation_tenant (tenant_id, status),
  CONSTRAINT fk_inv_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_inv_role FOREIGN KEY (role_id) REFERENCES role(id),
  CONSTRAINT ck_inv_status CHECK (status IN ('PENDING','ACCEPTED','REVOKED','EXPIRED'))
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- API pública de integración: keys por tenant, habilitadas por el Super Admin
-- ---------------------------------------------------------------------
CREATE TABLE api_key (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid          CHAR(36)     NOT NULL,
  tenant_id     BIGINT UNSIGNED NOT NULL,
  name          VARCHAR(120) NOT NULL,
  key_prefix    CHAR(12)     NOT NULL,         -- visible para identificarla
  key_hash      CHAR(64)     NOT NULL,         -- HMAC-SHA256 del secreto
  scopes        JSON         NOT NULL,
  status        VARCHAR(20)  NOT NULL DEFAULT 'PENDING', -- PENDING | ENABLED | DISABLED | REVOKED
  enabled_by    BIGINT UNSIGNED NULL,
  enabled_at    DATETIME(3)  NULL,
  expires_at    DATETIME(3)  NULL,
  last_used_at  DATETIME(3)  NULL,
  last_used_ip  VARCHAR(45)  NULL,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by    BIGINT UNSIGNED NULL,
  updated_at    DATETIME(3)  NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by    BIGINT UNSIGNED NULL,
  deleted_at    DATETIME(3)  NULL,
  deleted_by    BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_api_key_uuid (uuid),
  UNIQUE KEY uq_api_key_prefix (key_prefix),
  KEY ix_api_key_tenant (tenant_id),
  CONSTRAINT fk_api_key_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT ck_api_key_status CHECK (status IN ('PENDING','ENABLED','DISABLED','REVOKED'))
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- Archivos (metadatos; el binario vive en el volumen persistente STORAGE_PATH)
-- ---------------------------------------------------------------------
CREATE TABLE file_object (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid          CHAR(36)     NOT NULL,
  tenant_id     BIGINT UNSIGNED NULL,
  owner_type    VARCHAR(40)  NOT NULL,         -- TENANT_BRANDING, TALENT, ...
  owner_id      BIGINT UNSIGNED NULL,
  category      VARCHAR(40)  NOT NULL,         -- LOGO, FAVICON, CV, PHOTO, ...
  storage_key   VARCHAR(500) NOT NULL,
  original_name VARCHAR(255) NOT NULL,
  mime_type     VARCHAR(120) NOT NULL,
  size_bytes    BIGINT UNSIGNED NOT NULL,
  sha256        CHAR(64)     NOT NULL,
  is_encrypted  TINYINT(1)   NOT NULL DEFAULT 0,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by    BIGINT UNSIGNED NULL,
  updated_at    DATETIME(3)  NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by    BIGINT UNSIGNED NULL,
  deleted_at    DATETIME(3)  NULL,
  deleted_by    BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_file_uuid (uuid),
  KEY ix_file_owner (tenant_id, owner_type, owner_id),
  CONSTRAINT fk_file_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id)
) ENGINE=InnoDB;

ALTER TABLE tenant_branding
  ADD CONSTRAINT fk_branding_logo    FOREIGN KEY (logo_file_id)      REFERENCES file_object(id),
  ADD CONSTRAINT fk_branding_logo_dk FOREIGN KEY (logo_dark_file_id) REFERENCES file_object(id),
  ADD CONSTRAINT fk_branding_favicon FOREIGN KEY (favicon_file_id)   REFERENCES file_object(id);

-- ---------------------------------------------------------------------
-- Formatos y reportes configurables por tenant
-- ---------------------------------------------------------------------
CREATE TABLE report_template (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid          CHAR(36)     NOT NULL,
  tenant_id     BIGINT UNSIGNED NULL,          -- NULL = plantilla base de plataforma
  code          VARCHAR(60)  NOT NULL,         -- CV_STANDARD, TALENT_LIST, ...
  locale        VARCHAR(10)  NOT NULL DEFAULT 'es',
  name          VARCHAR(150) NOT NULL,
  engine        VARCHAR(20)  NOT NULL DEFAULT 'JSON_LAYOUT',
  definition    JSON         NOT NULL,
  version       INT          NOT NULL DEFAULT 1,
  is_active     TINYINT(1)   NOT NULL DEFAULT 1,
  tenant_key    BIGINT UNSIGNED AS (IFNULL(tenant_id, 0)) STORED,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by    BIGINT UNSIGNED NULL,
  updated_at    DATETIME(3)  NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by    BIGINT UNSIGNED NULL,
  deleted_at    DATETIME(3)  NULL,
  deleted_by    BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_report_uuid (uuid),
  UNIQUE KEY uq_report_code (tenant_key, code, locale),
  CONSTRAINT fk_report_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id)
) ENGINE=InnoDB;

-- Traducciones propias del tenant (sobrescriben textos de la UI)
CREATE TABLE tenant_translation (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  tenant_id     BIGINT UNSIGNED NOT NULL,
  locale        VARCHAR(10)  NOT NULL,
  tkey          VARCHAR(150) NOT NULL,
  tvalue        VARCHAR(1000) NOT NULL,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by    BIGINT UNSIGNED NULL,
  updated_at    DATETIME(3)  NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by    BIGINT UNSIGNED NULL,
  deleted_at    DATETIME(3)  NULL,
  deleted_by    BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_tenant_translation (tenant_id, locale, tkey),
  CONSTRAINT fk_tt_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id)
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- Sincronización offline: cada operación del cliente trae un id único
-- (idempotencia) y se registra aquí antes de aplicarse.
-- ---------------------------------------------------------------------
CREATE TABLE sync_operation (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  tenant_id     BIGINT UNSIGNED NOT NULL,
  user_id       BIGINT UNSIGNED NOT NULL,
  client_op_id  CHAR(36)     NOT NULL,
  entity        VARCHAR(60)  NOT NULL,
  operation     VARCHAR(20)  NOT NULL,
  entity_uuid   CHAR(36)     NULL,
  client_ts     DATETIME(3)  NOT NULL,
  status        VARCHAR(20)  NOT NULL DEFAULT 'APPLIED', -- APPLIED | CONFLICT | REJECTED
  result        JSON         NULL,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uq_sync_op (tenant_id, client_op_id),
  KEY ix_sync_user (user_id, created_at),
  CONSTRAINT fk_sync_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_sync_user FOREIGN KEY (user_id) REFERENCES app_user(id)
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- Auditoría
-- ---------------------------------------------------------------------
CREATE TABLE audit_log (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  tenant_id     BIGINT UNSIGNED NULL,
  user_id       BIGINT UNSIGNED NULL,
  api_key_id    BIGINT UNSIGNED NULL,
  action        VARCHAR(60)  NOT NULL,
  entity        VARCHAR(60)  NOT NULL,
  entity_id     VARCHAR(64)  NULL,
  detail        JSON         NULL,
  ip            VARCHAR(45)  NULL,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY ix_audit_tenant (tenant_id, created_at),
  KEY ix_audit_entity (entity, entity_id)
) ENGINE=InnoDB;
