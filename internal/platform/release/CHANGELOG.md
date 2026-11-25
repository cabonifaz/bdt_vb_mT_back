# Registro de versiones del Banco de Talentos

Formato: `## vMAYOR.MENOR — AAAA-MM-DD`, seguido de los cambios en viñetas (español). Al publicar una versión se
sube `VERSION` y se agrega su sección aquí: la API la registra al desplegarse en cada ambiente y la notifica.

## v1.18 — 2026-11-25
- Los movimientos registrados en IGDT (traspaso de área, cambio de cargo, jornada, sede, fecha de fin o remuneración) quedan en el historial del candidato.

## v1.17 — 2026-11-25
- Cambio de RQ desde IGDT: cuando IGDT pide llevar a un talento a otro requerimiento (uno o varios), BDT lo agrega como candidato en «Observado» en cada requerimiento destino abierto (si no estaba ya), indica en su tarjeta y en su ficha «Traído desde IGDT (cambio de RQ desde RQ-…)» y avisa a los responsables del requerimiento y a los jefes de selección. Un talento puede estar en varios requerimientos a la vez.
- Fin de contrato desde IGDT: el cese marca la baja como efectiva con su fecha, el candidato se ve como «Cesó» y el talento vuelve a estar disponible (si estaba «No disponible»). Las renovaciones y los cambios de área se registran en la línea de tiempo del candidato, sin cambiar el pipeline.

## v1.16 — 2026-11-25
- Gestor dirigido: al confirmar a un candidato, junto a la fecha tentativa de inicio, Selección puede elegir «Dirigir a gestor (IGDT)». Por defecto la solicitud de ingreso va a los gestores del cliente; también se puede dirigir a cualquier otro gestor de IGDT. El selector muestra primero los gestores del cliente del requerimiento y sugiere el último gestor elegido en el mismo requerimiento.
- El gestor dirigido se puede cambiar junto con la fecha mientras IGDT no haya registrado el ingreso; IGDT recibe el cambio automáticamente y queda en la línea de tiempo del postulante.
- Si IGDT no responde, el selector ofrece solo «Gestores del cliente» y la confirmación no se bloquea.

## v1.15 — 2026-11-25
- Al confirmar a un candidato en el pipeline del requerimiento, Selección debe registrar la fecha tentativa de inicio de labores (no anterior a 30 días atrás ni posterior a un año). La fecha se ve en la tarjeta del candidato confirmado y se puede corregir mientras IGDT no haya registrado el ingreso; cada cambio queda en la línea de tiempo del postulante.
- La solicitud de ingreso que BDT envía a IGDT incluye la fecha tentativa de inicio, y se vuelve a enviar cada vez que la fecha cambia. Los candidatos confirmados antes de esta versión quedan sin fecha hasta que Selección la registre.
- Técnico: en staging, los correos salientes pueden redirigirse a una sola dirección de pruebas (MAIL_REDIRECT_TO), con el asunto marcado [STAGING] y el destinatario original al inicio del texto.

## v1.14 — 2026-11-25
- Las solicitudes de ingreso que BDT envía a IGDT incluyen ahora a los solicitantes de la ficha: el nombre y correo del usuario que confirmó el ingreso y el nombre y correo del analista responsable del requerimiento (el responsable vigente asignado primero), para que IGDT les envíe las fichas de ingreso.
- Las solicitudes de ingreso vigentes ya enviadas se vuelven a enviar automáticamente con estos datos.

## v1.13 — 2026-11-24
- Las invitaciones se envían por correo con Resend desde no-reply@geeky-tech.es. Si el correo no sale, la invitación igual queda creada y se informa el motivo.

## v1.12 — 2026-11-23
- API de integración (IGDT): la ficha y los insumos del CV respetan siempre el mismo orden que BDT (habilidades técnicas de más a menos años, experiencia y estudios del más reciente al más antiguo, idiomas y archivos), para que el CV generado en IGDT sea idéntico al de BDT.

## v1.11 — 2026-11-12
- IGDT puede mostrar la pantalla de talentos igual que BDT: la búsqueda de integración filtra también por habilidades del catálogo (el talento debe tener todas las elegidas), por grado académico y por talentos archivados, e indica para cada talento su foto y si está en la lista negra (sin motivos ni clientes). Los nuevos filtros quedan en la auditoría de cada búsqueda.
- La ficha de integración incluye la foto del talento y si está en la lista negra, y la foto se puede descargar como los demás archivos (cada descarga queda en la auditoría).
- La llave automática de IGDT incluye el permiso de lectura de catálogos (habilidades, niveles de idioma y grados académicos), también las ya entregadas.

## v1.10 — 2026-11-11
- Insumos del CV para IGDT: cuando la empresa no tiene cabecera de CV ni logo subido, se entrega el logo fijo de la empresa (el mismo que usa BDT en sus CV), para que el CV generado en IGDT tenga el mismo formato que en BDT.

