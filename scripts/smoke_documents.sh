#!/usr/bin/env bash
# Prueba de humo de documentos de postulación: maestro del tenant, documentos por RQ, carga y validación
# manual, CUL sin RUC del tenant (queda pendiente sin consultar al MTPE), reutilización y línea de tiempo.
# Uso: SA_EMAIL=... SA_PASSWORD=... ./scripts/smoke_documents.sh [http://localhost:8080]
set -u
API=${1:-http://localhost:8080}
J="Content-Type: application/json"
pass=0; fail=0
check() { if [ "$2" = "$3" ]; then echo "  OK   $1"; pass=$((pass+1)); else echo "  FAIL $1 (esperado $3, obtuvo $2) $(head -c 200 /tmp/sd_body)"; fail=$((fail+1)); fi; }
code() { curl -s -o /tmp/sd_body -w '%{http_code}' "$@"; }
body() { cat /tmp/sd_body; }
jget() { node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const o=JSON.parse(d);const v=eval('o'+process.argv[1]);console.log(typeof v==='object'?JSON.stringify(v):(v??''))})" "$1"; }
js() { node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const o=JSON.parse(d);const v=(new Function('o','return '+process.argv[1]))(o);console.log(typeof v==='object'?JSON.stringify(v):String(v))})" "$1"; }
uuid() { node -e "console.log(crypto.randomUUID())"; }
req() { code -X "$1" -H "$3" -H "$J" -d "$4" "$API$2"; }
upload() { code -X POST -H "$1" -F "file=@$2;type=application/pdf" -F "typeId=$3" -F "culCode=${5:-}" -F "culDoc=${6:-}" -F "today=$TODAY" "$API/api/v1/tenant/requirements/candidates/$4/documents"; }
detail() { req POST /api/v1/tenant/requirements/candidates/$1/detail "$2" "{\"today\":\"$TODAY\"}" >/dev/null; body; }

code -H "$J" -d "{\"email\":\"$SA_EMAIL\",\"password\":\"$SA_PASSWORD\"}" $API/api/v1/auth/login >/dev/null; SA=$(body | jget .accessToken)
code -H "Authorization: Bearer $SA" $API/api/v1/platform/tenants >/dev/null; FRACTAL=$(body | jget '.items.find(t=>t.slug==="fractal").id')
check "Fractal sin RUC (el CUL no se consulta)" "$(code -H "Authorization: Bearer $SA" $API/api/v1/platform/tenants >/dev/null; body | js "o.items.find(t=>t.slug==='fractal').taxId??''")" ""
mkuser() {
  local email="$1.$RANDOM$RANDOM@fractal.test" role=$2 pw="Talent-$RANDOM-Seg!x"
  code -H "Authorization: Bearer $SA" -H "$J" -d "{\"email\":\"$email\",\"role\":\"$role\"}" "$API/api/v1/platform/invitations?tenantId=$FRACTAL" >/dev/null
  local tok=$(body | jget .link | sed 's#.*/invitacion/##')
  code -H "$J" -d "{\"fullName\":\"Smoke $role\",\"password\":\"$pw\",\"locale\":\"es\"}" $API/api/v1/invitations/$tok/accept >/dev/null
  code -H "$J" -d "{\"tenant\":\"fractal\",\"email\":\"$email\",\"password\":\"$pw\"}" $API/api/v1/auth/login >/dev/null
  body | jget .accessToken
}
AD=$(mkuser admin TENANT_ADMIN); RC=$(mkuser reclu RECRUITER); VW=$(mkuser consulta VIEWER)
A="Authorization: Bearer $AD"; R="Authorization: Bearer $RC"; V="Authorization: Bearer $VW"
code -H "$A" "$API/api/v1/masters/items?tables=RQ_STATUS,RQ_PROFILE,DOCUMENT_TYPE" >/dev/null; M=$(body)
mid() { echo "$M" | node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const a=JSON.parse(d);const x=a.find(i=>i.table===process.argv[1]&&i.code===process.argv[2]);console.log(x?x.id:'')})" "$1" "$2"; }
REG=$(mid RQ_STATUS REGISTERED); QA_P=$(mid RQ_PROFILE QA_ANALYST); CUL=$(mid DOCUMENT_TYPE CUL); CERT=$(mid DOCUMENT_TYPE CERTIFICATE)
TODAY=$(date +%F); DUE=$(date -d "+20 day" +%F)
check "tipo CUL en el catálogo" "$([ -n "$CUL" ] && [ -n "$CERT" ] && echo si)" si
PDF=$(cygpath -w /tmp/sd_doc.pdf 2>/dev/null || echo /tmp/sd_doc.pdf); printf '%%PDF-1.4\n%% smoke %s\n%%%%EOF\n' "$RANDOM" > $PDF

