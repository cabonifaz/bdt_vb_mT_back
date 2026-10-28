-- Consumo de servicios externos de pago por tenant (consultas de DNI y tokens de IA), acumulado por día (UTC).
-- Lo ve el Super Admin para controlar costos.
CREATE TABLE tenant_usage_daily (
  tenant_id         BIGINT UNSIGNED NOT NULL,
  usage_date        DATE            NOT NULL,
  kind              VARCHAR(20)     NOT NULL,  -- DNI_LOOKUP | AI_TRANSLATE
  provider          VARCHAR(60)     NOT NULL,  -- proveedor de DNI o modelo de IA
  calls             INT UNSIGNED    NOT NULL DEFAULT 0,
  errors            INT UNSIGNED    NOT NULL DEFAULT 0,
  prompt_tokens     BIGINT UNSIGNED NOT NULL DEFAULT 0,
  completion_tokens BIGINT UNSIGNED NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, usage_date, kind, provider),
  KEY ix_usage_date (usage_date),
  CONSTRAINT fk_usage_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT ck_usage_kind CHECK (kind IN ('DNI_LOOKUP','AI_TRANSLATE'))
);

INSERT INTO permission (code, description) VALUES ('platform.usage.read', 'Ver el consumo de DNI y tokens de IA de cada tenant');
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code = 'platform.usage.read' WHERE r.code = 'SUPER_ADMIN';
