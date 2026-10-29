#!/usr/bin/env bash
# Prueba de humo de requerimientos (RQ): tablero, ficha, vacantes, pipeline de candidatos, contactos y permisos.
# Uso: SA_EMAIL=... SA_PASSWORD=... ./scripts/smoke_requirements.sh [http://localhost:8080]
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
AD=$(mkuser admin TENANT_ADMIN); RC=$(mkuser reclu RECRUITER); VW=$(mkuser consulta VIEWER); LD=$(mkuser jefe SELECTION_LEAD)
A="Authorization: Bearer $AD"; R="Authorization: Bearer $RC"; V="Authorization: Bearer $VW"; L="Authorization: Bearer $LD"
code -H "$A" "$API/api/v1/masters/items?tables=RQ_STATUS,RQ_PROFILE,TIME_UNIT,WORK_MODALITY,CONTRACT_MODALITY,CANDIDATE_STAGE,CURRENCY,TECH_SKILL,ACADEMIC_GRADE" >/dev/null
M=$(body)
mid() { echo "$M" | node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const a=JSON.parse(d);const x=a.find(i=>i.table===process.argv[1]&&i.code===process.argv[2]);console.log(x?x.id:'')})" "$1" "$2"; }
REG=$(mid RQ_STATUS REGISTERED); ASG=$(mid RQ_STATUS ASSIGNED); CAN=$(mid RQ_STATUS CANCELLED); JAVA_P=$(mid RQ_PROFILE JAVA_DEV); QA_P=$(mid RQ_PROFILE QA_ANALYST); SEL=$(mid RQ_STATUS IN_SELECTION)
MONTHS=$(mid TIME_UNIT MONTHS); REMOTE=$(mid WORK_MODALITY REMOTE); SERV=$(mid CONTRACT_MODALITY SERVICES); PEN=$(mid CURRENCY PEN)
JAVA=$(mid TECH_SKILL JAVA); BACH=$(mid ACADEMIC_GRADE BACHELOR)
OBS=$(mid CANDIDATE_STAGE OBSERVED); DATA=$(mid CANDIDATE_STAGE DATA_COMPLETE); CONF=$(mid CANDIDATE_STAGE CONFIRMED)
check "maestros de RQ cargados" "$([ -n "$REG" ] && [ -n "$JAVA_P" ] && [ -n "$CONF" ] && echo si)" si
req() { code -X "$1" -H "$3" -H "$J" -d "$4" "$API$2"; }
# jq-like con node: js "<expresión sobre o>"
js() { node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const o=JSON.parse(d);const v=(new Function('o','return '+process.argv[1]))(o);console.log(typeof v==='object'?JSON.stringify(v):String(v))})" "$1"; }
edit() { echo "$RQB" | node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const o=JSON.parse(d);(new Function('o',process.argv[1]))(o);console.log(JSON.stringify(o))})" "$1"; }

echo "== Cliente y contactos"
CL=$(uuid); CT=$(uuid)
check "admin crea cliente" "$(req PUT /api/v1/tenant/masters/items "$A" "{\"table\":\"CLIENT\",\"id\":\"$CL\",\"code\":\"RQC$RANDOM$RANDOM\",\"label\":{\"es\":\"Cliente RQ $RANDOM\"}}")" 200
check "contacto del cliente" "$(req PUT /api/v1/tenant/clients/$CL/contacts/$CT "$A" '{"firstName":"Carla","lastName":"Rios","position":"Jefa de TI","email":"carla@cliente.test","phone":"999888777"}')" 204
check "correo de contacto inválido" "$(req PUT /api/v1/tenant/clients/$CL/contacts/$(uuid) "$A" '{"firstName":"X","lastName":"Y","email":"no-es-correo"}')" 422
check "reclutador no edita contactos" "$(req PUT /api/v1/tenant/clients/$CL/contacts/$(uuid) "$R" '{"firstName":"X","lastName":"Y"}')" 403
check "contactos del cliente" "$(code -H "$R" $API/api/v1/tenant/clients/$CL/contacts >/dev/null; body | js 'o[0].firstName')" Carla

