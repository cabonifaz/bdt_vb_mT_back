#!/usr/bin/env bash
# Prueba de humo de notificaciones (centro en la app y suscripción push).
# Uso: SA_EMAIL=... SA_PASSWORD=... ./scripts/smoke_notifications.sh [http://localhost:8080]
set -u
API=${1:-http://localhost:8080}
J="Content-Type: application/json"
pass=0; fail=0
check() { if [ "$2" = "$3" ]; then echo "  OK   $1"; pass=$((pass+1)); else echo "  FAIL $1 (esperado $3, obtuvo $2) $(head -c 200 /tmp/st_body)"; fail=$((fail+1)); fi; }
code() { curl -s -o /tmp/st_body -w '%{http_code}' "$@"; }
body() { cat /tmp/st_body; }
jget() { node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const o=JSON.parse(d);const v=eval('o'+process.argv[1]);console.log(typeof v==='object'?JSON.stringify(v):(v??''))})" "$1"; }
srch() { code -X POST -H "$1" -H "$J" -d "$2" $API/api/v1/tenant/talents/search; }
uuid() { node -e "console.log(crypto.randomUUID())"; }

# --- preparar: admin y reclutador de Fractal (invitados por el SA)
code -H "$J" -d "{\"email\":\"$SA_EMAIL\",\"password\":\"$SA_PASSWORD\"}" $API/api/v1/auth/login >/dev/null; SA=$(body | jget .accessToken)
code -H "Authorization: Bearer $SA" $API/api/v1/platform/tenants >/dev/null; FRACTAL=$(body | jget '.items.find(t=>t.slug==="fractal").id')
mkuser() {
  local email="$1.$RANDOM$RANDOM@fractal.test" role=$2 pw="Talent-$RANDOM-Seg!x"
  code -H "Authorization: Bearer $SA" -H "$J" -d "{\"email\":\"$email\",\"role\":\"$role\"}" "$API/api/v1/platform/invitations?tenantId=$FRACTAL" >/dev/null
  local tok=$(body | jget .link | sed 's#.*/invitacion/##')
  code -H "$J" -d "{\"fullName\":\"Smoke $role\",\"password\":\"$pw\",\"locale\":\"es\"}" $API/api/v1/invitations/$tok/accept >/dev/null
  code -H "$J" -d "{\"tenant\":\"fractal\",\"email\":\"$email\",\"password\":\"$pw\"}" $API/api/v1/auth/login >/dev/null
  body | jget .accessToken
}
AD=$(mkuser admin TENANT_ADMIN); RC=$(mkuser reclu RECRUITER)
A="Authorization: Bearer $AD"; R="Authorization: Bearer $RC"
N=$API/api/v1/tenant/notifications
unread() { code -H "$1" "$N?limit=50" >/dev/null; body | jget .unread; }
item() { code -H "$1" "$N?limit=50" >/dev/null; body | jget ".items.find(i=>i.entityId==='$2')?$3"; }

echo "== Alta de talento: notifica a los demás usuarios del tenant, no al autor"
U0=$(unread "$R")
T=$(uuid)
TAL="{\"id\":\"$T\",\"firstName\":\"Noti\",\"lastName\":\"Prueba$RANDOM\",\"headline\":\"QA Analyst\"}"
check "admin crea talento" "$(code -X POST -H "$A" -H "$J" -d "$TAL" $API/api/v1/tenant/talents)" 201
check "reclutador recibe TALENT_CREATED" "$(item "$R" $T .type)" TALENT_CREATED
check "con el nombre del autor" "$(item "$R" $T .actor.name)" "Smoke TENANT_ADMIN"
check "y el puesto del talento" "$(item "$R" $T .payload.headline)" "QA Analyst"
check "no leída" "$(item "$R" $T ".readAt===undefined")" true
check "el autor no se notifica a sí mismo" "$(item "$A" $T .type)" ""

echo "== Ediciones seguidas del mismo autor se agrupan"
check "admin edita" "$(code -X PUT -H "$A" -H "$J" -d "{\"id\":\"$T\",\"firstName\":\"Noti\",\"lastName\":\"Editada\",\"headline\":\"QA Lead\"}" $API/api/v1/tenant/talents/$T)" 200
check "sigue siendo una sola notificación" "$(code -H "$R" "$N?limit=50" >/dev/null; body | jget ".items.filter(i=>i.entityId==='$T').length")" 1
check "con 2 cambios" "$(item "$R" $T .count)" 2
check "y el dato actualizado" "$(item "$R" $T .payload.headline)" "QA Lead"

