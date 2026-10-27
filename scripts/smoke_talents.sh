#!/usr/bin/env bash
# Prueba de humo del módulo de talentos.
# Uso: SA_EMAIL=... SA_PASSWORD=... ./scripts/smoke_talents.sh [http://localhost:8080]
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
code -H "$A" "$API/api/v1/masters/items?tables=LANGUAGE,LANGUAGE_LEVEL,TECH_SKILL,SOFT_SKILL,COUNTRY,DOC_IDENTITY_TYPE,ACADEMIC_GRADE,CURRENCY,TALENT_AVAILABILITY" >/dev/null
M=$(body)
mid() { echo "$M" | node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const a=JSON.parse(d);console.log(a.find(i=>i.table===process.argv[1]&&i.code===process.argv[2]).id)})" "$1" "$2"; }
EN=$(mid LANGUAGE EN); ADV=$(mid LANGUAGE_LEVEL ADVANCED); INT=$(mid LANGUAGE_LEVEL INTERMEDIATE); JAVA=$(mid TECH_SKILL JAVA); REACT=$(mid TECH_SKILL REACT)
TEAM=$(mid SOFT_SKILL TEAMWORK); PE=$(mid COUNTRY PE); DNI=$(mid DOC_IDENTITY_TYPE DNI); DEG=$(mid ACADEMIC_GRADE DEGREE); PEN=$(mid CURRENCY PEN)
IMM=$(mid TALENT_AVAILABILITY IMMEDIATE); M1=$(mid TALENT_AVAILABILITY MONTH_1)

echo "== Alta y ficha"
T=$(uuid); EMAIL="ana.$RANDOM$RANDOM@correo.test"; DOCN="$RANDOM$RANDOM"
TAL="{\"id\":\"$T\",\"firstName\":\"Ana\",\"lastName\":\"Quispe\",\"secondLastName\":\"Rojas\",\"headline\":\"Desarrolladora Java Senior\",\"docTypeId\":\"$DNI\",\"docNumber\":\"$DOCN\",\"email\":\"$EMAIL\",\"phone\":\"987654321\",\"countryId\":\"$PE\",\"city\":\"Lima\",\"linkedinUrl\":\"https://www.linkedin.com/in/ana\",\"summary\":\"Backend con 8 años\",\"availabilityId\":\"$IMM\",\"hasEquipment\":true,\"rates\":{\"payrollMin\":6000,\"payrollMax\":8000,\"payrollCurrencyId\":\"$PEN\"}}"
check "reclutador no puede fijar tarifas" "$(code -X POST -H "$R" -H "$J" -d "$TAL" $API/api/v1/tenant/talents)" 403
check "admin crea talento" "$(code -X POST -H "$A" -H "$J" -d "$TAL" $API/api/v1/tenant/talents)" 201
check "reenviar misma alta no duplica (upsert por uuid)" "$(code -X POST -H "$A" -H "$J" -d "$TAL" $API/api/v1/tenant/talents)" 200
T2=$(uuid)
check "correo duplicado rechazado" "$(code -X POST -H "$A" -H "$J" -d "{\"id\":\"$T2\",\"firstName\":\"Otra\",\"lastName\":\"Persona\",\"email\":\"$EMAIL\"}" $API/api/v1/tenant/talents)" 409
check "linkedin inválido rechazado" "$(code -X POST -H "$A" -H "$J" -d "{\"id\":\"$T2\",\"firstName\":\"Otra\",\"lastName\":\"Persona\",\"linkedinUrl\":\"javascript:alert(1)\"}" $API/api/v1/tenant/talents)" 422
check "habilidad técnica Java" "$(code -X PUT -H "$A" -H "$J" -d "{\"skillId\":\"$JAVA\",\"years\":8}" $API/api/v1/tenant/talents/$T/tech-skills)" 204
check "habilidad técnica React" "$(code -X PUT -H "$A" -H "$J" -d "{\"skillId\":\"$REACT\",\"years\":3}" $API/api/v1/tenant/talents/$T/tech-skills)" 204
check "habilidad nueva por nombre (se crea en el tenant)" "$(code -X PUT -H "$A" -H "$J" -d "{\"label\":\"Quarkus $RANDOM\",\"years\":2}" $API/api/v1/tenant/talents/$T/tech-skills)" 204
check "años inválidos" "$(code -X PUT -H "$A" -H "$J" -d "{\"skillId\":\"$JAVA\",\"years\":99}" $API/api/v1/tenant/talents/$T/tech-skills)" 422
check "habilidad blanda" "$(code -X POST -H "$A" -H "$J" -d "{\"skillId\":\"$TEAM\"}" $API/api/v1/tenant/talents/$T/soft-skills)" 204
check "experiencia actual" "$(code -X PUT -H "$A" -H "$J" -d '{"company":"Banco X","position":"Java Developer","startDate":"2018-01-01","isCurrent":true,"functions":"APIs"}' $API/api/v1/tenant/talents/$T/experiences)" 204
check "experiencia fechas inválidas" "$(code -X PUT -H "$A" -H "$J" -d '{"company":"Y","position":"Z","startDate":"2020-01-01","endDate":"2019-01-01"}' $API/api/v1/tenant/talents/$T/experiences)" 422
check "educación" "$(code -X PUT -H "$A" -H "$J" -d "{\"institution\":\"UNI\",\"career\":\"Ingeniería de Sistemas\",\"gradeId\":\"$DEG\",\"startDate\":\"2010-03-01\",\"endDate\":\"2015-12-01\"}" $API/api/v1/tenant/talents/$T/educations)" 204
check "idioma inglés avanzado" "$(code -X PUT -H "$A" -H "$J" -d "{\"languageId\":\"$EN\",\"levelId\":\"$ADV\",\"stars\":4}" $API/api/v1/tenant/talents/$T/languages)" 204
check "feedback del reclutador" "$(code -X PUT -H "$R" -H "$J" -d '{"stars":5,"comment":"Excelente entrevista"}' $API/api/v1/tenant/talents/$T/feedback)" 200
check "promedio de estrellas" "$(body | jget .avgRating)" 5
check "admin ve la ficha completa" "$(code -H "$A" $API/api/v1/tenant/talents/$T)" 200
check "correo descifrado" "$(body | jget .email)" "$EMAIL"
check "tarifas visibles para admin" "$(body | jget .rates.payrollMax)" 8000
check "3 habilidades técnicas" "$(body | jget .techSkills.length)" 3
check "versión incrementada" "$( [ "$(body | jget .version)" -gt 5 ] && echo si)" si
FB=$(body | jget '.feedback[0].id')
check "reclutador ve la ficha" "$(code -H "$R" $API/api/v1/tenant/talents/$T)" 200
check "reclutador NO ve tarifas" "$(body | jget .rates)" ""
check "admin no puede editar feedback ajeno" "$(code -X PUT -H "$A" -H "$J" -d "{\"id\":\"$FB\",\"stars\":1,\"comment\":\"x\"}" $API/api/v1/tenant/talents/$T/feedback)" 403

