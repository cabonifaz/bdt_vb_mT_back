#!/usr/bin/env bash
# Prueba de humo: clientes, lista negra y entrevistas.
# Uso: SA_EMAIL=... SA_PASSWORD=... ./scripts/smoke_blacklist_interviews.sh [http://localhost:8080]
set -u
API=${1:-http://localhost:8080}
J="Content-Type: application/json"
pass=0; fail=0
BODY=$(mktemp); trap 'rm -f "$BODY" /tmp/sbi_body' EXIT
check() { if [ "$2" = "$3" ]; then echo "  OK   $1"; pass=$((pass+1)); else echo "  FAIL $1 (esperado $3, obtuvo $2) $(head -c 220 /tmp/sbi_body)"; fail=$((fail+1)); fi; }
# cuerpo por archivo: en Windows los argumentos pierden el UTF-8
req() { local m=$1 path=$2 tok=$3 data=${4:-}; if [ -n "$data" ]; then printf '%s' "$data" > "$BODY"; curl -s -o /tmp/sbi_body -w '%{http_code}' -X "$m" -H "Authorization: Bearer $tok" -H "$J" --data-binary "@$(cygpath -w "$BODY" 2>/dev/null || echo "$BODY")" "$API$path"; else curl -s -o /tmp/sbi_body -w '%{http_code}' -X "$m" -H "Authorization: Bearer $tok" "$API$path"; fi; }
body() { cat /tmp/sbi_body; }
jget() { node -e "let d='';process.stdin.setEncoding('utf8');process.stdin.on('data',c=>d+=c).on('end',()=>{const o=JSON.parse(d);const v=eval('o'+process.argv[1]);console.log(typeof v==='object'?JSON.stringify(v):(v??''))})" "$1"; }
uuid() { node -e "console.log(crypto.randomUUID())"; }

# --- preparación: admin y reclutador de Fractal
SA=$(curl -s -H "$J" -d "{\"email\":\"$SA_EMAIL\",\"password\":\"$SA_PASSWORD\"}" $API/api/v1/auth/login | jget .accessToken)
FRACTAL=$(curl -s -H "Authorization: Bearer $SA" $API/api/v1/platform/tenants | jget '.items.find(t=>t.slug==="fractal").id')
mkuser() {
  local email="$1.$RANDOM$RANDOM@fractal.test" role=$2 pw="Smoke-$RANDOM-Seg!x"
  local link=$(curl -s -H "Authorization: Bearer $SA" -H "$J" -d "{\"email\":\"$email\",\"role\":\"$role\"}" "$API/api/v1/platform/invitations?tenantId=$FRACTAL" | jget .link)
  curl -s -o /dev/null -H "$J" -d "{\"fullName\":\"Smoke $role\",\"password\":\"$pw\"}" $API/api/v1/invitations/${link##*/}/accept
  curl -s -H "$J" -d "{\"tenant\":\"fractal\",\"email\":\"$email\",\"password\":\"$pw\"}" $API/api/v1/auth/login | jget .accessToken
}
AD=$(mkuser admin TENANT_ADMIN); RC=$(mkuser reclu RECRUITER); VW=$(mkuser viewer VIEWER)
M=$(curl -s -H "Authorization: Bearer $AD" "$API/api/v1/masters/items?tables=INTERVIEW_TYPE,INTERVIEW_STAGE,INTERVIEW_STATUS,INTERVIEW_DURATION,INTERVIEW_FILE_TYPE,PHONE_QUESTION")
mid() { echo "$M" | node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const a=JSON.parse(d);const i=a.find(i=>i.table===process.argv[1]&&i.code===process.argv[2]);console.log(i?i.id:'')})" "$1" "$2"; }
VIRTUAL=$(mid INTERVIEW_TYPE VIRTUAL); INPERSON=$(mid INTERVIEW_TYPE IN_PERSON); PHONE=$(mid INTERVIEW_TYPE PHONE)
RSTEAM=$(mid INTERVIEW_STAGE RS_TEAM); PHONESCR=$(mid INTERVIEW_STAGE PHONE_SCREEN); TECHC=$(mid INTERVIEW_STAGE TECH_CLIENT)
REG=$(mid INTERVIEW_STATUS REGISTERED); CANC=$(mid INTERVIEW_STATUS CANCELLED); D60=$(mid INTERVIEW_DURATION MIN60)
FT_TEST=$(mid INTERVIEW_FILE_TYPE TECH_TEST); Q1=$(mid PHONE_QUESTION Q_AVAILABILITY); Q2=$(mid PHONE_QUESTION Q_SALARY)

echo "== Clientes (catálogo del tenant)"
C1=$(uuid); C2=$(uuid)
check "admin crea cliente 1" "$(req PUT /api/v1/tenant/masters/items $AD "{\"table\":\"CLIENT\",\"id\":\"$C1\",\"code\":\"CLI$RANDOM$RANDOM\",\"label\":{\"es\":\"Banco Andino\"},\"str1\":\"20100000001\",\"str2\":\"Av. Arequipa 123\",\"str3\":\"https://maps.app.goo.gl/abc\"}")" 200
check "admin crea cliente 2" "$(req PUT /api/v1/tenant/masters/items $AD "{\"table\":\"CLIENT\",\"id\":\"$C2\",\"code\":\"CLI$RANDOM$RANDOM\",\"label\":{\"es\":\"Retail Digital\"}}")" 200
check "reclutador no administra clientes" "$(req PUT /api/v1/tenant/masters/items $RC "{\"table\":\"CLIENT\",\"code\":\"X$RANDOM\",\"label\":{\"es\":\"X\"}}")" 403

T=$(uuid); LAST="Restringido$RANDOM$RANDOM"
req POST /api/v1/tenant/talents $AD "{\"id\":\"$T\",\"firstName\":\"Pablo\",\"lastName\":\"$LAST\",\"email\":\"pablo.$RANDOM$RANDOM@correo.test\"}" >/dev/null

echo "== Lista negra"
B1=$(uuid)
check "restricción para cliente 1" "$(req POST /api/v1/tenant/blacklist $RC "{\"id\":\"$B1\",\"talentId\":\"$T\",\"clientId\":\"$C1\",\"reason\":\"No se presentó a la entrevista\"}")" 201
check "duplicada rechazada" "$(req POST /api/v1/tenant/blacklist $RC "{\"talentId\":\"$T\",\"clientId\":\"$C1\",\"reason\":\"otra\"}")" 409
check "motivo obligatorio" "$(req POST /api/v1/tenant/blacklist $RC "{\"talentId\":\"$T\",\"clientId\":\"$C2\",\"reason\":\"  \"}")" 422
check "consulta (VIEWER) no puede restringir" "$(req POST /api/v1/tenant/blacklist $VW "{\"talentId\":\"$T\",\"clientId\":\"$C2\",\"reason\":\"x\"}")" 403
check "estado: 1 restricción" "$(req GET /api/v1/tenant/talents/$T/blacklist $VW)" 200
check "1 activa" "$(body | jget .length)" 1
check "editar motivo" "$(req PATCH /api/v1/tenant/blacklist/$B1 $RC '{"reason":"No asistió y no avisó"}')" 200
BG=$(uuid)
check "restricción GLOBAL absorbe la del cliente" "$(req POST /api/v1/tenant/blacklist $AD "{\"id\":\"$BG\",\"talentId\":\"$T\",\"reason\":\"Información falsa en el CV\"}")" 201
check "queda solo la global" "$(body | jget '.length + (o[0].clientId?"c":"g")')" 1g
check "con global no se agregan por cliente" "$(req POST /api/v1/tenant/blacklist $RC "{\"talentId\":\"$T\",\"clientId\":\"$C2\",\"reason\":\"x\"}")" 409
check "listado agrupado por talento" "$(req POST /api/v1/tenant/blacklist/search $RC "{\"search\":\"$LAST\"}")" 200
check "aparece como global" "$(body | jget .items[0].isGlobal)" true
check "filtro solo globales" "$(req POST /api/v1/tenant/blacklist/search $RC "{\"search\":\"$LAST\",\"globalOnly\":true}")" 200
check "lo incluye" "$(body | jget .total)" 1
check "la búsqueda de talentos lo marca" "$(req POST /api/v1/tenant/talents/search $RC "{\"search\":\"$LAST\"}")" 200
check "isBlacklisted" "$(body | jget .items[0].isBlacklisted)" true
check "levantar global sin motivo de cliente falla" "$(req POST /api/v1/tenant/blacklist/$BG/lift-global $AD "{\"reason\":\"Se aclaró\",\"keep\":[{\"client\":\"$C1\",\"reason\":\"\"}]}")" 422
check "y es atómico: la global sigue activa" "$(req GET /api/v1/tenant/talents/$T/blacklist $RC; body | jget '.length + (o[0].clientId?"c":"g")')" 2001g
check "levantar global conservando cliente 1" "$(req POST /api/v1/tenant/blacklist/$BG/lift-global $AD "{\"reason\":\"Se aclaró la información\",\"keep\":[{\"client\":\"$C1\",\"reason\":\"Se mantiene por inasistencia\"}]}")" 200
check "queda 1 restricción por cliente" "$(body | jget '.length + (o[0].clientId?"c":"g")')" 1c
check "historial con todos los movimientos" "$(req GET "/api/v1/tenant/talents/$T/blacklist/history?pageSize=50" $RC)" 200
check "CREATE/UPDATE/DELETE registrados" "$(body | jget '.items.map(i=>i.movement).filter((v,i,a)=>a.indexOf(v)===i).sort().join(",")')" "CREATE,DELETE,UPDATE"
check "con autor" "$(body | jget '.items.every(i=>i.userName.startsWith("Smoke"))')" true

echo "== Entrevistas"
I1=$(uuid)
base="\"talentId\":\"$T\",\"stageId\":\"$TECHC\",\"statusId\":\"$REG\",\"profile\":\"Java Senior\",\"date\":\"2026-11-05\",\"time\":\"10:30\",\"durationId\":\"$D60\""
check "virtual sin enlace rechazada" "$(req POST /api/v1/tenant/interviews $RC "{\"id\":\"$I1\",$base,\"typeId\":\"$VIRTUAL\",\"clientId\":\"$C2\",\"interviewers\":[{\"name\":\"Luis\"}]}")" 422
check "  código" "$(body | jget .error.code)" INTERVIEW_LINK_REQUIRED
check "sin entrevistadores (etapa cliente) rechazada" "$(req POST /api/v1/tenant/interviews $RC "{\"id\":\"$I1\",$base,\"typeId\":\"$VIRTUAL\",\"clientId\":\"$C2\",\"meetingLink\":\"https://meet.example/abc\"}")" 422
check "  código" "$(body | jget .error.code)" INTERVIEWERS_REQUIRED
check "notificar sin correo rechazado" "$(req POST /api/v1/tenant/interviews $RC "{\"id\":\"$I1\",$base,\"typeId\":\"$VIRTUAL\",\"clientId\":\"$C2\",\"meetingLink\":\"https://meet.example/abc\",\"interviewers\":[{\"name\":\"Luis\",\"notify\":true}]}")" 422
check "virtual completa" "$(req POST /api/v1/tenant/interviews $RC "{\"id\":\"$I1\",$base,\"typeId\":\"$VIRTUAL\",\"clientId\":\"$C2\",\"meetingLink\":\"https://meet.example/abc\",\"interviewers\":[{\"name\":\"Luis Paz\",\"email\":\"luis.paz@cliente.test\",\"notify\":true}],\"recordings\":[{\"url\":\"https://drive.example/rec1\",\"date\":\"2026-11-05\"}]}")" 201
check "reenvío idempotente (mismo uuid)" "$(req POST /api/v1/tenant/interviews $RC "{\"id\":\"$I1\",$base,\"typeId\":\"$VIRTUAL\",\"clientId\":\"$C2\",\"meetingLink\":\"https://meet.example/abc\",\"interviewers\":[{\"name\":\"Luis Paz\",\"email\":\"luis.paz@cliente.test\",\"notify\":true}]}")" 200
check "detalle" "$(req GET /api/v1/tenant/interviews/$I1 $VW)" 200
check "correo del entrevistador descifrado" "$(body | jget .interviewers[0].email)" luis.paz@cliente.test
I2=$(uuid)
check "cliente con restricción: aviso de lista negra" "$(req POST /api/v1/tenant/interviews $RC "{\"id\":\"$I2\",$base,\"typeId\":\"$VIRTUAL\",\"clientId\":\"$C1\",\"meetingLink\":\"https://meet.example/x\",\"interviewers\":[{\"name\":\"Ana\"}]}")" 409
check "  código" "$(body | jget .error.code)" TALENT_BLACKLISTED
check "confirmando el aviso se registra" "$(req POST /api/v1/tenant/interviews $RC "{\"id\":\"$I2\",$base,\"typeId\":\"$VIRTUAL\",\"clientId\":\"$C1\",\"meetingLink\":\"https://meet.example/x\",\"interviewers\":[{\"name\":\"Ana\"}],\"acknowledgeBlacklist\":true}")" 201
I3=$(uuid)
check "presencial sin Google Maps rechazada" "$(req POST /api/v1/tenant/interviews $RC "{\"id\":\"$I3\",$base,\"typeId\":\"$INPERSON\",\"clientId\":\"$C2\",\"locationUrl\":\"https://evil.example/maps\",\"address\":\"Av. X 1\",\"interviewers\":[{\"name\":\"Ana\"}]}")" 422
check "presencial completa" "$(req POST /api/v1/tenant/interviews $RC "{\"id\":\"$I3\",$base,\"typeId\":\"$INPERSON\",\"clientId\":\"$C2\",\"locationUrl\":\"https://maps.app.goo.gl/xyz\",\"address\":\"Av. Arequipa 123, Lima\",\"interviewers\":[{\"name\":\"Ana\"}]}")" 201
check "grabaciones solo en virtual" "$(req PUT /api/v1/tenant/interviews/$I3 $RC "{$base,\"typeId\":\"$INPERSON\",\"clientId\":\"$C2\",\"locationUrl\":\"https://maps.app.goo.gl/xyz\",\"address\":\"Av. Arequipa 123\",\"interviewers\":[{\"name\":\"Ana\"}],\"recordings\":[{\"url\":\"https://x.example/r\",\"date\":\"2026-11-05\"}]}")" 422
IP=$(uuid)
check "telefónica sin cliente ni enlace + cuestionario" "$(req POST /api/v1/tenant/interviews $RC "{\"id\":\"$IP\",\"talentId\":\"$T\",\"typeId\":\"$PHONE\",\"stageId\":\"$PHONESCR\",\"statusId\":\"$REG\",\"profile\":\"Analista QA\",\"date\":\"2026-11-04\",\"time\":\"09:15\",\"meetingLink\":\"https://ignorado.example\",\"interviewers\":[{\"name\":\"Smoke Reclutador\"}],\"answers\":[{\"question\":\"$Q1\",\"answer\":\"Inmediata\"},{\"question\":\"$Q2\",\"answer\":\"S/ 6000 planilla\"}]}")" 201
req GET /api/v1/tenant/interviews/$IP $RC >/dev/null
check "telefónica descarta el enlace" "$(body | jget '.meetingLink??"null"')" null
check "respuestas guardadas" "$(body | jget '.answers.filter(a=>a.answer).length')" 2
check "vaciar una respuesta la elimina" "$(req PUT /api/v1/tenant/interviews/$IP $RC "{\"talentId\":\"$T\",\"typeId\":\"$PHONE\",\"stageId\":\"$PHONESCR\",\"statusId\":\"$REG\",\"profile\":\"Analista QA\",\"date\":\"2026-11-04\",\"time\":\"09:15\",\"interviewers\":[{\"name\":\"Smoke Reclutador\"}],\"answers\":[{\"question\":\"$Q1\",\"answer\":\"\"}],\"ratingGeneral\":4}")" 200
req GET /api/v1/tenant/interviews/$IP $RC >/dev/null
check "queda 1 respuesta" "$(body | jget '.answers.filter(a=>a.answer).length')" 1
check "el talento no se puede cambiar" "$(req PUT /api/v1/tenant/interviews/$IP $RC "{\"talentId\":\"$(uuid)\",\"typeId\":\"$PHONE\",\"stageId\":\"$PHONESCR\",\"statusId\":\"$REG\",\"profile\":\"x\",\"date\":\"2026-11-04\",\"time\":\"09:15\",\"interviewers\":[{\"name\":\"a\"}]}")" 404
check "cancelar con motivo" "$(req PUT /api/v1/tenant/interviews/$I3 $RC "{$base,\"typeId\":\"$INPERSON\",\"statusId\":\"$CANC\",\"clientId\":\"$C2\",\"locationUrl\":\"https://maps.app.goo.gl/xyz\",\"address\":\"Av. Arequipa 123\",\"interviewers\":[{\"name\":\"Ana\"}],\"cancelReason\":\"El candidato aceptó otra oferta\"}")" 200
check "estrellas fuera de rango" "$(req PUT /api/v1/tenant/interviews/$I1 $RC "{$base,\"typeId\":\"$VIRTUAL\",\"clientId\":\"$C2\",\"meetingLink\":\"https://meet.example/abc\",\"interviewers\":[{\"name\":\"Luis\"}],\"ratingGeneral\":7}")" 422
check "búsqueda por talento" "$(req POST /api/v1/tenant/interviews/search $VW "{\"talentId\":\"$T\"}")" 200
check "4 entrevistas" "$(body | jget .total)" 4
check "filtro telefónicas" "$(req POST /api/v1/tenant/interviews/search $VW "{\"talentId\":\"$T\",\"typeId\":\"$PHONE\"}")" 200
check "1 telefónica" "$(body | jget .total)" 1
check "consulta (VIEWER) no puede crear" "$(req POST /api/v1/tenant/interviews $VW "{\"talentId\":\"$T\"}")" 403
check "invitación de calendario (ICS)" "$(req GET /api/v1/tenant/interviews/$I1/ics $VW)" 200
check "ICS válido con hora UTC" "$(grep -c 'DTSTART:20261105T153000Z' /tmp/sbi_body)" 1
printf '%%PDF-1.4\n1 0 obj<<>>endobj\n%%%%EOF\n' > "$BODY"
check "subir prueba técnica" "$(curl -s -o /tmp/sbi_body -w '%{http_code}' -X POST -H "Authorization: Bearer $RC" -F "file=@$(cygpath -w "$BODY" 2>/dev/null || echo "$BODY");filename=prueba.pdf;type=application/pdf" "$API/api/v1/tenant/interviews/$I1/files?typeId=$FT_TEST")" 201
FID=$(body | jget .id)
check "descargar archivo" "$(req GET /api/v1/tenant/interviews/files/$FID $VW)" 200
echo "== Resultado: $pass OK, $fail FAIL"
[ $fail -eq 0 ]
