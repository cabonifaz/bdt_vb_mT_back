-- "Recordarme": la sesión recordada dura más y sobrevive al cierre del navegador.
ALTER TABLE user_session ADD COLUMN remember TINYINT(1) NOT NULL DEFAULT 0 AFTER family_id;