echo "== Búsqueda"
check "buscar por nombre" "$(srch "$R" "{\"search\":\"quispe\"}")" 200
check "encontrado" "$(body | jget ".items.some(i=>i.id==='$T')")" true
check "filtro skills Java+React" "$(srch "$R" "{\"skills\":[\"$JAVA\",\"$REACT\"]}")" 200
check "encontrado con ambas" "$(body | jget ".items.some(i=>i.id==='$T')")" true
check "filtro inglés intermedio+" "$(srch "$R" "{\"englishLevel\":\"INTERMEDIATE\",\"search\":\"quispe\"}")" 200
check "incluido (avanzado >= intermedio)" "$(body | jget ".items.some(i=>i.id==='$T')")" true
check "filtro inglés nativo" "$(srch "$R" "{\"englishLevel\":\"NATIVE\",\"search\":\"quispe\"}")" 200
check "excluido" "$(body | jget ".items.some(i=>i.id==='$T')")" false
check "filtro años >= 5" "$(srch "$R" "{\"minYears\":5,\"search\":\"quispe\"}")" 200
check "incluido por experiencia" "$(body | jget ".items.some(i=>i.id==='$T')")" true
check "filtro educación" "$(srch "$R" "{\"education\":\"sistemas\",\"gradeId\":\"$DEG\",\"search\":\"quispe\"}")" 200
check "incluido por educación" "$(body | jget ".items.some(i=>i.id==='$T')")" true
check "uuid de skill inválido" "$(srch "$R" "{\"skills\":[\"1 OR 1\"]}")" 422
check "GET con filtros en la URL ya no existe" "$(code -H "$R" "$API/api/v1/tenant/talents?search=quispe")" 405

echo "== Favoritos"
C=$(uuid)
check "crear colección" "$(code -X POST -H "$R" -H "$J" -d "{\"id\":\"$C\",\"name\":\"Java senior $RANDOM\"}" $API/api/v1/tenant/collections)" 201
check "agregar a favoritos" "$(code -X PUT -H "$R" $API/api/v1/tenant/collections/$C/talents/$T)" 204
check "filtrar por colección" "$(srch "$R" "{\"collectionId\":\"$C\"}")" 200
check "solo ese talento" "$(body | jget .total)" 1
check "otro usuario no usa mi colección" "$(srch "$A" "{\"collectionId\":\"$C\"}")" 404