echo "== Alta del RQ (código autogenerado, vacantes, tarifas y banda salarial)"
RQ=$(uuid); V1=$(uuid); V2=$(uuid); TODAY=$(date +%F); DUE=$(date -d "+20 day" +%F)
RQB="{\"clientId\":\"$CL\",\"title\":\"Desarrolladores Java para core bancario\",\"description\":\"Proyecto de migracion\",\"statusId\":\"$REG\",\"requestDate\":\"$TODAY\",\"dueDate\":\"$DUE\",\"contractValue\":6,\"contractUnitId\":\"$MONTHS\",\"workModalityId\":\"$REMOTE\",\"needsLaptop\":true,\"contacts\":[\"$CT\"],\"billing\":[{\"contractId\":\"$SERV\",\"currencyId\":\"$PEN\",\"baseMin\":6000,\"baseMax\":9000}],\"vacancies\":[{\"id\":\"$V1\",\"profileId\":\"$JAVA_P\",\"quantity\":1,\"rateInitial\":45,\"rateFinal\":60,\"skills\":[{\"skillId\":\"$JAVA\",\"years\":3}],\"careers\":[{\"career\":\"Ingenieria de Sistemas\",\"gradeId\":\"$BACH\"}]},{\"id\":\"$V2\",\"profileId\":\"$QA_P\",\"quantity\":2}]}"
check "reclutador no crea RQ" "$(req PUT /api/v1/tenant/requirements/$RQ "$R" "$RQB")" 403
check "admin crea RQ" "$(req PUT /api/v1/tenant/requirements/$RQ "$A" "$RQB")" 201
CODE=$(body | js 'o.code')
check "código autogenerado" "$(echo "$CODE" | grep -cE '^RQ-[0-9]{4}-[0-9]{4}$')" 1
check "reenviar no duplica" "$(req PUT /api/v1/tenant/requirements/$RQ "$A" "$RQB")" 200
check "fechas invertidas" "$(req PUT /api/v1/tenant/requirements/$(uuid) "$A" "$(edit "o.dueDate='2000-01-01'")")" 422
check "sin vacantes" "$(req PUT /api/v1/tenant/requirements/$(uuid) "$A" "$(edit 'o.vacancies=[]')")" 422
check "código repetido" "$(req PUT /api/v1/tenant/requirements/$(uuid) "$A" "$(edit "o.code='$CODE';o.vacancies[0].id='$(uuid)';o.vacancies[1].id='$(uuid)'")")" 409

echo "== Ficha y tarifas"
code -H "$A" $API/api/v1/tenant/requirements/$RQ >/dev/null; RA=$(body)
check "admin ve tarifa" "$(echo "$RA" | js "o.vacancies.find(v=>v.id==='$V1').rateFinal")" 60
check "admin ve banda salarial" "$(echo "$RA" | js 'o.billing[0].baseMax')" 9000
check "responsable = quien lo creó" "$(echo "$RA" | js 'o.ownerName')" "Smoke TENANT_ADMIN"
check "skills y carreras" "$(echo "$RA" | js "o.vacancies.find(v=>v.id==='$V1').skills.length+'/'+o.vacancies.find(v=>v.id==='$V1').careers[0].career")" "1/Ingenieria de Sistemas"
code -H "$R" $API/api/v1/tenant/requirements/$RQ >/dev/null; RR=$(body)
check "reclutador: sin tarifa" "$(echo "$RR" | js "o.vacancies.find(v=>v.id==='$V1').rateFinal==null")" true
check "reclutador: sin banda salarial" "$(echo "$RR" | js 'o.billing==null && o.canSeeRates===false')" true

echo "== Tablero"
check "aparece en el tablero" "$(req POST /api/v1/tenant/requirements/board "$V" "{\"scope\":\"ALL\",\"search\":\"$CODE\"}" >/dev/null; body | js 'o.cards.length')" 1
check "con vacantes y cubiertas" "$(body | js "o.cards[0].vacancies+'/'+o.cards[0].covered")" "3/0"
check "mover a Asignado" "$(req PATCH /api/v1/tenant/requirements/$RQ/status "$A" "{\"statusId\":\"$ASG\",\"order\":1}")" 200
check "estado anterior" "$(body | js 'o.previousStatus')" REGISTERED
check "consulta no mueve" "$(req PATCH /api/v1/tenant/requirements/$RQ/status "$V" "{\"statusId\":\"$REG\"}")" 403

