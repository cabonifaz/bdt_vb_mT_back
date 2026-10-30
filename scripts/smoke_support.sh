#!/usr/bin/env bash
# Prueba de humo de soporte, novedades y guías: un usuario reporta (con imagen y origen), el Super Admin lo ve y
# responde, cada lado recibe su notificación; novedades para todos; guías vistas por usuario.
# Uso: SA_EMAIL=... SA_PASSWORD=... ./scripts/smoke_support.sh [http://localhost:8080]
set -u
API=${1:-http://localhost:8080}
J="Content-Type: application/json"
pass=0; fail=0
check() { if [ "$2" = "$3" ]; then echo "  OK   $1"; pass=$((pass+1)); else echo "  FAIL $1 (esperado $3, obtuvo $2) $(head -c 200 /tmp/ss_body)"; fail=$((fail+1)); fi; }
code() { curl -s -o /tmp/ss_body -w '%{http_code}' "$@"; }
body() { cat /tmp/ss_body; }
jget() { node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const o=JSON.parse(d);const v=eval('o'+process.argv[1]);console.log(typeof v==='object'?JSON.stringify(v):(v??''))})" "$1"; }
js() { node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const o=JSON.parse(d);const v=(new Function('o','return '+process.argv[1]))(o);console.log(typeof v==='object'?JSON.stringify(v):String(v))})" "$1"; }
req() { code -X "$1" -H "$3" -H "$J" -d "$4" "$API$2"; }
inbox() { code -H "$1" "$API/api/v1/$2/notifications?limit=50" >/dev/null; body; }

code -H "$J" -d "{\"email\":\"$SA_EMAIL\",\"password\":\"$SA_PASSWORD\"}" $API/api/v1/auth/login >/dev/null; SAT=$(body | jget .accessToken); S="Authorization: Bearer $SAT"
code -H "$S" $API/api/v1/platform/tenants >/dev/null; FRACTAL=$(body | jget '.items.find(t=>t.slug==="fractal").id')
mkuser() {
  local email="$1.$RANDOM$RANDOM@fractal.test" role=$2 pw="Talent-$RANDOM-Seg!x"
  code -H "$S" -H "$J" -d "{\"email\":\"$email\",\"role\":\"$role\"}" "$API/api/v1/platform/invitations?tenantId=$FRACTAL" >/dev/null
  local tok=$(body | jget .link | sed 's#.*/invitacion/##')
  code -H "$J" -d "{\"fullName\":\"Smoke $role\",\"password\":\"$pw\",\"locale\":\"es\"}" $API/api/v1/invitations/$tok/accept >/dev/null
  code -H "$J" -d "{\"tenant\":\"fractal\",\"email\":\"$email\",\"password\":\"$pw\"}" $API/api/v1/auth/login >/dev/null
  body | jget .accessToken
}
RC=$(mkuser reclu RECRUITER); OT=$(mkuser otro RECRUITER)
R="Authorization: Bearer $RC"; O="Authorization: Bearer $OT"
# imagen PNG de 1x1
IMG=/tmp/ss_img.png; IMGW=$(cygpath -w $IMG 2>/dev/null || echo $IMG); node -e "require('fs').writeFileSync(process.argv[1],Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==','base64'))" "$IMGW"

echo "== Solicitud del usuario"
check "sube una imagen" "$(code -X POST -H "$R" -F "file=@$IMGW;type=image/png" $API/api/v1/support/files)" 201
F1=$(body | jget .id)
check "un ejecutable no se acepta" "$(printf 'MZ\x90\x00binario' > /tmp/ss_bad.bin; code -X POST -H "$R" -F "file=@$(cygpath -w /tmp/ss_bad.bin 2>/dev/null || echo /tmp/ss_bad.bin)" $API/api/v1/support/files)" 422
ORIGIN='{"module":"requirements","screen":"Tablero de requerimientos","path":"/fractal/requerimientos","entity":null,"app":"web","viewport":"1440x900"}'
check "faltan datos" "$(req POST /api/v1/support "$R" "{\"kind\":\"INCIDENT\",\"title\":\"\",\"body\":\"x\",\"origin\":$ORIGIN}")" 422
check "tipo inválido" "$(req POST /api/v1/support "$R" "{\"kind\":\"OTRO\",\"title\":\"x\",\"body\":\"x\",\"origin\":$ORIGIN}")" 422
check "registra la incidencia" "$(req POST /api/v1/support "$R" "{\"kind\":\"INCIDENT\",\"impact\":\"HIGH\",\"title\":\"No guarda el RQ\",\"body\":\"Al guardar sale un error.\",\"origin\":$ORIGIN,\"refs\":[\"https://ejemplo.test/captura\",\"RQ-2026-0001\"],\"files\":[\"$F1\"]}")" 201
TK=$(body | jget .id); NUM=$(body | jget .number)
check "con número correlativo" "$([ "$NUM" -gt 0 ] && echo si)" si
check "el mismo archivo no se adjunta dos veces" "$(req POST /api/v1/support "$R" "{\"kind\":\"OBSERVATION\",\"title\":\"x\",\"body\":\"x\",\"origin\":$ORIGIN,\"files\":[\"$F1\"]}")" 422
check "aparece en sus solicitudes" "$(code -H "$R" $API/api/v1/support >/dev/null; body | js "o.some(t=>t.id==='$TK'&&t.status==='OPEN'&&t.attachments===1)")" true
check "otro usuario no la ve" "$(code -H "$O" $API/api/v1/support/$TK)" 404
check "ni su imagen" "$(code -H "$O" $API/api/v1/support/files/$F1)" 404
check "el autor ve el origen y las referencias" "$(code -H "$R" $API/api/v1/support/$TK >/dev/null; body | js "o.origin.screen+'|'+o.refs.length+'|'+o.canManage")" "Tablero de requerimientos|2|false"