## v1.9 — 2026-11-10
- Nuevo permiso de API «Buscar en todo el banco de talentos (solo lectura)». Con él, IGDT puede listar y buscar todos los talentos activos de la empresa (por nombre o titular, puesto, estudios, habilidad, nivel de inglés y años de experiencia), sin datos personales, tarifas ni lista negra. Cada búsqueda queda en la auditoría.
- Con ese permiso, la ficha, los CV, los documentos y los insumos del CV de la API de integración están disponibles para cualquier talento de la empresa, no solo para quienes postulan a un requerimiento.
- La llave automática de IGDT incluye ese permiso (también las ya entregadas).

## v1.8 — 2026-11-10
- La API de integración entrega también los documentos y certificados del candidato (en la ficha y para descargar), además de sus CV. Cada descarga queda en la auditoría.
- Nueva consulta de integración con los insumos del CV con la marca de la empresa (datos del candidato, plantilla con cabecera, pie, logo y foto, y en inglés los textos traducidos con IA), para que IGDT genere el mismo CV que BDT aunque no se haya guardado.

## v1.7 — 2026-11-10
- Al conectar la empresa con IGDT, BDT le entrega automáticamente una llave de solo lectura («IGDT (automática)») para que IGDT consulte la ficha, los CV y las postulaciones de los candidatos. Si IGDT no la recibe, se reintenta sola; al desconectar, la llave se revoca.
- Nueva API de integración: ficha completa del candidato y descarga de sus CV (original y formato empresa), solo para talentos que postulan en algún requerimiento de la empresa. Cada consulta queda en la auditoría.
- Los cambios de fecha de inicio registrados en IGDT se reflejan en el candidato y en su línea de tiempo.
- Si el cliente revoca un ingreso en IGDT, el candidato deja de figurar como ingresado y se avisa a los responsables del RQ y a quienes pueden reabrirlo (el RQ no se reabre solo).
- La línea de tiempo del postulante muestra el ingreso registrado en IGDT con su fecha de inicio.

## v1.6 — 2026-11-09
- IGDT recibe la fecha de registro del requerimiento y la configuración de alertas de RQ sin movimiento, para mostrar las mismas vistas y etiquetas del tablero. Al cambiar los días de alerta se reenvían los requerimientos abiertos.

## v1.5 — 2026-11-09
- Con el ingreso registrado en IGDT, el candidato aparece como «Ingresó» en el pipeline y ya no se puede mover ni retirar.
- El requerimiento abierto que alcanza tantos ingresos como vacantes se cierra solo como Atendido.

## v1.4 — 2026-11-09
- Con la empresa conectada a IGDT, los requerimientos y sus postulantes se publican en IGDT al momento, y cada candidato confirmado crea allá una solicitud de ingreso para el gestor de servicios (se cancela si se retira la confirmación).
- La pantalla de integración muestra el estado de esos envíos; los que fallan se reintentan solos.
- Corrección: al elegir el perfil de una vacante, la tarifa sugerida ya no borra la cantidad escrita mientras tanto.

## v1.3 — 2026-11-05
- Integración con IGDT, el maestro único de catálogos: con la empresa conectada, clientes y contactos, perfiles, tarifario, modalidades, monedas, plazos, áreas y motivos se administran en IGDT y llegan solos a BDT.
- Al crear una vacante se propone la tarifa del tarifario de IGDT.

## v1.2 — 2026-11-04
- Indicadores de selección para el jefe de selección y el administrador: vencidos con y sin postulantes, postulantes, entrevistas, horas y días por ingreso, tiempo hasta cerrar, cierres a tiempo, altas y reuniones por analista (semanas o meses), embudo, postulantes en varios procesos, clientes y perfiles.
- Las alertas de RQ sin movimiento se cuentan en días hábiles (lunes a viernes).

## v1.1 — 2026-11-04
- Alertas de RQ sin movimiento: vista «Sin movimiento» en el tablero, etiqueta en la tarjeta, aviso en la ficha y días en etapa de cada postulante.
- Notificación a los responsables del RQ y a los jefes de selección (resumen si son varios); días configurables por empresa.
- Las guías paso a paso ya no se superponen con la pantalla y la versión se ve completa bajo el logo.

## v1.0 — 2026-11-02
- Banco de talentos multiempresa con búsqueda, filtros, ficha y CV con la marca de la empresa.
- Carga rápida y relectura de CV con IA (años por tecnología) y consulta de DNI, sin talentos duplicados.
- Requerimientos en Kanban con vistas (abiertos, por vencer, vencidos, nuevos, cerrados), filtros y varios responsables.
- Pipeline de candidatos, reapertura por renuncia o más vacantes y rol Jefe de selección.
- Documentos de postulación con validación manual y validación automática del CUL contra el MTPE.
- Soporte al Super Admin desde cualquier pantalla, novedades, guías de primer uso y manual por rol.
- Catálogo de funcionalidades activable por empresa y control de versiones por ambiente.