echo "== Pipeline de candidatos"
T1=$(uuid); T2=$(uuid)
code -X POST -H "$A" -H "$J" -d "{\"id\":\"$T1\",\"firstName\":\"Cand\",\"lastName\":\"Uno$RANDOM$RANDOM\"}" $API/api/v1/tenant/talents >/dev/null
code -X POST -H "$A" -H "$J" -d "{\"id\":\"$T2\",\"firstName\":\"Cand\",\"lastName\":\"Dos$RANDOM$RANDOM\"}" $API/api/v1/tenant/talents >/dev/null
C1=$(uuid); C2=$(uuid)
check "reclutador agrega candidato" "$(req POST /api/v1/tenant/requirements/$RQ/candidates "$R" "{\"id\":\"$C1\",\"talentId\":\"$T1\",\"vacancyId\":\"$V1\"}")" 201
check "mismo talento otra vez" "$(req POST /api/v1/tenant/requirements/$RQ/candidates "$R" "{\"id\":\"$(uuid)\",\"talentId\":\"$T1\",\"vacancyId\":\"$V2\"}")" 409
check "consulta no agrega" "$(req POST /api/v1/tenant/requirements/$RQ/candidates "$V" "{\"id\":\"$(uuid)\",\"talentId\":\"$T2\",\"vacancyId\":\"$V1\"}")" 403
check "segundo candidato" "$(req POST /api/v1/tenant/requirements/$RQ/candidates "$R" "{\"id\":\"$C2\",\"talentId\":\"$T2\",\"vacancyId\":\"$V1\"}")" 201
check "no se confirma desde Observado" "$(req PATCH /api/v1/tenant/requirements/candidates/$C1 "$R" "{\"stageId\":\"$CONF\"}")" 422
check "a Datos completos" "$(req PATCH /api/v1/tenant/requirements/candidates/$C1 "$R" "{\"stageId\":\"$DATA\",\"order\":0}")" 200
check "confirmar" "$(req PATCH /api/v1/tenant/requirements/candidates/$C1 "$R" "{\"stageId\":\"$CONF\"}")" 200
req PATCH /api/v1/tenant/requirements/candidates/$C2 "$R" "{\"stageId\":\"$DATA\"}" >/dev/null
check "vacante llena (cantidad 1)" "$(req PATCH /api/v1/tenant/requirements/candidates/$C2 "$R" "{\"stageId\":\"$CONF\"}")" 409
check "pero puede pasar a otra vacante con cupo" "$(req PATCH /api/v1/tenant/requirements/candidates/$C2 "$R" "{\"stageId\":\"$CONF\",\"vacancyId\":\"$V2\"}")" 200
check "cubiertas en el tablero" "$(req POST /api/v1/tenant/requirements/board "$A" "{\"scope\":\"ALL\",\"search\":\"$CODE\"}" >/dev/null; body | js 'o.cards[0].covered')" 2
check "no se quita una vacante con candidatos" "$(req PUT /api/v1/tenant/requirements/$RQ "$A" "$(edit 'o.vacancies=[o.vacancies[0]]')")" 409
check "postulaciones del talento" "$(code -H "$R" $API/api/v1/tenant/talents/$T1/applications >/dev/null; body | js "o[0].code+'|'+o[0].stage.code")" "$CODE|CONFIRMED"
check "no se borra un RQ con confirmados" "$(req DELETE /api/v1/tenant/requirements/$RQ "$A" '')" 409

echo "== RQ cerrado"
check "cancelar" "$(req PATCH /api/v1/tenant/requirements/$RQ/status "$A" "{\"statusId\":\"$CAN\"}")" 200
check "cerrado: no se agregan candidatos" "$(req POST /api/v1/tenant/requirements/$RQ/candidates "$R" "{\"id\":\"$(uuid)\",\"talentId\":\"$T2\",\"vacancyId\":\"$V1\"}")" 409
check "cerrado: no se mueven" "$(req PATCH /api/v1/tenant/requirements/candidates/$C1 "$R" "{\"stageId\":\"$DATA\"}")" 409

echo "== Reapertura (jefe de selección)"
check "mover de cerrado a abierto: exige reabrir" "$(req PATCH /api/v1/tenant/requirements/$RQ/status "$A" "{\"statusId\":\"$ASG\"}")" 409
check "el admin no reabre" "$(req POST /api/v1/tenant/requirements/$RQ/reopen "$A" "{\"reason\":\"RESIGNATION\"}")" 403
check "motivo obligatorio" "$(req POST /api/v1/tenant/requirements/$RQ/reopen "$L" "{\"reason\":\"OTRO\"}")" 422
check "el jefe reabre" "$(req POST /api/v1/tenant/requirements/$RQ/reopen "$L" "{\"reason\":\"MORE_VACANCIES\",\"notes\":\"2 mas\"}")" 200
check "vuelve a En selección" "$(body | js o.status)" IN_SELECTION
check "ya abierto: no se reabre" "$(req POST /api/v1/tenant/requirements/$RQ/reopen "$L" "{\"reason\":\"MORE_VACANCIES\"}")" 409
check "queda en el historial" "$(code -H "$V" $API/api/v1/tenant/requirements/$RQ >/dev/null; body | js "o.reopens.map(r=>r.reason+\":\"+r.notes).join()")" "MORE_VACANCIES:2 mas"
T3=$(uuid); code -X POST -H "$A" -H "$J" -d "{\"id\":\"$T3\",\"firstName\":\"Cand\",\"lastName\":\"Tres$RANDOM$RANDOM\"}" $API/api/v1/tenant/talents >/dev/null
check "abierto otra vez: se agregan candidatos" "$(req POST /api/v1/tenant/requirements/$RQ/candidates "$R" "{\"id\":\"$(uuid)\",\"talentId\":\"$T3\",\"vacancyId\":\"$V2\"}")" 201

