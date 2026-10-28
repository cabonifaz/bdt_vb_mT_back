-- Formato del CV adoptado desde un CV de ejemplo (PDF) que sube el tenant: alineación y colores del nombre y títulos,
-- línea bajo títulos, recuadro del pie, tipografía. JSON validado por la aplicación; NULL = formato por defecto.
ALTER TABLE tenant_branding ADD COLUMN cv_style JSON NULL AFTER cv_show_photo;