echo "== Lectura"
NID=$(item "$R" $T .id)
check "marcar una como leída" "$(code -X POST -H "$R" -H "$J" -d "{\"id\":\"$NID\"}" $N/read)" 204
check "queda leída" "$(item "$R" $T ".readAt!==undefined")" true
check "solo no leídas no la incluye" "$(code -H "$R" "$N?unread=1&limit=50" >/dev/null; body | jget ".items.some(i=>i.id==='$NID')")" false
check "marcar todas" "$(code -X POST -H "$R" -H "$J" -d '{}' $N/read)" 204
check "sin no leídas" "$(unread "$R")" 0
check "id inválido" "$(code -X POST -H "$R" -H "$J" -d '{"id":"x"}' $N/read)" 422

echo "== Preferencias"
check "preferencias por defecto (todo activo)" "$(code -H "$R" $N/prefs >/dev/null; body | jget ".types.every(t=>t.inApp&&t.push)")" true
check "tipo desconocido" "$(code -X PUT -H "$R" -H "$J" -d '{"type":"X","inApp":true,"push":true}' $N/prefs)" 422
check "silenciar altas en la app" "$(code -X PUT -H "$R" -H "$J" -d '{"type":"TALENT_CREATED","inApp":false,"push":false}' $N/prefs)" 204
T2=$(uuid)
code -X POST -H "$A" -H "$J" -d "{\"id\":\"$T2\",\"firstName\":\"Silencio\",\"lastName\":\"Prueba$RANDOM\"}" $API/api/v1/tenant/talents >/dev/null
check "silenciado: no la recibe" "$(item "$R" $T2 .type)" ""
check "reactivar" "$(code -X PUT -H "$R" -H "$J" -d '{"type":"TALENT_CREATED","inApp":true,"push":true}' $N/prefs)" 204

echo "== Push"
check "clave pública VAPID" "$(code -H "$R" $N/push/key >/dev/null; body | jget ".enabled && .publicKey.length>40" 2>/dev/null || body | jget '.enabled')" true
SUB='{"endpoint":"https://push.example.test/wpush/'$RANDOM'","keys":{"p256dh":"BNcRdreALRFXTkOOUHK1EtK2wtaz5Ry4YfYCA_0QTpQtUbVlUls0VJXg7A8u-Ts1XbjhazAkj7I99e8QcYP7DkM","auth":"tBHItJI5svbpez7KI4CCXg"}}'
check "endpoint http rechazado" "$(code -X POST -H "$R" -H "$J" -d '{"endpoint":"http://push.example.test/x","keys":{"p256dh":"a","auth":"b"}}' $N/push/subscribe)" 422
check "endpoint interno rechazado" "$(code -X POST -H "$R" -H "$J" -d '{"endpoint":"https://localhost/x","keys":{"p256dh":"a","auth":"b"}}' $N/push/subscribe)" 422
check "suscribir dispositivo" "$(code -X POST -H "$R" -H "$J" -d "$SUB" $N/push/subscribe)" 200
check "1 dispositivo" "$(code -H "$R" $N/prefs >/dev/null; body | jget .devices)" 1
check "desuscribir" "$(code -X POST -H "$R" -H "$J" -d "$SUB" $N/push/unsubscribe)" 422
check "desuscribir (solo endpoint)" "$(code -X POST -H "$R" -H "$J" -d "{\"endpoint\":$(echo "$SUB" | jget .endpoint | node -e 'let d="";process.stdin.on("data",c=>d+=c).on("end",()=>console.log(JSON.stringify(d.trim())))')}" $N/push/unsubscribe)" 204
check "0 dispositivos" "$(code -H "$R" $N/prefs >/dev/null; body | jget .devices)" 0

echo "== Seguridad"
check "sin sesión" "$(code $N)" 401
check "Super Admin no tiene bandeja de tenant" "$(code -H "Authorization: Bearer $SA" $N)" 403
echo "== Resultado: $pass OK, $fail FAIL"
[ $fail -eq 0 ]