echo "== Renuncia de un confirmado"
check "se cierra otra vez" "$(req PATCH /api/v1/tenant/requirements/$RQ/status "$A" "{\"statusId\":\"$CAN\"}")" 200
check "renuncia sin indicar quién" "$(req POST /api/v1/tenant/requirements/$RQ/reopen "$L" "{\"reason\":\"RESIGNATION\"}")" 422
check "renuncia de alguien que no es candidato confirmado" "$(req POST /api/v1/tenant/requirements/$RQ/reopen "$L" "{\"reason\":\"RESIGNATION\",\"candidates\":[\"$(uuid)\"]}")" 422
check "más vacantes no lleva candidatos" "$(req POST /api/v1/tenant/requirements/$RQ/reopen "$L" "{\"reason\":\"MORE_VACANCIES\",\"candidates\":[\"$C1\"]}")" 422
check "reabre por renuncia del primero" "$(req POST /api/v1/tenant/requirements/$RQ/reopen "$L" "{\"reason\":\"RESIGNATION\",\"candidates\":[\"$C1\"]}")" 200
check "ya no cubre la vacante" "$(req POST /api/v1/tenant/requirements/board "$A" "{\"scope\":\"ALL\",\"search\":\"$CODE\"}" >/dev/null; body | js 'o.cards[0].covered')" 1
code -H "$V" $API/api/v1/tenant/requirements/$RQ >/dev/null; RD=$(body)
check "marcado como renuncia con baja pendiente" "$(echo "$RD" | js "o.candidates.find(c=>c.id==='$C1').resignation.offboard")" PENDING
check "la vacante queda libre" "$(echo "$RD" | js "o.vacancies.find(v=>v.id==='$V1').covered")" 0
check "el historial dice quién renunció" "$(echo "$RD" | js "o.reopens.find(r=>r.reason==='RESIGNATION').resigned.length")" 1
check "no se mueve a quien renunció" "$(req PATCH /api/v1/tenant/requirements/candidates/$C1 "$R" "{\"stageId\":\"$DATA\"}")" 409
check "ni se quita" "$(req DELETE /api/v1/tenant/requirements/candidates/$C1 "$R" '')" 409
check "postulaciones del talento con la renuncia" "$(code -H "$R" $API/api/v1/tenant/talents/$T1/applications >/dev/null; body | js "o[0].resignation.offboard")" PENDING
check "el jefe crea RQ" "$(req PUT /api/v1/tenant/requirements/$(uuid) "$L" "{\"clientId\":\"$CL\",\"title\":\"Del jefe\",\"statusId\":\"$REG\",\"requestDate\":\"2026-10-01\",\"dueDate\":\"2026-12-01\",\"vacancies\":[{\"id\":\"$(uuid)\",\"profileId\":\"$QA_P\",\"quantity\":1}]}")" 201
check "el jefe no elimina RQ (solo el admin)" "$(req DELETE /api/v1/tenant/requirements/$RQ "$L" '')" 403

echo "== Notificaciones de RQ"
code -H "$R" "$API/api/v1/tenant/notifications?limit=50" >/dev/null; NR=$(body)
check "el reclutador recibe el alta (agrupada con los cambios)" "$(echo "$NR" | js "(o.items.find(i=>i.entityId==='$RQ'&&i.type==='RQ_CREATED')||{}).type")" RQ_CREATED
check "con el código" "$(echo "$NR" | js "(o.items.find(i=>i.entityId==='$RQ'&&i.type==='RQ_CREATED')||{payload:{}}).payload.code")" "$CODE"

echo "== Seguridad"
check "sin sesión" "$(code -X POST -H "$J" -d '{}' $API/api/v1/tenant/requirements/board)" 401
check "Super Admin no accede" "$(code -X POST -H "Authorization: Bearer $SA" -H "$J" -d '{}' $API/api/v1/tenant/requirements/board)" 403
check "RQ inexistente" "$(code -H "$A" $API/api/v1/tenant/requirements/$(uuid))" 404
echo "== Resultado: $pass OK, $fail FAIL"
[ $fail -eq 0 ]