echo "== Super Admin"
check "le llega la notificación" "$(inbox "$S" platform | js "o.items.some(n=>n.type==='SUPPORT_NEW'&&n.entityId==='$TK')")" true
check "la ve con empresa y autor" "$(code -H "$S" "$API/api/v1/support?status=ACTIVE" >/dev/null; body | js "(t=>t.tenant.slug+'|'+t.reporterName)(o.find(t=>t.id==='$TK'))")" "fractal|Smoke RECRUITER"
check "ve la imagen" "$(code -H "$S" "$API/api/v1/support/files/$F1?inline=1")" 200
check "estado inválido" "$(req POST /api/v1/support/$TK/messages "$S" '{"body":"x","status":"NADA"}')" 422
check "pasa a en atención sin texto" "$(req POST /api/v1/support/$TK/messages "$S" '{"status":"IN_PROGRESS"}')" 200
check "responde" "$(req POST /api/v1/support/$TK/messages "$S" '{"body":"Corregido en la versión 1.2. Prueba de nuevo, por favor."}')" 200
check "queda respondida" "$(body | js o.status)" ANSWERED
check "el autor recibe la respuesta" "$(inbox "$R" tenant | js "o.items.some(n=>n.type==='SUPPORT_REPLY'&&n.entityId==='$TK')")" true
check "el otro usuario no" "$(inbox "$O" tenant | js "o.items.some(n=>n.entityId==='$TK')")" false

echo "== Conversación"
check "el autor no puede cambiar el estado" "$(req POST /api/v1/support/$TK/messages "$R" '{"body":"Sigue fallando","status":"CLOSED"}' >/dev/null; body | js o.status)" OPEN
check "su respuesta vuelve a avisar al Super Admin" "$(inbox "$S" platform | js "o.items.filter(n=>n.entityId==='$TK').map(n=>n.type).sort().join()")" "SUPPORT_NEW,SUPPORT_REPLY"
check "cierra" "$(req POST /api/v1/support/$TK/messages "$S" '{"body":"Listo.","status":"CLOSED"}' >/dev/null; body | js o.status)" CLOSED
check "conversación completa" "$(code -H "$R" $API/api/v1/support/$TK >/dev/null; body | js "o.messages.map(m=>(m.isStaff?'S':'U')+(m.statusTo?':'+m.statusTo:'')).join()")" "U,S:IN_PROGRESS,S:ANSWERED,U:OPEN,S:CLOSED"

echo "== Novedades"
check "solo el Super Admin publica" "$(req POST /api/v1/releases "$R" '{"title":{"es":"x"},"body":{"es":"x"}}')" 403
check "publica" "$(req POST /api/v1/releases "$S" '{"title":{"es":"Documentos de postulacion","en":"Application documents"},"body":{"es":"Ahora cada RQ puede pedir documentos y validar el CUL."},"module":"requirements","tourKey":"rq-detail"}')" 201
REL=$(body | jget .id)
check "llega a todos" "$(inbox "$O" tenant | js "o.items.some(n=>n.type==='RELEASE_NOTE'&&n.entityId==='$REL')")" true
check "se lista" "$(code -H "$R" $API/api/v1/releases >/dev/null; body | js "o[0].title.es")" "Documentos de postulacion"

echo "== Guías"
check "aún no vio ninguna" "$(code -H "$R" $API/api/v1/me/tours >/dev/null; body | js "Object.keys(o).length")" 0
check "marca la del tablero" "$(req PUT /api/v1/me/tours/rq-board "$R" '{"version":1}')" 204
check "queda registrada" "$(code -H "$R" $API/api/v1/me/tours >/dev/null; body | js "o['rq-board']")" 1
check "clave inválida" "$(req PUT /api/v1/me/tours/BAD_KEY "$R" '{"version":1}')" 422

echo "== Seguridad"
check "sin sesión" "$(code $API/api/v1/support)" 401

echo "== Resultado: $pass OK, $fail FAIL"
[ $fail -eq 0 ]