echo "== Favoritos compartidos"
code -H "$A" $API/api/v1/auth/me >/dev/null; AID=$(body | jget .id); AEM=$(body | jget .email)
check "sin término no expone el directorio" "$(code -H "$R" $API/api/v1/tenant/colleagues)" 200
check "lista vacía sin búsqueda" "$(body | jget .length)" 0
check "buscar usuario por correo" "$(code -H "$R" "$API/api/v1/tenant/colleagues?q=${AEM%%@*}")" 200
check "máximo 10 resultados" "$(body | jget '.length<=10')" true
check "el admin aparece en la búsqueda" "$(body | jget ".some(u=>u.id==='$AID')")" true
check "compartir con permiso VER" "$(code -X PUT -H "$R" -H "$J" -d '{"permission":"VIEW"}' $API/api/v1/tenant/collections/$C/shares/$AID)" 200
code -H "$A" $API/api/v1/tenant/collections >/dev/null
check "el compartido ve la colección (VIEW)" "$(body | jget ".find(c=>c.id==='$C').access")" VIEW
check "el compartido filtra por la colección" "$(srch "$A" "{\"collectionId\":\"$C\"}")" 200
check "ve sus talentos" "$(body | jget .total)" 1
check "VER no permite quitar talentos" "$(code -X DELETE -H "$A" $API/api/v1/tenant/collections/$C/talents/$T)" 403
check "el compartido no puede re-compartir" "$(code -X PUT -H "$A" -H "$J" -d '{"permission":"EDIT"}' $API/api/v1/tenant/collections/$C/shares/$AID)" 403
check "el compartido no puede renombrar" "$(code -X PATCH -H "$A" -H "$J" -d '{"name":"x"}' $API/api/v1/tenant/collections/$C)" 403
check "el dueño sube a EDITAR" "$(code -X PUT -H "$R" -H "$J" -d '{"permission":"EDIT"}' $API/api/v1/tenant/collections/$C/shares/$AID)" 200
check "EDITAR permite quitar talento" "$(code -X DELETE -H "$A" $API/api/v1/tenant/collections/$C/talents/$T)" 204
check "y volver a agregarlo" "$(code -X PUT -H "$A" $API/api/v1/tenant/collections/$C/talents/$T)" 204
check "el dueño ve con quién compartió" "$(code -H "$R" $API/api/v1/tenant/collections/$C/shares)" 200
check "1 persona" "$(body | jget .length)" 1
check "el compartido sale de la colección" "$(code -X DELETE -H "$A" $API/api/v1/tenant/collections/$C)" 204
check "ya no la ve" "$(srch "$A" "{\"collectionId\":\"$C\"}")" 404
check "la colección sigue existiendo para el dueño" "$(srch "$R" "{\"collectionId\":\"$C\"}")" 200

echo "== Archivos (cifrados en el volumen)"
mkdir -p tmp
printf '%%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%%%EOF\n' > tmp/cv_smoke.pdf
check "subir CV PDF" "$(code -X POST -H "$A" -F "file=@tmp/cv_smoke.pdf;type=application/pdf" "$API/api/v1/tenant/talents/$T/files?category=CV")" 201
FID=$(body | jget .id)
check "descargar CV" "$(code -H "$R" $API/api/v1/tenant/talents/files/$FID)" 200
check "contenido íntegro" "$(cmp -s /tmp/st_body tmp/cv_smoke.pdf && echo igual)" igual
check "en disco está cifrado" "$(grep -rl --include='*.enc' 'PDF-1.4' data/files >/dev/null 2>&1 && echo plano || echo cifrado)" cifrado
printf '<html><script>alert(1)</script></html>' > tmp/fake.pdf
check "HTML disfrazado de PDF rechazado" "$(code -X POST -H "$A" -F "file=@tmp/fake.pdf;type=application/pdf" "$API/api/v1/tenant/talents/$T/files?category=CV")" 422
check "sin token no descarga" "$(code $API/api/v1/tenant/talents/files/$FID)" 401

