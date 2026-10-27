#!/usr/bin/env bash
# Datos de demostración para STAGING: un admin del tenant y talentos de ejemplo.
# Uso: SA_EMAIL=... SA_PASSWORD=... ADMIN_EMAIL=... ADMIN_PASSWORD=... ./scripts/seed_demo.sh [api] [tenant]
set -eu
API=${1:-http://localhost:8080}
TENANT=${2:-fractal}
J="Content-Type: application/json"
# el cuerpo va por archivo: en Windows los argumentos de línea de comandos pierden el UTF-8
BODY=$(mktemp)
trap 'rm -f "$BODY"' EXIT
post() { printf '%s' "$4" > "$BODY"; curl -s -X "$1" -H "$J" ${3:+-H "Authorization: Bearer $3"} --data-binary "@$(cygpath -w "$BODY" 2>/dev/null || echo "$BODY")" "$API$2"; }
jget() { node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const o=JSON.parse(d);console.log(eval('o'+process.argv[1])??'')})" "$1"; }

SA=$(post POST /api/v1/auth/login "" "{\"email\":\"$SA_EMAIL\",\"password\":\"$SA_PASSWORD\"}" | jget .accessToken)
TID=$(curl -s -H "Authorization: Bearer $SA" "$API/api/v1/platform/tenants?search=$TENANT" | jget ".items.find(t=>t.slug==='$TENANT').id")

# 1) admin del tenant (si ya existe, se omite)
LOGIN=$(post POST /api/v1/auth/login "" "{\"tenant\":\"$TENANT\",\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}")
AD=$(echo "$LOGIN" | jget .accessToken)
if [ -z "$AD" ]; then
  LINK=$(post POST "/api/v1/platform/invitations?tenantId=$TID" "$SA" "{\"email\":\"$ADMIN_EMAIL\",\"role\":\"TENANT_ADMIN\",\"locale\":\"es\"}" | jget .link)
  [ -n "$LINK" ] || { echo "no se pudo invitar al admin"; exit 1; }
  post POST "/api/v1/invitations/${LINK##*/}/accept" "" "{\"fullName\":\"Administrador $TENANT\",\"password\":\"$ADMIN_PASSWORD\",\"locale\":\"es\"}" >/dev/null
  AD=$(post POST /api/v1/auth/login "" "{\"tenant\":\"$TENANT\",\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" | jget .accessToken)
  echo "admin creado: $ADMIN_EMAIL"
else
  echo "admin ya existía: $ADMIN_EMAIL"
fi

# 2) talentos de demostración
M=$(curl -s -H "Authorization: Bearer $AD" "$API/api/v1/masters/items?tables=LANGUAGE,LANGUAGE_LEVEL,TECH_SKILL,SOFT_SKILL,COUNTRY,ACADEMIC_GRADE,CURRENCY,DOC_IDENTITY_TYPE")
mid() { echo "$M" | node -e "let d='';process.stdin.on('data',c=>d+=c).on('end',()=>{const a=JSON.parse(d);const i=a.find(i=>i.table===process.argv[1]&&i.code===process.argv[2]);console.log(i?i.id:'')})" "$1" "$2"; }
EN=$(mid LANGUAGE EN); PE=$(mid COUNTRY PE); CO=$(mid COUNTRY CO); PEN=$(mid CURRENCY PEN); DEG=$(mid ACADEMIC_GRADE DEGREE); BACH=$(mid ACADEMIC_GRADE BACHELOR)

