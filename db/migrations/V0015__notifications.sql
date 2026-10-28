-- =====================================================================
-- Notificaciones del tenant: centro de notificaciones en la app y push del sistema operativo (Web Push, VAPID)
-- para la app instalada. Tipos actuales: TALENT_CREATED, TALENT_UPDATED; luego RQ_CREATED, RQ_UPDATED.
-- Una notificación por evento + una fila por destinatario (estado de lectura propio).
-- Las actualizaciones seguidas del mismo talento por el mismo usuario se agrupan (no se repiten notificaciones).
-- =====================================================================

CREATE TABLE notification (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  uuid          CHAR(36)        NOT NULL,
  tenant_id     BIGINT UNSIGNED NOT NULL,
  type          VARCHAR(40)     NOT NULL,          -- TALENT_CREATED | TALENT_UPDATED | ...
  entity_type   VARCHAR(30)     NOT NULL,          -- TALENT | REQUIREMENT
  entity_uuid   CHAR(36)        NOT NULL,
  actor_user_id BIGINT UNSIGNED NULL,
  payload       JSON            NOT NULL,          -- datos para el texto (nombre, puesto...)
  event_count   INT UNSIGNED    NOT NULL DEFAULT 1, -- cambios agrupados en esta notificación
  created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uq_notification_uuid (uuid),
  KEY ix_notification_group (tenant_id, entity_uuid, type, actor_user_id, updated_at),
  CONSTRAINT fk_notification_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id),
  CONSTRAINT fk_notification_actor FOREIGN KEY (actor_user_id) REFERENCES app_user(id)
) ENGINE=InnoDB;

CREATE TABLE notification_recipient (
  notification_id BIGINT UNSIGNED NOT NULL,
  user_id         BIGINT UNSIGNED NOT NULL,
  read_at         DATETIME(3)     NULL,
  delivered_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3), -- última vez que se (re)notificó
  PRIMARY KEY (notification_id, user_id),
  KEY ix_recipient_inbox (user_id, delivered_at),
  KEY ix_recipient_unread (user_id, read_at),
  CONSTRAINT fk_recipient_notification FOREIGN KEY (notification_id) REFERENCES notification(id) ON DELETE CASCADE,
  CONSTRAINT fk_recipient_user FOREIGN KEY (user_id) REFERENCES app_user(id)
) ENGINE=InnoDB;

-- Suscripciones push por dispositivo/navegador (endpoint del servicio push del navegador)
CREATE TABLE push_subscription (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id       BIGINT UNSIGNED NOT NULL,
  tenant_id     BIGINT UNSIGNED NOT NULL,
  endpoint_hash CHAR(64)        NOT NULL,
  endpoint      VARCHAR(1000)   NOT NULL,
  p256dh        VARCHAR(200)    NOT NULL,
  auth          VARCHAR(100)    NOT NULL,
  user_agent    VARCHAR(300)    NULL,
  created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_ok_at    DATETIME(3)     NULL,
  failures      INT UNSIGNED    NOT NULL DEFAULT 0,
  UNIQUE KEY uq_push_endpoint (endpoint_hash),
  KEY ix_push_user (user_id),
  CONSTRAINT fk_push_user FOREIGN KEY (user_id) REFERENCES app_user(id),
  CONSTRAINT fk_push_tenant FOREIGN KEY (tenant_id) REFERENCES tenant(id)
) ENGINE=InnoDB;

-- Preferencias por usuario y tipo (sin fila = recibe en la app y por push)
CREATE TABLE notification_pref (
  user_id    BIGINT UNSIGNED NOT NULL,
  type       VARCHAR(40)     NOT NULL,
  in_app     TINYINT(1)      NOT NULL DEFAULT 1,
  push       TINYINT(1)      NOT NULL DEFAULT 1,
  updated_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (user_id, type),
  CONSTRAINT fk_npref_user FOREIGN KEY (user_id) REFERENCES app_user(id)
) ENGINE=InnoDB;
