# Banco de Talentos MT · Backend

API en Go, multi-tenant, con arquitectura hexagonal. Toda la lógica de datos vive en procedimientos almacenados de MySQL.

## Estructura

```
cmd/api          servidor HTTP
cmd/migrate      aplica db/ con el usuario migrator
cmd/bootstrap    crea el primer Super Admin (solo si no existe)
db/migrations    V*__*.sql  versionadas (se aplican una vez)
db/procedures    R__*.sql   repetibles (procedimientos; se reaplican si cambian)
internal/domain  entidades y errores de negocio (sin dependencias)
internal/app     casos de uso + ports/ (interfaces)
internal/adapters
  mysql          repositorios: solo ejecutan CALL sp_*
  httpapi        REST, middleware de seguridad, API de integración (OpenAPI)
  security       AES-256-GCM, índice ciego HMAC, argon2id, JWT
  storage        archivos en volumen persistente (STORAGE_PATH)
  mailer         envío de invitaciones
internal/platform config, wiring y utilidades
```

## Reglas

- La app conecta con un usuario MySQL que **solo tiene EXECUTE**: no puede leer ni escribir tablas directamente.
- Los datos sensibles (correo, nombre, teléfono) se guardan cifrados con AES-256-GCM. Las búsquedas usan un índice ciego HMAC.
- Los errores de negocio salen de los SP como `SIGNAL 'BDT:<CODIGO>'` y llegan al cliente como `{error:{code}}`. La UI los traduce.
- Ningún secreto va en el repo: todo se configura por variables de entorno (ver `.env.example`).

## Local

```bash
cp .env.example .env    # completar
go run ./cmd/migrate
BOOTSTRAP_EMAIL=... BOOTSTRAP_NAME=... BOOTSTRAP_PASSWORD=... go run ./cmd/bootstrap
go run ./cmd/api
SA_EMAIL=... SA_PASSWORD=... ./scripts/smoke.sh
```

## API

- `/api/v1/public/*`: branding y archivos públicos del tenant.
- `/api/v1/auth/*`: login, refresh y logout. El refresh token se guarda en una cookie HttpOnly y rota en cada uso; si se reutiliza, se revoca toda la familia.
- `/api/v1/tenant/*`: administración del tenant (branding, usuarios, invitaciones, maestros, API keys, auditoría).
- `/api/v1/platform/*`: Super Admin (tenants, habilitación de API keys, maestros globales).
- `/integration/docs`: documentación tipo Swagger. Pide una API key habilitada.
- `/integration/v1/*`: API pública para sistemas externos (cabecera `X-Api-Key`).
