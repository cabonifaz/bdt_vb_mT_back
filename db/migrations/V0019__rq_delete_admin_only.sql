-- Eliminar un requerimiento: solo el administrador del tenant (antes bastaba rq.write).
-- Jefe de selección y comercial siguen creando, editando y cerrando RQ, pero no los eliminan.
INSERT INTO permission (code, description) VALUES ('rq.delete', 'Eliminar requerimientos');
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code = 'rq.delete' WHERE r.code = 'TENANT_ADMIN';
