#!/usr/bin/env bash
# Prueba de humo del núcleo contra una API en ejecución.
# Uso: SA_EMAIL=... SA_PASSWORD=... ./scripts/smoke.sh [http://localhost:8080]
set -u
API=${1:-http://localhost:8080}
O="Origin: http://localhost:5173"
J="Content-Type: application/json"
pass=0; fail=0
check() { if [ "$2" = "$3" ]; then echo "  OK   $1"; pass=$((pass+1)); else echo "  FAIL $1 (esperado $3, obtuvo $2)"; fail=$((fail+1)); fi; }
code() { curl -s -o /tmp/smoke_body -w '%{http_code}' "$@"; }
body() { cat /tmp/smoke_body; }
jget() { node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const o=JSON.parse(d);console.log(eval('o'+process.argv[1])??'')})" "$1"; }

echo "== Público"
check "health" "$(code $API/health)" 200
check "branding fractal" "$(code $API/api/v1/public/tenants/fractal)" 200
check "tenant inexistente" "$(code $API/api/v1/public/tenants/no-existe)" 404

echo "== Super Admin"
check "login SA clave errónea" "$(code -H "$J" -d "{\"email\":\"$SA_EMAIL\",\"password\":\"mala-clave-123\"}" $API/api/v1/auth/login)" 401
check "login SA" "$(code -c /tmp/sa.jar -H "$J" -d "{\"email\":\"$SA_EMAIL\",\"password\":\"$SA_PASSWORD\"}" $API/api/v1/auth/login)" 200
SA=$(body | jget .accessToken)
check "SA lista tenants" "$(code -H "Authorization: Bearer $SA" $API/api/v1/platform/tenants)" 200
FRACTAL=$(body | jget '.items.find(t=>t.slug==="fractal").id')
ADMIN_EMAIL="admin.e2e.$RANDOM@fractal.test"
check "SA invita admin de Fractal" "$(code -H "Authorization: Bearer $SA" -H "$J" -d "{\"email\":\"$ADMIN_EMAIL\",\"role\":\"TENANT_ADMIN\",\"locale\":\"es\"}" "$API/api/v1/platform/invitations?tenantId=$FRACTAL")" 201
TOKEN=$(body | jget .link | sed 's#.*/invitacion/##')
check "info invitación" "$(code $API/api/v1/invitations/$TOKEN)" 200
check "aceptar con clave débil" "$(code -H "$J" -d '{"fullName":"Admin E2E","password":"debil","locale":"es"}' $API/api/v1/invitations/$TOKEN/accept)" 422
ADMIN_PW="Fractal-E2E-$RANDOM-Seg!"
check "aceptar invitación" "$(code -H "$J" -d "{\"fullName\":\"Admin E2E\",\"password\":\"$ADMIN_PW\",\"locale\":\"es\"}" $API/api/v1/invitations/$TOKEN/accept)" 204
check "reusar invitación" "$(code -H "$J" -d "{\"fullName\":\"X\",\"password\":\"$ADMIN_PW\",\"locale\":\"es\"}" $API/api/v1/invitations/$TOKEN/accept)" 409

echo "== Admin del tenant"
check "login admin en otro tenant" "$(code -H "$J" -d "{\"tenant\":\"otro\",\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PW\"}" $API/api/v1/auth/login)" 401
check "login admin Fractal (recordarme)" "$(code -c /tmp/ad.jar -D /tmp/ad.hdr -H "$J" -d "{\"tenant\":\"fractal\",\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PW\",\"remember\":true}" $API/api/v1/auth/login)" 200
check "cookie persistente con recordarme" "$(grep -i "set-cookie: bdt_rt" /tmp/ad.hdr | grep -ci expires)" 1
AD=$(body | jget .accessToken)
check "admin NO accede a plataforma" "$(code -H "Authorization: Bearer $AD" $API/api/v1/platform/tenants)" 403
check "admin lista usuarios" "$(code -H "Authorization: Bearer $AD" $API/api/v1/tenant/users)" 200
check "color inválido rechazado" "$(code -X PUT -H "Authorization: Bearer $AD" -H "$J" -d '{"displayName":"Fractal","colorPrimary":"red","colorPrimaryHover":"#2D8D8D","colorSecondary":"#0B85C3","colorAccent":"#FAAB34","colorSidebar":"#FFFFFF","fontFamily":"Inter"}' $API/api/v1/tenant/branding)" 422
check "maestros visibles" "$(code -H "Authorization: Bearer $AD" "$API/api/v1/masters/items?tables=LANGUAGE,CURRENCY")" 200
check "tenant NO edita maestro global" "$(code -X PUT -H "Authorization: Bearer $AD" -H "$J" -d '{"table":"LANGUAGE","code":"IT","label":{"es":"Italiano"}}' $API/api/v1/tenant/masters/items)" 403
check "SA ve consumo por tenant" "$(code -H "Authorization: Bearer $SA" "$API/api/v1/platform/usage?from=2026-01-01&to=2026-12-31")" 200
check "consumo: rango inválido" "$(code -H "Authorization: Bearer $SA" "$API/api/v1/platform/usage?from=2026-12-31&to=2026-01-01")" 422
check "admin NO ve consumo de plataforma" "$(code -H "Authorization: Bearer $AD" "$API/api/v1/platform/usage?from=2026-01-01&to=2026-12-31")" 403
check "leer CV sin texto (escaneo)" "$(code -H "Authorization: Bearer $AD" -H "$J" -d '{"text":"escaneo"}' $API/api/v1/tenant/talents/cv/read)" 422
check "tenant crea skill propia" "$(code -X PUT -H "Authorization: Bearer $AD" -H "$J" -d "{\"table\":\"TECH_SKILL\",\"code\":\"GO$RANDOM\",\"label\":{\"es\":\"Go\",\"en\":\"Go\"}}" $API/api/v1/tenant/masters/items)" 200

