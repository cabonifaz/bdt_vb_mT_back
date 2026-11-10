# Registro de versiones del Banco de Talentos

Formato: `## vMAYOR.MENOR — AAAA-MM-DD`, seguido de los cambios en viñetas (español). Al publicar una versión se
sube `VERSION` y se agrega su sección aquí: la API la registra al desplegarse en cada ambiente y la notifica.

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
