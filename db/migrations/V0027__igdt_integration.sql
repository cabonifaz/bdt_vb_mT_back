-- Integración con IGDT (Ingreso y Gestión de Talentos): IGDT gobierna los catálogos compartidos (clientes y contactos,
-- perfiles, tarifario, modalidades de contrato, monedas, modalidad de trabajo, plazos, áreas y motivos).
-- - Sin integración, el admin de BDT administra todos sus maestros como siempre.
-- - Con la integración ACTIVE, esos catálogos son una copia de solo lectura que BDT sincroniza desde IGDT por API.
-- - tenant_integration: token de IGDT del tenant (cifrado), estado, cursor de sincronización y último resultado.
-- - master_table.igdt_shared: catálogos que gobierna IGDT.
-- - igdt_map: id en IGDT de cada registro local, por tenant, cuando difiere del de BDT (catálogos globales sembrados por
--   separado, perfiles globales de BDT que en IGDT son del tenant o códigos que ya existían en IGDT).
-- - tariff: copia del tarifario de IGDT (propone la tarifa al crear una vacante).

CREATE TABLE tenant_integration (
  tenant_id     BIGINT UNSIGNED NOT NULL,
  provider      VARCHAR(20)     NOT NULL,
  api_key_enc   VARBINARY(1024) NOT NULL,
  remote_tenant VARCHAR(60)     NULL,
  status        VARCHAR(10)     NOT NULL DEFAULT 'ACTIVE',
  sync_cursor   BIGINT UNSIGNED NOT NULL DEFAULT 0,
  imported_at   DATETIME(3)     NULL,
  last_sync_at  DATETIME(3)     NULL,
  last_error    VARCHAR(500)    NULL,
  created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by    BIGINT UNSIGNED NULL,
  updated_at    DATETIME(3)     NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by    BIGINT UNSIGNED NULL,
  PRIMARY KEY (tenant_id, provider),
  CONSTRAINT fk_tenant_integration_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT ck_tenant_integration_provider CHECK (provider IN ('IGDT')),
  CONSTRAINT ck_tenant_integration_status CHECK (status IN ('ACTIVE', 'DISABLED'))
) ENGINE=InnoDB;

ALTER TABLE master_table ADD COLUMN igdt_shared TINYINT(1) NOT NULL DEFAULT 0;
CREATE TABLE igdt_map (
  tenant_id   BIGINT UNSIGNED NOT NULL,
  entity      VARCHAR(20)     NOT NULL,
  remote_uuid CHAR(36)        NOT NULL,
  local_id    BIGINT UNSIGNED NOT NULL,
  created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (tenant_id, entity, remote_uuid),
  UNIQUE KEY uq_igdt_map_local (tenant_id, entity, local_id),
  CONSTRAINT fk_igdt_map_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT ck_igdt_map_entity CHECK (entity IN ('MASTER_ITEM', 'CLIENT_CONTACT'))
) ENGINE=InnoDB;

-- catálogos que BDT aún no tenía y que IGDT comparte
INSERT INTO master_table (code, name_i18n, scope, is_system) VALUES
 ('RATE_TYPE',          JSON_OBJECT('es','Tipos de tarifa','en','Rate types'), 'GLOBAL', 1),
 ('AREA',               JSON_OBJECT('es','Áreas o unidades','en','Areas'), 'BOTH', 0),
 ('HIRE_REASON',        JSON_OBJECT('es','Motivos de ingreso','en','Hire reasons'), 'BOTH', 0),
 ('TERMINATION_REASON', JSON_OBJECT('es','Motivos de cese','en','Termination reasons'), 'BOTH', 0);

UPDATE master_table SET igdt_shared = 1
 WHERE code IN ('CLIENT', 'RQ_PROFILE', 'CONTRACT_MODALITY', 'CURRENCY', 'WORK_MODALITY', 'TIME_UNIT',
                'RATE_TYPE', 'AREA', 'HIRE_REASON', 'TERMINATION_REASON');

CREATE TABLE tariff (
  id                BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid              CHAR(36)        NOT NULL,
  tenant_id         BIGINT UNSIGNED NOT NULL,
  client_item_id    BIGINT UNSIGNED NULL,
  profile_item_id   BIGINT UNSIGNED NOT NULL,
  currency_item_id  BIGINT UNSIGNED NOT NULL,
  rate_type_item_id BIGINT UNSIGNED NOT NULL,
  rate              DECIMAL(12,2)   NOT NULL,
  valid_from        DATE            NOT NULL,
  valid_to          DATE            NULL,
  synced_at         DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  deleted_at        DATETIME(3)     NULL,
  UNIQUE KEY uq_tariff_uuid (uuid),
  KEY ix_tariff_lookup (tenant_id, profile_item_id, client_item_id, valid_from),
  CONSTRAINT fk_tariff_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_tariff_client FOREIGN KEY (client_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_tariff_profile FOREIGN KEY (profile_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_tariff_currency FOREIGN KEY (currency_item_id) REFERENCES master_item(id),
  CONSTRAINT fk_tariff_rate_type FOREIGN KEY (rate_type_item_id) REFERENCES master_item(id)
) ENGINE=InnoDB;

INSERT INTO permission (code, description) VALUES ('tenant.integrations.manage', 'Conectar la empresa con IGDT y sincronizar sus catálogos');
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code = 'tenant.integrations.manage' WHERE r.code = 'TENANT_ADMIN';
