-- =====================================================================
-- Soporte, novedades y guías
-- - Notificaciones de plataforma: tenant_id NULL (al Super Admin, o novedades para todos los tenants).
-- - support_ticket: observación, incidencia o mejora que un usuario envía al Super Admin, con el origen
--   (pantalla desde donde se pidió), referencias, imágenes y una conversación (support_message).
-- - release_note: novedad publicada por el Super Admin; se notifica a todos los usuarios.
-- - user_tour: guías de primer uso vistas por cada usuario (una por pantalla y versión).
-- =====================================================================

ALTER TABLE notification MODIFY tenant_id BIGINT UNSIGNED NULL;
ALTER TABLE push_subscription MODIFY tenant_id BIGINT UNSIGNED NULL;

INSERT INTO permission (code, description) VALUES
 ('platform.support.manage',  'Atender y responder las solicitudes de soporte de los usuarios'),
 ('platform.releases.manage', 'Publicar novedades de la plataforma');
INSERT INTO role_permission (role_id, permission_id)
SELECT r.id, p.id FROM role r JOIN permission p ON p.code IN ('platform.support.manage','platform.releases.manage')
 WHERE r.code = 'SUPER_ADMIN';

CREATE TABLE support_ticket (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid             CHAR(36)        NOT NULL,
  number           INT UNSIGNED    NOT NULL,          -- correlativo de la plataforma (#123)
  tenant_id        BIGINT UNSIGNED NOT NULL,
  reporter_user_id BIGINT UNSIGNED NOT NULL,
  kind             VARCHAR(15)     NOT NULL,          -- INCIDENT | OBSERVATION | IMPROVEMENT
  impact           VARCHAR(10)     NOT NULL DEFAULT 'MEDIUM', -- LOW | MEDIUM | HIGH
  title            VARCHAR(200)    NOT NULL,
  status           VARCHAR(15)     NOT NULL DEFAULT 'OPEN',   -- OPEN | IN_PROGRESS | ANSWERED | CLOSED
  origin           JSON            NOT NULL,          -- pantalla, ruta, módulo, entidad, navegador, versión
  refs             JSON            NULL,              -- enlaces o referencias
  created_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at       DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_staff_at    DATETIME(3)     NULL,
  closed_at        DATETIME(3)     NULL,
  UNIQUE KEY uq_ticket_uuid (uuid),
  UNIQUE KEY uq_ticket_number (number),
  KEY ix_ticket_reporter (reporter_user_id, updated_at),
  KEY ix_ticket_status (status, updated_at),
  CONSTRAINT ck_ticket_kind CHECK (kind IN ('INCIDENT','OBSERVATION','IMPROVEMENT')),
  CONSTRAINT ck_ticket_impact CHECK (impact IN ('LOW','MEDIUM','HIGH')),
  CONSTRAINT ck_ticket_status CHECK (status IN ('OPEN','IN_PROGRESS','ANSWERED','CLOSED')),
  CONSTRAINT fk_ticket_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_ticket_reporter FOREIGN KEY (reporter_user_id) REFERENCES app_user(id)
) ENGINE=InnoDB;

CREATE TABLE support_message (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid        CHAR(36)        NOT NULL,
  ticket_id   BIGINT UNSIGNED NOT NULL,
  author_id   BIGINT UNSIGNED NOT NULL,
  is_staff    TINYINT(1)      NOT NULL DEFAULT 0,
  body        TEXT            NOT NULL,
  status_to   VARCHAR(15)     NULL,                   -- cambio de estado hecho con este mensaje
  created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uq_support_msg_uuid (uuid),
  KEY ix_support_msg_ticket (ticket_id, created_at),
  CONSTRAINT fk_support_msg_ticket FOREIGN KEY (ticket_id) REFERENCES support_ticket(id),
  CONSTRAINT fk_support_msg_author FOREIGN KEY (author_id) REFERENCES app_user(id)
) ENGINE=InnoDB;

-- Imágenes y archivos de referencia (file_object cifrado, owner_type SUPPORT)
CREATE TABLE support_attachment (
  ticket_id  BIGINT UNSIGNED NOT NULL,
  message_id BIGINT UNSIGNED NOT NULL,
  file_id    BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (message_id, file_id),
  KEY ix_support_att_ticket (ticket_id),
  CONSTRAINT fk_support_att_ticket FOREIGN KEY (ticket_id) REFERENCES support_ticket(id),
  CONSTRAINT fk_support_att_msg FOREIGN KEY (message_id) REFERENCES support_message(id),
  CONSTRAINT fk_support_att_file FOREIGN KEY (file_id) REFERENCES file_object(id)
) ENGINE=InnoDB;

CREATE TABLE release_note (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid         CHAR(36)        NOT NULL,
  title_i18n   JSON            NOT NULL,
  body_i18n    JSON            NOT NULL,
  module       VARCHAR(40)     NULL,                 -- pantalla relacionada (para abrirla y su guía)
  tour_key     VARCHAR(40)     NULL,                 -- guía que se vuelve a mostrar con la novedad
  published_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  created_by   BIGINT UNSIGNED NOT NULL,
  UNIQUE KEY uq_release_uuid (uuid),
  KEY ix_release_published (published_at),
  CONSTRAINT fk_release_author FOREIGN KEY (created_by) REFERENCES app_user(id)
) ENGINE=InnoDB;

CREATE TABLE user_tour (
  user_id    BIGINT UNSIGNED NOT NULL,
  tour_key   VARCHAR(40)     NOT NULL,
  version    INT             NOT NULL,
  seen_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (user_id, tour_key),
  CONSTRAINT fk_user_tour_user FOREIGN KEY (user_id) REFERENCES app_user(id)
) ENGINE=InnoDB;