echo "== Maestro de documentos del tenant"
check "CUL: obligatorio, reutilizable, 90 días, automático" "$(req PUT /api/v1/tenant/document-rules/$CUL "$A" '{"defaultMode":"REQUIRED","reusable":true,"validityDays":90,"verification":"AUTO_CUL","active":true,"sortOrder":1}')" 204
check "certificado: opcional, por postulación, manual" "$(req PUT /api/v1/tenant/document-rules/$CERT "$A" '{"defaultMode":"OPTIONAL","reusable":false,"verification":"MANUAL","active":true,"sortOrder":2}')" 204
check "automático solo para el CUL" "$(req PUT /api/v1/tenant/document-rules/$CERT "$A" '{"defaultMode":"OPTIONAL","reusable":false,"verification":"AUTO_CUL","active":true}')" 422
check "el reclutador no edita el maestro" "$(req PUT /api/v1/tenant/document-rules/$CERT "$R" '{"defaultMode":"OPTIONAL","verification":"MANUAL","active":true}')" 403
check "el reclutador lo consulta" "$(code -H "$R" $API/api/v1/tenant/document-rules >/dev/null; body | js "o.filter(r=>r.typeId==='$CUL'||r.typeId==='$CERT').map(r=>r.code+':'+r.verification).sort().join()")" "CERTIFICATE:MANUAL,CUL:AUTO_CUL"

echo "== RQ con documentos"
CL=$(uuid)
req PUT /api/v1/tenant/masters/items "$A" "{\"table\":\"CLIENT\",\"id\":\"$CL\",\"code\":\"DOC$RANDOM$RANDOM\",\"label\":{\"es\":\"Cliente docs $RANDOM\"}}" >/dev/null
rq() { echo "{\"clientId\":\"$CL\",\"title\":\"$1\",\"statusId\":\"$REG\",\"requestDate\":\"$TODAY\",\"dueDate\":\"$DUE\",\"vacancies\":[{\"id\":\"$2\",\"profileId\":\"$QA_P\",\"quantity\":1}],\"documents\":[{\"typeId\":\"$CUL\",\"mode\":\"REQUIRED\"},{\"typeId\":\"$CERT\",\"mode\":\"OPTIONAL\"}]}"; }
RQ1=$(uuid); V1=$(uuid); RQ2=$(uuid); V2=$(uuid)
check "alta con documentos" "$(req PUT /api/v1/tenant/requirements/$RQ1 "$A" "$(rq 'Docs uno' $V1)")" 201
check "otro RQ con los mismos documentos" "$(req PUT /api/v1/tenant/requirements/$RQ2 "$A" "$(rq 'Docs dos' $V2)")" 201
check "la ficha lista los documentos" "$(code -H "$V" $API/api/v1/tenant/requirements/$RQ1 >/dev/null; body | js "o.documents.map(d=>d.code+':'+d.mode).sort().join()")" "CERTIFICATE:OPTIONAL,CUL:REQUIRED"
T1=$(uuid)
code -X POST -H "$A" -H "$J" -d "{\"id\":\"$T1\",\"firstName\":\"Doc\",\"lastName\":\"Smoke$RANDOM$RANDOM\"}" $API/api/v1/tenant/talents >/dev/null
C1=$(uuid); C2=$(uuid)
req POST /api/v1/tenant/requirements/$RQ1/candidates "$R" "{\"id\":\"$C1\",\"talentId\":\"$T1\",\"vacancyId\":\"$V1\"}" >/dev/null
req POST /api/v1/tenant/requirements/$RQ2/candidates "$R" "{\"id\":\"$C2\",\"talentId\":\"$T1\",\"vacancyId\":\"$V2\"}" >/dev/null
check "sin documentos: todo faltante" "$(code -H "$V" $API/api/v1/tenant/requirements/$RQ1 >/dev/null; body | js "o.candidates[0].docs.map(d=>d.status).join()")" "MISSING,MISSING"
check "ficha del postulante: alta en la línea de tiempo" "$(detail $C1 "$V" | js "o.events.map(e=>e.type).join()+'|'+o.documents.length+'|'+o.events[0].byName")" "ADDED|2|Smoke RECRUITER"

