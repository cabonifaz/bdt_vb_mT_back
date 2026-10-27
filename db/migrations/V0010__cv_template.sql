-- Plantilla del CV con la marca del tenant (como el "CV Fractal" del sistema heredado, pero configurable por tenant):
-- imagen de cabecera y de pie (opcional), texto del pie y si se muestra la foto del talento.
ALTER TABLE tenant_branding
  ADD COLUMN cv_header_file_id BIGINT UNSIGNED NULL AFTER favicon_file_id,
  ADD COLUMN cv_footer_file_id BIGINT UNSIGNED NULL AFTER cv_header_file_id,
  ADD COLUMN cv_footer_text    VARCHAR(300) NULL AFTER cv_footer_file_id,
  ADD COLUMN cv_show_photo     TINYINT(1) NOT NULL DEFAULT 0 AFTER cv_footer_text,
  ADD CONSTRAINT fk_branding_cv_header FOREIGN KEY (cv_header_file_id) REFERENCES file_object(id),
  ADD CONSTRAINT fk_branding_cv_footer FOREIGN KEY (cv_footer_file_id) REFERENCES file_object(id);

INSERT INTO permission (code, description) VALUES ('talent.cv.generate', 'Generar el CV del talento con la marca del tenant');
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code = 'talent.cv.generate'
 WHERE r.code IN ('TENANT_ADMIN','RECRUITER','COMMERCIAL');