echo "== API keys"
check "admin solicita key" "$(code -H "Authorization: Bearer $AD" -H "$J" -d '{"name":"ERP e2e","scopes":["masters.read"]}' $API/api/v1/tenant/api-keys)" 201
KEY=$(body | jget .secret); KID=$(body | jget .id)
check "key PENDING no entra" "$(code -H "X-Api-Key: $KEY" $API/integration/v1/whoami)" 403
check "admin NO puede habilitar" "$(code -X PATCH -H "Authorization: Bearer $AD" -H "$J" -d '{"status":"ENABLED"}' $API/api/v1/platform/api-keys/$KID/status)" 403
check "SA habilita key" "$(code -X PATCH -H "Authorization: Bearer $SA" -H "$J" -d '{"status":"ENABLED"}' $API/api/v1/platform/api-keys/$KID/status)" 200
check "key habilitada: whoami" "$(code -H "X-Api-Key: $KEY" $API/integration/v1/whoami)" 200
check "key: maestros" "$(code -H "X-Api-Key: $KEY" "$API/integration/v1/masters?tables=LANGUAGE")" 200
check "key: openapi" "$(code -H "X-Api-Key: $KEY" $API/integration/v1/openapi.json)" 200
check "sin key: openapi" "$(code $API/integration/v1/openapi.json)" 401
check "key alterada" "$(code -H "X-Api-Key: ${KEY}x" $API/integration/v1/whoami)" 401
check "admin revoca key" "$(code -X POST -H "Authorization: Bearer $AD" $API/api/v1/tenant/api-keys/$KID/revoke)" 200
check "key revocada no entra" "$(code -H "X-Api-Key: $KEY" $API/integration/v1/whoami)" 403

echo "== Sesión"
check "refresh sin Origin (CSRF)" "$(code -X POST -b /tmp/ad.jar $API/api/v1/auth/refresh)" 403
check "refresh" "$(code -X POST -b /tmp/ad.jar -c /tmp/ad2.jar -H "$O" $API/api/v1/auth/refresh)" 200
check "reuso inmediato tolerado (2 pestañas / recarga)" "$(code -X POST -b /tmp/ad.jar -H "$O" $API/api/v1/auth/refresh)" 200
sleep ${GRACE_WAIT:-4}
check "reuso fuera de tolerancia revoca familia" "$(code -X POST -b /tmp/ad.jar -H "$O" $API/api/v1/auth/refresh)" 401
check "refresh nuevo también revocado" "$(code -X POST -b /tmp/ad2.jar -H "$O" $API/api/v1/auth/refresh)" 401
check "token manipulado" "$(code -H "Authorization: Bearer ${AD}x" $API/api/v1/auth/me)" 401
check "alg none" "$(code -H "Authorization: Bearer eyJhbGciOiJub25lIn0.eyJzdWIiOiJ4In0." $API/api/v1/auth/me)" 401

echo "== Bloqueo por intentos"
for i in 1 2 3 4 5; do code -H "$J" -d "{\"tenant\":\"fractal\",\"email\":\"$ADMIN_EMAIL\",\"password\":\"Mala-$i-clave\"}" $API/api/v1/auth/login >/dev/null; done
check "cuenta bloqueada tras 5 fallos" "$(code -H "$J" -d "{\"tenant\":\"fractal\",\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PW\"}" $API/api/v1/auth/login)" 403
echo "== Resultado: $pass OK, $fail FAIL"
[ $fail -eq 0 ]
