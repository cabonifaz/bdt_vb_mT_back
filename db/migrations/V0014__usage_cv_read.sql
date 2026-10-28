-- Lectura de CV con IA (autocompletar el alta del talento) también se cuenta en el consumo por tenant.
ALTER TABLE tenant_usage_daily DROP CHECK ck_usage_kind;
ALTER TABLE tenant_usage_daily ADD CONSTRAINT ck_usage_kind CHECK (kind IN ('DNI_LOOKUP','AI_TRANSLATE','AI_CV_READ'));