echo "== Carga y validación manual"
check "consulta no carga documentos" "$(upload "$V" $PDF $CERT $C1)" 403
check "carga el certificado" "$(upload "$R" $PDF $CERT $C1)" 201
check "queda pendiente" "$(body | js o.status)" PENDING
DOC=$(detail $C1 "$R" | js "o.documents.find(d=>d.code==='CERTIFICATE').current.id")
vfy() { req POST /api/v1/tenant/requirements/candidates/$C1/documents/$DOC/verify "$R" "$1"; }
check "rechazar exige motivo" "$(vfy '{"approve":false}')" 422
check "rechazar con motivo" "$(vfy '{"approve":false,"reason":"Ilegible"}')" 204
check "rechazado con el motivo" "$(detail $C1 "$R" | js "(d=>d.status+':'+d.reason+':'+d.method)(o.documents.find(d=>d.code==='CERTIFICATE').current)")" "REJECTED:Ilegible:MANUAL"
check "aprobar" "$(vfy '{"approve":true}')" 204
check "verificado, con quién lo validó" "$(detail $C1 "$R" | js "(d=>d.status+':'+d.verifiedByName)(o.documents.find(d=>d.code==='CERTIFICATE').current)")" "VERIFIED:Smoke RECRUITER"
check "el manual no se reconsulta" "$(req POST /api/v1/tenant/requirements/candidates/$C1/documents/$DOC/recheck "$R" '{}')" 422
check "por postulación: en el otro RQ sigue faltando" "$(detail $C2 "$R" | js "o.documents.find(d=>d.code==='CERTIFICATE').current===null")" true

echo "== CUL (automático)"
check "carga el CUL" "$(upload "$R" $PDF $CUL $C1 20269409807 43989632)" 201
check "sin RUC del tenant queda pendiente" "$(body | js "o.status+':'+o.reason")" "PENDING:TENANT_RUC_MISSING"
check "reutilizable: también cuenta en el otro RQ" "$(detail $C2 "$R" | js "(d=>d.status+':'+d.fromOtherRq)(o.documents.find(d=>d.code==='CUL').current)")" "PENDING:true"
CULDOC=$(detail $C1 "$R" | js "o.documents.find(d=>d.code==='CUL').current.id")
check "reconsultar sin código del QR" "$(req POST /api/v1/tenant/requirements/candidates/$C1/documents/$CULDOC/recheck "$R" '{}' >/dev/null; body | js "o.status+':'+o.reason")" "PENDING:TENANT_RUC_MISSING"
check "vence a los 90 días" "$(detail $C1 "$R" | js "o.documents.find(d=>d.code==='CUL').current.expiresAt")" "$(date -d "+90 day" +%F)"
check "resumen en el tablero del RQ" "$(code -H "$V" $API/api/v1/tenant/requirements/$RQ1 >/dev/null; body | js "o.candidates[0].docs.map(d=>d.status).sort().join()")" "PENDING,VERIFIED"
check "línea de tiempo completa" "$(detail $C1 "$R" | js "o.events.map(e=>e.type).join()")" "ADDED,DOC_UPLOADED,DOC_REJECTED,DOC_VERIFIED,DOC_UPLOADED,DOC_CHECK_FAILED,DOC_CHECK_FAILED"

echo "== Seguridad"
check "postulante de otro tenant o inexistente" "$(req POST /api/v1/tenant/requirements/candidates/$(uuid)/detail "$R" "{}")" 404
check "sin sesión" "$(code -X POST -H "$J" -d '{}' $API/api/v1/tenant/requirements/candidates/$C1/detail)" 401

echo "== Resultado: $pass OK, $fail FAIL"
[ $fail -eq 0 ]
