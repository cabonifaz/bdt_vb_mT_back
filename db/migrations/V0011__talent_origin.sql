-- Procedencia del talento (país y ciudad de origen); la residencia sigue en country_item_id / city
ALTER TABLE talent
  ADD COLUMN origin_country_item_id BIGINT UNSIGNED NULL AFTER city,
  ADD COLUMN origin_city VARCHAR(120) NULL AFTER origin_country_item_id,
  ADD CONSTRAINT fk_talent_origin_country FOREIGN KEY (origin_country_item_id) REFERENCES master_item(id);
