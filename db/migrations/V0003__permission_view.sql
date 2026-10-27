-- Permisos efectivos por usuario activo (reemplaza funciones: el servidor
-- tiene binlog activo y no se permite CREATE FUNCTION sin SUPER).
CREATE OR REPLACE SQL SECURITY INVOKER VIEW v_user_permission AS
SELECT DISTINCT ur.user_id, u.tenant_id, p.code
  FROM user_role ur
  JOIN role_permission rp ON rp.role_id = ur.role_id
  JOIN permission p ON p.id = rp.permission_id AND p.deleted_at IS NULL
  JOIN app_user u ON u.id = ur.user_id AND u.deleted_at IS NULL AND u.status = 'ACTIVE';