echo "== Offline / idempotencia"
OP=$(uuid)
check "operación con X-Client-Op-Id" "$(code -X PUT -H "$A" -H "$J" -H "X-Client-Op-Id: $OP" -d "{\"skillId\":\"$JAVA\",\"years\":9}" $API/api/v1/tenant/talents/$T/tech-skills)" 204
check "reenvío marcado como replay" "$(curl -s -o /dev/null -D - -X PUT -H "$A" -H "$J" -H "X-Client-Op-Id: $OP" -d "{\"skillId\":\"$JAVA\",\"years\":1}" $API/api/v1/tenant/talents/$T/tech-skills | grep -ci 'x-idempotent-replay: true')" 1
code -H "$A" $API/api/v1/tenant/talents/$T >/dev/null
check "el reenvío no se aplicó (sigue 9 años)" "$(body | jget ".techSkills.find(s=>s.skillId==='$JAVA').years")" 9
V=$(body | jget .version)
check "edición con versión vieja => conflicto" "$(code -X PUT -H "$A" -H "$J" -d "{\"firstName\":\"Ana\",\"lastName\":\"Quispe\",\"email\":\"$EMAIL\",\"expectedVersion\":1}" $API/api/v1/tenant/talents/$T)" 409
check "edición con versión actual" "$(code -X PUT -H "$A" -H "$J" -d "{\"firstName\":\"Ana María\",\"lastName\":\"Quispe\",\"email\":\"$EMAIL\",\"expectedVersion\":$V}" $API/api/v1/tenant/talents/$T)" 200

echo "== Aislamiento entre tenants"
SLUG="tal-$RANDOM$RANDOM"
code -X POST -H "Authorization: Bearer $SA" -H "$J" -d "{\"slug\":\"$SLUG\",\"name\":\"Otro\",\"adminEmail\":\"a@$SLUG.test\"}" $API/api/v1/platform/tenants >/dev/null
OTOK=$(body | jget .adminInvitation.link | sed 's#.*/invitacion/##'); OID=$(body | jget .tenant.id)
code -H "$J" -d '{"fullName":"Otro Admin","password":"Otro-Tenant-123!x","locale":"es"}' $API/api/v1/invitations/$OTOK/accept >/dev/null
code -H "$J" -d "{\"tenant\":\"$SLUG\",\"email\":\"a@$SLUG.test\",\"password\":\"Otro-Tenant-123!x\"}" $API/api/v1/auth/login >/dev/null
O="Authorization: Bearer $(body | jget .accessToken)"
check "otro tenant no ve el talento" "$(code -H "$O" $API/api/v1/tenant/talents/$T)" 404
check "otro tenant no descarga el CV" "$(code -H "$O" $API/api/v1/tenant/talents/files/$FID)" 404
check "otro tenant no modifica el talento" "$(code -X PUT -H "$O" -H "$J" -d '{"firstName":"Hack","lastName":"X"}' $API/api/v1/tenant/talents/$T)" 403
check "no se comparte con usuarios de otro tenant" "$(code -X PUT -H "$R" -H "$J" -d "{\"permission\":\"VIEW\"}" $API/api/v1/tenant/collections/$C/shares/$(curl -s -H "$O" $API/api/v1/auth/me | node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>console.log(JSON.parse(d).id))"))" 422
check "otro tenant no ve en búsqueda" "$(srch "$O" "{\"search\":\"quispe\"}")" 200
check "lista vacía" "$(body | jget .total)" 0

echo "== Edición rápida (disponibilidad y equipo)"
check "cambiar disponibilidad" "$(code -X PATCH -H "$A" -H "$J" -d "{\"availabilityId\":\"$M1\"}" $API/api/v1/tenant/talents/$T/availability)" 204
check "disponibilidad guardada" "$(code -H "$A" $API/api/v1/tenant/talents/$T >/dev/null; body | jget .availabilityId)" "$M1"
check "disponibilidad inválida" "$(code -X PATCH -H "$A" -H "$J" -d "{\"availabilityId\":\"$PE\"}" $API/api/v1/tenant/talents/$T/availability)" 422
check "quitar equipo propio" "$(code -X PATCH -H "$A" -H "$J" -d '{"hasEquipment":false}' $API/api/v1/tenant/talents/$T/equipment)" 204
check "equipo guardado" "$(code -H "$A" $API/api/v1/tenant/talents/$T >/dev/null; body | jget .hasEquipment)" false
check "otro tenant no cambia la disponibilidad" "$(code -X PATCH -H "$O" -H "$J" -d '{"availabilityId":null}' $API/api/v1/tenant/talents/$T/availability)" 404

code -X PATCH -H "Authorization: Bearer $SA" -H "$J" -d '{"status":"CANCELLED"}' $API/api/v1/platform/tenants/$OID/status >/dev/null
echo "== Archivar"
check "archivar talento" "$(code -X PATCH -H "$A" -H "$J" -d '{"status":"ARCHIVED"}' $API/api/v1/tenant/talents/$T/status)" 204
check "archivado no sale en búsqueda activa" "$(srch "$R" "{\"search\":\"quispe\"}")" 200
check "no aparece" "$(body | jget ".items.some(i=>i.id==='$T')")" false
echo "== Resultado: $pass OK, $fail FAIL"
[ $fail -eq 0 ]
