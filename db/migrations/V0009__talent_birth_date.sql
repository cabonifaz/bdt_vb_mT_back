-- Fecha de nacimiento del talento (dato personal: cifrada con AES-GCM como el documento y el teléfono)
ALTER TABLE talent ADD COLUMN birth_date_enc VARBINARY(256) NULL AFTER phone_enc;