#   nombre|apellido|apellido2|puesto|país|ciudad|skills(code:años,...)|soft|nivelEN|empresa|cargo|inicio|carrera|institución|planillaMin|planillaMax|estrellas
DATA=(
"Ana|Quispe|Rojas|Desarrolladora Java Senior|PE|Lima|JAVA:8,SPRING:6,SQL:7,AWS:3|TEAMWORK,COMMUNICATION|ADVANCED|Banco Andino|Tech Lead Java|2019-03-01|Ingeniería de Sistemas|UNI|9000|11000|5"
"Luis|Mamani|Torres|Frontend React|PE|Arequipa|REACT:5,TYPESCRIPT:4,JAVASCRIPT:6|PROACTIVITY,ADAPTABILITY|INTERMEDIATE|Retail Digital|Frontend Developer|2020-06-01|Ingeniería de Software|UNSA|6000|7500|4"
"María|Gonzales|Paredes|Analista QA|PE|Lima|QA:6,SQL:4,SCRUM:3|COMMUNICATION,TIME_MANAGEMENT|BASIC|Seguros del Sur|QA Analyst|2018-01-15|Ingeniería Industrial|PUCP|5000|6500|4"
"Jorge|Rodríguez|Vargas|Arquitecto Cloud|CO|Bogotá|AWS:7,AZURE:4,KUBERNETES:5,DOCKER:6,GO:3|LEADERSHIP,PROBLEM_SOLVING|NATIVE|CloudOps|Cloud Architect|2016-09-01|Ingeniería Electrónica|U. Nacional de Colombia|12000|15000|5"
"Carla|Huamán|Díaz|Desarrolladora .NET|PE|Trujillo|DOTNET:5,SQLSERVER:5,ANGULAR:3|TEAMWORK|INTERMEDIATE|Gobierno Regional|Desarrolladora .NET|2019-11-01|Ingeniería de Computación|UPAO|5500|7000|3"
"Diego|Flores|Ramos|Fullstack Node / React|PE|Lima|NODE:4,REACT:4,MYSQL:3,DOCKER:2|PROACTIVITY,COMMUNICATION|ADVANCED|Startup Pagos|Fullstack Developer|2021-02-01|Computación|UPC|6500|8000|4"
"Valeria|Castro|Mendoza|Data Engineer Python|PE|Cusco|PYTHON:5,SQL:6,AWS:2|PROBLEM_SOLVING,ADAPTABILITY|ADVANCED|Analytics Perú|Data Engineer|2020-04-01|Estadística|UNSAAC|7000|9000|5"
"Renzo|Salazar|Cruz|Scrum Master|PE|Lima|SCRUM:6,JAVA:2|LEADERSHIP,COMMUNICATION,TIME_MANAGEMENT|INTERMEDIATE|Telco Andina|Scrum Master|2017-08-01|Administración|U. de Lima|8000|10000|4"
)
n=0
for row in "${DATA[@]}"; do
  IFS='|' read -r FN LN LN2 HL CC CITY SKILLS SOFT ENL COMP POS START CAR INST PMIN PMAX STARS <<<"$row"
  # el mismo talento demo no se duplica: uuid determinista por nombre
  ID=$(node -e "const h=require('crypto').createHash('sha1').update('demo:'+process.argv[1]).digest('hex');console.log(h.slice(0,8)+'-'+h.slice(8,12)+'-5'+h.slice(13,16)+'-a'+h.slice(17,20)+'-'+h.slice(20,32))" "$TENANT$FN$LN")
  CID=$(mid COUNTRY "$CC")
  EMAIL=$(echo "$FN.$LN" | iconv -f utf-8 -t ascii//TRANSLIT 2>/dev/null | tr 'A-Z' 'a-z' | tr -cd 'a-z.')@demo.test
  post POST /api/v1/tenant/talents "$AD" "{\"id\":\"$ID\",\"firstName\":\"$FN\",\"lastName\":\"$LN\",\"secondLastName\":\"$LN2\",\"headline\":\"$HL\",\"email\":\"$EMAIL\",\"phone\":\"9$((10000000 + RANDOM*100))\",\"countryId\":\"$CID\",\"city\":\"$CITY\",\"summary\":\"Perfil de demostración. $HL con experiencia en proyectos de transformación digital.\",\"availability\":\"Inmediata\",\"hasEquipment\":true,\"linkedinUrl\":\"https://www.linkedin.com/in/demo\",\"rates\":{\"payrollMin\":$PMIN,\"payrollMax\":$PMAX,\"payrollCurrencyId\":\"$PEN\"}}" >/dev/null
  for s in ${SKILLS//,/ }; do
    post PUT "/api/v1/tenant/talents/$ID/tech-skills" "$AD" "{\"skillId\":\"$(mid TECH_SKILL "${s%%:*}")\",\"years\":${s##*:}}" >/dev/null
  done
  for s in ${SOFT//,/ }; do
    post POST "/api/v1/tenant/talents/$ID/soft-skills" "$AD" "{\"skillId\":\"$(mid SOFT_SKILL "$s")\"}" >/dev/null
  done
  post PUT "/api/v1/tenant/talents/$ID/languages" "$AD" "{\"languageId\":\"$EN\",\"levelId\":\"$(mid LANGUAGE_LEVEL "$ENL")\",\"stars\":3}" >/dev/null
  post PUT "/api/v1/tenant/talents/$ID/experiences" "$AD" "{\"id\":\"${ID:0:24}000000000001\",\"company\":\"$COMP\",\"position\":\"$POS\",\"startDate\":\"$START\",\"isCurrent\":true,\"functions\":\"- Diseño y desarrollo de soluciones\\n- Trabajo con equipos ágiles\"}" >/dev/null
  post PUT "/api/v1/tenant/talents/$ID/educations" "$AD" "{\"id\":\"${ID:0:24}000000000002\",\"institution\":\"$INST\",\"career\":\"$CAR\",\"gradeId\":\"$DEG\",\"startDate\":\"2008-03-01\",\"endDate\":\"2013-12-01\"}" >/dev/null
  post PUT "/api/v1/tenant/talents/$ID/feedback" "$AD" "{\"id\":\"${ID:0:24}000000000003\",\"stars\":$STARS,\"comment\":\"Entrevista técnica de demostración.\"}" >/dev/null
  n=$((n+1))
done
echo "talentos demo cargados: $n"
