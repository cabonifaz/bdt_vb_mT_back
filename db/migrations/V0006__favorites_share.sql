-- Colecciones de favoritos compartidas con otros usuarios del mismo tenant.
-- permission: VIEW (solo ver) | EDIT (agregar/quitar talentos). Solo el dueño comparte, renombra o elimina.
CREATE TABLE favorite_collection_share (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  collection_id BIGINT UNSIGNED NOT NULL,
  user_id       BIGINT UNSIGNED NOT NULL,
  permission    VARCHAR(10)  NOT NULL DEFAULT 'VIEW',
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by    BIGINT UNSIGNED NULL,
  updated_at    DATETIME(3)  NULL ON UPDATE CURRENT_TIMESTAMP(3),
  updated_by    BIGINT UNSIGNED NULL,
  UNIQUE KEY uq_fcs (collection_id, user_id),
  KEY ix_fcs_user (user_id),
  CONSTRAINT fk_fcs_coll FOREIGN KEY (collection_id) REFERENCES favorite_collection(id),
  CONSTRAINT fk_fcs_user FOREIGN KEY (user_id) REFERENCES app_user(id),
  CONSTRAINT ck_fcs_perm CHECK (permission IN ('VIEW','EDIT'))
) ENGINE=InnoDB;
