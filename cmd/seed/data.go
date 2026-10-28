package main

// Datos de referencia para generar información de prueba realista (Perú): nombres, rubros, perfiles, clientes,
// skills, empresas, instituciones y ciudades. Todo es ficticio salvo nombres de empresas usados como clientes de ejemplo.

var maleNames = []string{"José", "Luis", "Carlos", "Jorge", "Juan", "Miguel", "César", "Víctor", "Javier", "Pedro", "Raúl", "Daniel",
	"Fernando", "Ricardo", "Eduardo", "Alberto", "Óscar", "Martín", "Diego", "Andrés", "Christian", "Renzo", "Gustavo", "Wilmer",
	"Edwin", "Iván", "Hugo", "Manuel", "Roberto", "Julio", "Alonso", "Sebastián", "Rodrigo", "Kevin", "Bruno", "Franco", "Elmer",
	"Percy", "Walter", "Freddy", "Néstor", "Rubén", "Alex", "Jhon", "Henry", "Marco", "Paolo", "Gerson", "Yuri", "Abel"}

var femaleNames = []string{"María", "Rosa", "Ana", "Carmen", "Lucía", "Patricia", "Gabriela", "Milagros", "Karina", "Silvia",
	"Elizabeth", "Jessica", "Claudia", "Mónica", "Diana", "Andrea", "Vanessa", "Katherine", "Fiorella", "Valeria", "Daniela",
	"Paola", "Cinthya", "Lorena", "Sandra", "Roxana", "Gladys", "Yesenia", "Melissa", "Stephany", "Camila", "Alejandra", "Nataly",
	"Ruth", "Erika", "Kelly", "Jimena", "Mariela", "Pilar", "Yolanda", "Liliana", "Susana", "Flor", "Noemí", "Wendy"}

var middleNames = []string{"Alberto", "Antonio", "Enrique", "Manuel", "Francisco", "Alejandro", "Rafael", "Augusto", "Isabel",
	"Del Pilar", "Esther", "Elena", "Victoria", "Teresa", "Beatriz", "Cecilia", "Jesús", "Fernando", "Martín", "Eduardo"}

var surnames = []string{"Quispe", "Flores", "Sánchez", "Rodríguez", "García", "Rojas", "Mamani", "Huamán", "Vásquez", "Chávez",
	"Ramírez", "Torres", "Mendoza", "Castillo", "Díaz", "Espinoza", "Gutiérrez", "Romero", "Vargas", "Cruz", "Pérez", "Ramos",
	"Salazar", "Morales", "Herrera", "Medina", "Cárdenas", "Paredes", "Zapata", "Ríos", "Córdova", "Benites", "Gonzales", "Aguilar",
	"Condori", "Choque", "Ccama", "Apaza", "Huanca", "Ticona", "Villanueva", "Palomino", "Alarcón", "Navarro", "Cabrera", "Delgado",
	"Valdivia", "Linares", "Peña", "Tello", "Arce", "Ponce", "Lazo", "Málaga", "Bustamante", "Zegarra", "Pacheco", "Carbajal",
	"Huerta", "Saavedra", "Soto", "Campos", "Inga", "Yupanqui", "Cóndor", "Ttito", "Ayala", "Loayza", "Oré", "Alvarado"}

var emailDomains = []string{"gmail.com", "gmail.com", "gmail.com", "hotmail.com", "outlook.com", "yahoo.es"}

var softSkillCodes = []string{"TEAMWORK", "COMMUNICATION", "LEADERSHIP", "PROBLEM_SOLVING", "ADAPTABILITY", "PROACTIVITY"}

// Profile: perfil de vacante con sus skills típicas, puestos para la experiencia y rango salarial mensual (PEN).
type Profile struct {
	Code, Name  string
	Headlines   []string
	Skills      []string
	Positions   []string
	Functions   []string
	Careers     []string
	SalaryMin   float64
	SalaryMax   float64
	QtyMax      int // vacantes por RQ de este perfil
	TechnicalEd bool
}

// Industry: rubro del tenant (perfiles, clientes, empresas para la experiencia, instituciones y ciudades).
type Industry struct {
	Profiles     []Profile
	Clients      []Client
	Companies    []string
	Institutions []string
	Cities       []string
}

type Client struct{ Name, RUC, Address string }

// TenantSpec: tenant a generar.
type TenantSpec struct {
	Slug, Name, Legal, RUC string
	Primary, Secondary     string
	Analysts               int // usuarios con rol reclutador (analistas de selección)
	Commercial             int
	Talents                int
	RQs                    int
	CandMin, CandMax       int // candidatos por RQ
	Industry               *Industry
	Existing               bool // tenant que ya existe (Fractal)
}

var itIndustry = &Industry{
	Profiles: []Profile{
		{Code: "JAVA_DEV", Name: "Desarrollador Java", Headlines: []string{"Desarrollador Java Senior", "Desarrollador Backend Java", "Analista Programador Java"},
			Skills: []string{"Java", "Spring Boot", "SQL", "Microservicios", "Docker", "Git", "Oracle"}, Positions: []string{"Desarrollador Java", "Analista Programador", "Desarrollador Backend"},
			Functions: []string{"Desarrollo de microservicios con Spring Boot", "Integración de APIs REST con sistemas core", "Optimización de consultas SQL y procedimientos almacenados", "Pruebas unitarias con JUnit y revisión de código"},
			Careers:   []string{"Ingeniería de Sistemas", "Ingeniería de Software", "Ingeniería Informática"}, SalaryMin: 4500, SalaryMax: 11000, QtyMax: 3},
		{Code: "FRONTEND_DEV", Name: "Desarrollador Frontend", Headlines: []string{"Desarrollador Frontend React", "Desarrollador Angular", "Frontend Developer"},
			Skills: []string{"React", "Angular", "TypeScript", "JavaScript", "HTML", "CSS", "Git"}, Positions: []string{"Desarrollador Frontend", "Desarrollador Web"},
			Functions: []string{"Desarrollo de interfaces en React con TypeScript", "Maquetación responsive y accesible", "Consumo de APIs REST y manejo de estado", "Pruebas E2E con Cypress"},
			Careers:   []string{"Ingeniería de Sistemas", "Computación e Informática"}, SalaryMin: 4000, SalaryMax: 9500, QtyMax: 2},
		{Code: "QA_ANALYST", Name: "Analista QA", Headlines: []string{"Analista QA Automatizador", "QA Engineer", "Analista de Calidad de Software"},
			Skills: []string{"Selenium", "Cypress", "Postman", "Jira", "SQL", "Java"}, Positions: []string{"Analista QA", "Tester", "QA Automation"},
			Functions: []string{"Diseño y ejecución de casos de prueba", "Automatización de pruebas con Selenium", "Pruebas de APIs con Postman", "Gestión de incidencias en Jira"},
			Careers:   []string{"Ingeniería de Sistemas", "Computación e Informática"}, SalaryMin: 3500, SalaryMax: 7500, QtyMax: 2},
		{Code: "BUSINESS_ANALYST", Name: "Analista de negocio", Headlines: []string{"Analista de Negocio", "Analista Funcional", "Product Owner"},
			Skills: []string{"Scrum", "Jira", "SQL", "Power BI", "BPMN"}, Positions: []string{"Analista de Negocio", "Analista Funcional"},
			Functions: []string{"Levantamiento de requerimientos con usuarios", "Elaboración de historias de usuario", "Modelado de procesos en BPMN", "Seguimiento del backlog"},
			Careers:   []string{"Ingeniería Industrial", "Ingeniería de Sistemas", "Administración"}, SalaryMin: 4500, SalaryMax: 9000, QtyMax: 1},
		{Code: "PROJECT_MANAGER", Name: "Jefe de proyecto", Headlines: []string{"Jefe de Proyectos TI", "Project Manager", "Scrum Master"},
			Skills: []string{"Scrum", "PMBOK", "Jira", "MS Project", "Gestión de riesgos"}, Positions: []string{"Jefe de Proyecto", "Scrum Master", "Coordinador de TI"},
			Functions: []string{"Planificación y seguimiento de proyectos de software", "Gestión de riesgos y stakeholders", "Facilitación de ceremonias ágiles"},
			Careers:   []string{"Ingeniería de Sistemas", "Ingeniería Industrial"}, SalaryMin: 8000, SalaryMax: 15000, QtyMax: 1},
	},
	Clients: []Client{{"Banco de Crédito del Perú", "20100047218", "Calle Centenario 156, La Molina"}, {"Interbank", "20100053455", "Av. Carlos Villarán 140, La Victoria"},
		{"BBVA Perú", "20100130204", "Av. República de Panamá 3055, San Isidro"}, {"Scotiabank Perú", "20100043140", "Av. Canaval y Moreyra 522, San Isidro"},
		{"Rimac Seguros", "20100041953", "Av. Paseo de la República 3505, San Isidro"}, {"Pacífico Seguros", "20332970411", "Av. Juan de Arona 830, San Isidro"},
		{"Alicorp", "20100055237", "Av. Argentina 4793, Callao"}, {"Telefónica del Perú", "20100017491", "Av. Arequipa 1155, Santa Beatriz"},
		{"Entel Perú", "20106897914", "Av. República de Colombia 791, San Isidro"}, {"Mibanco", "20382036655", "Av. Domingo Orué 165, Surquillo"},
		{"Caja Arequipa", "20100209641", "Calle La Merced 106, Arequipa"}, {"Falabella Perú", "20100128056", "Av. Paseo de la República 3220, San Isidro"}},
	Companies:    []string{"Indra Perú", "NTT Data", "Stefanini", "GMD", "Tata Consultancy Services", "Everis", "Cosapi Data", "Canvia", "IBM Perú", "Accenture", "Belatrix", "Globant", "Kunan", "Inetum"},
	Institutions: []string{"Universidad Nacional de Ingeniería", "Pontificia Universidad Católica del Perú", "Universidad Peruana de Ciencias Aplicadas", "Universidad Nacional Mayor de San Marcos", "Universidad de Lima", "Universidad Tecnológica del Perú", "Universidad San Ignacio de Loyola", "Instituto Cibertec", "IDAT"},
	Cities:       []string{"Lima", "Lima", "Lima", "Lima", "Arequipa", "Trujillo", "Chiclayo", "Cusco", "Piura"},
}

var miningIndustry = &Industry{
	Profiles: []Profile{
		{Code: "DRILLER", Name: "Perforista diamantina", Headlines: []string{"Perforista Diamantino", "Perforista de Aire Reverso", "Operador de Perforadora"},
			Skills: []string{"Perforación diamantina", "Perforación aire reverso", "Equipos Boart Longyear", "IPERC", "Trabajos en altura"}, Positions: []string{"Perforista", "Perforista Líder", "Operador de Perforadora"},
			Functions: []string{"Operación de perforadora diamantina LF-90 y LF-230", "Control de avance y recuperación de testigos", "Mantenimiento básico del equipo de perforación", "Cumplimiento de estándares SSOMA e IPERC"},
			Careers:   []string{"Técnico en Operaciones Mineras", "Mecánica de Mantenimiento"}, SalaryMin: 3800, SalaryMax: 7500, QtyMax: 8, TechnicalEd: true},
		{Code: "DRILL_HELPER", Name: "Ayudante de perforación", Headlines: []string{"Ayudante de Perforación", "Ayudante de Perforista"},
			Skills: []string{"Perforación diamantina", "Manejo de herramientas", "IPERC", "Trabajos en altura"}, Positions: []string{"Ayudante de Perforación", "Ayudante General"},
			Functions: []string{"Apoyo en maniobras de perforación y cambio de barras", "Preparación de lodos y aditivos", "Orden y limpieza de la plataforma", "Registro de testigos en cajas porta testigos"},
			Careers:   []string{"Técnico en Operaciones Mineras"}, SalaryMin: 1800, SalaryMax: 3200, QtyMax: 10, TechnicalEd: true},
		{Code: "HEAVY_MECHANIC", Name: "Mecánico de equipo pesado", Headlines: []string{"Mecánico de Equipo Pesado", "Técnico Mecánico de Mantenimiento"},
			Skills: []string{"Mantenimiento preventivo", "Hidráulica", "Motores diésel", "Lectura de planos", "SAP PM"}, Positions: []string{"Mecánico de Equipo Pesado", "Técnico Mecánico"},
			Functions: []string{"Mantenimiento preventivo y correctivo de perforadoras y compresoras", "Diagnóstico de sistemas hidráulicos", "Registro de órdenes de trabajo en SAP PM"},
			Careers:   []string{"Mecánica de Maquinaria Pesada", "Mantenimiento de Maquinaria Pesada"}, SalaryMin: 3500, SalaryMax: 6500, QtyMax: 4, TechnicalEd: true},
		{Code: "DRILL_SUPERVISOR", Name: "Supervisor de perforación", Headlines: []string{"Supervisor de Perforación", "Jefe de Guardia de Perforación"},
			Skills: []string{"Perforación diamantina", "Gestión de seguridad", "IPERC", "Liderazgo de cuadrillas", "Excel"}, Positions: []string{"Supervisor de Perforación", "Jefe de Guardia"},
			Functions: []string{"Supervisión de la operación de 6 a 10 equipos de perforación", "Control de indicadores de avance y disponibilidad", "Coordinación con geología del cliente", "Liderazgo de cuadrillas en sistema 14x7"},
			Careers:   []string{"Ingeniería de Minas", "Ingeniería Geológica"}, SalaryMin: 7000, SalaryMax: 13000, QtyMax: 2},
		{Code: "HEAVY_OPERATOR", Name: "Operador de equipo pesado", Headlines: []string{"Operador de Camión Grúa", "Operador de Excavadora", "Operador de Equipo Pesado"},
			Skills: []string{"Licencia A-IIIC", "Camión grúa", "Excavadora", "IPERC", "Manejo defensivo"}, Positions: []string{"Operador de Equipo Pesado", "Conductor de Camión Grúa"},
			Functions: []string{"Operación de camión grúa para traslado de equipos de perforación", "Inspección pre-uso del equipo", "Traslado de personal y materiales en mina"},
			Careers:   []string{"Operación de Maquinaria Pesada"}, SalaryMin: 3000, SalaryMax: 5500, QtyMax: 4, TechnicalEd: true},
		{Code: "INDUSTRIAL_ELECTRICIAN", Name: "Electricista industrial", Headlines: []string{"Electricista Industrial", "Técnico Electricista de Mantenimiento"},
			Skills: []string{"Electricidad industrial", "Tableros eléctricos", "Bloqueo y etiquetado", "Mantenimiento preventivo"}, Positions: []string{"Electricista Industrial", "Técnico Electricista"},
			Functions: []string{"Mantenimiento de sistemas eléctricos de grupos electrógenos", "Instalación de tableros y luminarias en campamento", "Aplicación de bloqueo y etiquetado (LOTO)"},
			Careers:   []string{"Electricidad Industrial", "Electrotecnia Industrial"}, SalaryMin: 3200, SalaryMax: 6000, QtyMax: 3, TechnicalEd: true},
		{Code: "HSE_SUPERVISOR", Name: "Supervisor SSOMA", Headlines: []string{"Supervisor de Seguridad SSOMA", "Prevencionista de Riesgos"},
			Skills: []string{"IPERC", "ISO 45001", "Gestión de seguridad", "Investigación de incidentes", "Excel"}, Positions: []string{"Supervisor SSOMA", "Prevencionista de Riesgos"},
			Functions: []string{"Inspecciones de seguridad en plataformas de perforación", "Capacitación en estándares y PETS", "Investigación de incidentes y seguimiento de acciones"},
			Careers:   []string{"Ingeniería de Seguridad y Salud en el Trabajo", "Ingeniería Ambiental", "Ingeniería de Minas"}, SalaryMin: 6000, SalaryMax: 10000, QtyMax: 2},
		{Code: "FIELD_GEOLOGIST", Name: "Geólogo de campo", Headlines: []string{"Geólogo de Exploraciones", "Geólogo de Campo"},
			Skills: []string{"Logueo geológico", "AutoCAD", "Leapfrog", "Muestreo", "Excel"}, Positions: []string{"Geólogo de Campo", "Geólogo Junior"},
			Functions: []string{"Logueo geológico y geotécnico de testigos", "Control de calidad del muestreo", "Elaboración de secciones y planos"},
			Careers:   []string{"Ingeniería Geológica"}, SalaryMin: 5500, SalaryMax: 9500, QtyMax: 2},
		{Code: "WELDER", Name: "Soldador homologado", Headlines: []string{"Soldador Homologado 6G", "Soldador Estructural"},
			Skills: []string{"Soldadura SMAW", "Soldadura MIG", "Lectura de planos", "Trabajos en caliente"}, Positions: []string{"Soldador", "Soldador Homologado"},
			Functions: []string{"Soldadura de estructuras y reparación de componentes", "Fabricación de accesorios para equipos de perforación"},
			Careers:   []string{"Soldadura Estructural", "Construcciones Metálicas"}, SalaryMin: 3000, SalaryMax: 5500, QtyMax: 3, TechnicalEd: true},
	},
	Clients: []Client{{"Compañía Minera Antamina", "20330262428", "Av. El Derby 055, Santiago de Surco"}, {"Southern Peru Copper Corporation", "20100147514", "Av. Caminos del Inca 171, Santiago de Surco"},
		{"Sociedad Minera Cerro Verde", "20170072465", "Asiento Minero Cerro Verde, Uchumayo, Arequipa"}, {"Minera Las Bambas", "20538428524", "Av. El Derby 055, Santiago de Surco"},
		{"Compañía Minera Antapaccay", "20114915026", "Espinar, Cusco"}, {"Volcan Compañía Minera", "20383045267", "Av. Manuel Olguín 375, Santiago de Surco"},
		{"Compañía de Minas Buenaventura", "20100079501", "Calle Las Begonias 415, San Isidro"}, {"Minera Yanacocha", "20137291313", "Av. La Paz 1049, Miraflores"},
		{"Hudbay Perú", "20511165181", "Av. El Derby 250, Santiago de Surco"}, {"Minera Chinalco Perú", "20506675457", "Av. Pardo y Aliaga 640, San Isidro"},
		{"Nexa Resources Perú", "20100110513", "Av. Paseo de la República 3005, San Isidro"}, {"Minsur", "20100136237", "Jr. Lorenzo Bernini 149, San Borja"},
		{"Compañía Minera Poderosa", "20137025354", "Av. Primavera 834, Santiago de Surco"}, {"Anglo American Quellaveco", "20137913250", "Moquegua"}},
	Companies:    []string{"Explomin del Perú", "AK Drilling International", "Boart Longyear Perú", "Geotec", "MDH Perforaciones", "Redrilsa", "Major Drilling", "Perforaciones Andinas", "Sondajes Perú", "Ferreyros", "Unimaq", "Stracon", "San Martín Contratistas", "JJC Contratistas"},
	Institutions: []string{"SENATI", "TECSUP", "Universidad Nacional de San Agustín", "Universidad Nacional de San Antonio Abad del Cusco", "Universidad Nacional del Centro del Perú", "Universidad Nacional de Cajamarca", "Universidad Nacional de Ingeniería", "Instituto Superior Tecnológico Público Jorge Basadre", "SENCICO"},
	Cities:       []string{"Arequipa", "Arequipa", "Cusco", "Cajamarca", "Huancayo", "Trujillo", "Moquegua", "Tacna", "Puno", "Abancay", "Huaraz", "Lima", "Ayacucho", "Juliaca", "Cerro de Pasco"},
}

var financeIndustry = &Industry{
	Profiles: []Profile{
		{Code: "CREDIT_ANALYST", Name: "Analista de créditos", Headlines: []string{"Analista de Créditos", "Asesor de Negocios", "Analista de Créditos Pyme"},
			Skills: []string{"Evaluación crediticia", "Excel avanzado", "Riesgo crediticio", "Cobranzas"}, Positions: []string{"Analista de Créditos", "Asesor de Negocios"},
			Functions: []string{"Evaluación de expedientes de crédito pyme y consumo", "Visitas de campo a clientes", "Seguimiento de cartera y mora"},
			Careers:   []string{"Administración", "Economía", "Contabilidad"}, SalaryMin: 2500, SalaryMax: 5500, QtyMax: 5},
		{Code: "CALL_CENTER", Name: "Asesor de contact center", Headlines: []string{"Asesor de Atención al Cliente", "Ejecutivo de Contact Center", "Asesor Telefónico"},
			Skills: []string{"Atención al cliente", "CRM Salesforce", "Ventas telefónicas", "Office"}, Positions: []string{"Asesor de Contact Center", "Teleoperador"},
			Functions: []string{"Atención de consultas y reclamos por canal telefónico", "Venta cruzada de productos financieros", "Registro de casos en CRM"},
			Careers:   []string{"Administración", "Marketing"}, SalaryMin: 1150, SalaryMax: 1900, QtyMax: 10, TechnicalEd: true},
		{Code: "ACCOUNTANT", Name: "Analista contable", Headlines: []string{"Analista Contable", "Contador Senior", "Asistente Contable"},
			Skills: []string{"NIIF", "SAP FI", "Excel avanzado", "Conciliaciones bancarias", "PDT"}, Positions: []string{"Analista Contable", "Asistente Contable"},
			Functions: []string{"Registro de operaciones y cierres contables mensuales", "Conciliaciones bancarias", "Preparación de estados financieros bajo NIIF"},
			Careers:   []string{"Contabilidad"}, SalaryMin: 3000, SalaryMax: 7000, QtyMax: 2},
		{Code: "RISK_ANALYST", Name: "Analista de riesgos", Headlines: []string{"Analista de Riesgos", "Analista de Riesgo de Mercado"},
			Skills: []string{"Riesgo crediticio", "Python", "SQL", "Power BI", "SAS"}, Positions: []string{"Analista de Riesgos", "Especialista de Riesgos"},
			Functions: []string{"Modelos de scoring y seguimiento de cartera", "Reportes regulatorios a la SBS", "Tableros de indicadores en Power BI"},
			Careers:   []string{"Economía", "Ingeniería Estadística", "Ingeniería Industrial"}, SalaryMin: 5000, SalaryMax: 10000, QtyMax: 2},
		{Code: "COLLECTIONS", Name: "Gestor de cobranzas", Headlines: []string{"Gestor de Cobranzas", "Ejecutivo de Recuperaciones"},
			Skills: []string{"Cobranzas", "Negociación", "Excel", "Atención al cliente"}, Positions: []string{"Gestor de Cobranzas", "Ejecutivo de Recuperaciones"},
			Functions: []string{"Gestión de cartera morosa temprana y tardía", "Negociación de acuerdos de pago", "Reporte diario de efectividad"},
			Careers:   []string{"Administración", "Derecho"}, SalaryMin: 1500, SalaryMax: 3000, QtyMax: 6, TechnicalEd: true},
	},
	Clients: []Client{{"Banco de Crédito del Perú", "20100047218", "Calle Centenario 156, La Molina"}, {"Interbank", "20100053455", "Av. Carlos Villarán 140, La Victoria"},
		{"BBVA Perú", "20100130204", "Av. República de Panamá 3055, San Isidro"}, {"Scotiabank Perú", "20100043140", "Av. Canaval y Moreyra 522, San Isidro"},
		{"Mibanco", "20382036655", "Av. Domingo Orué 165, Surquillo"}, {"Caja Huancayo", "20130200789", "Jr. Real 341, Huancayo"},
		{"Caja Piura", "20102361939", "Jr. Ayacucho 353, Piura"}, {"Financiera Confianza", "20228319182", "Av. Javier Prado Oeste 757, Magdalena"},
		{"Rimac Seguros", "20100041953", "Av. Paseo de la República 3505, San Isidro"}, {"La Positiva Seguros", "20100210909", "Av. Javier Prado Este 492, San Isidro"},
		{"Atento Perú", "20503255916", "Av. Venezuela 2470, Lima"}, {"Konecta Perú", "20516233512", "Av. Paseo de la República 291, Lima"}},
	Companies:    []string{"Banco Pichincha", "Caja Arequipa", "Financiera Oh!", "Compartamos Financiera", "Atento", "Konecta", "Teleperformance", "Banco Falabella", "Caja Sullana", "Edpyme Alternativa", "Crediscotia"},
	Institutions: []string{"Universidad del Pacífico", "Universidad de Lima", "Universidad Nacional Mayor de San Marcos", "Universidad San Martín de Porres", "Universidad Peruana de Ciencias Aplicadas", "Universidad César Vallejo", "Instituto ISIL", "Universidad Continental"},
	Cities:       []string{"Lima", "Lima", "Lima", "Arequipa", "Huancayo", "Piura", "Trujillo", "Chiclayo", "Ica"},
}

var retailIndustry = &Industry{
	Profiles: []Profile{
		{Code: "STORE_MANAGER", Name: "Jefe de tienda", Headlines: []string{"Jefe de Tienda", "Administrador de Tienda", "Subgerente de Tienda"},
			Skills: []string{"Gestión de tiendas", "Manejo de personal", "KPIs comerciales", "Excel", "Visual merchandising"}, Positions: []string{"Jefe de Tienda", "Subgerente de Tienda"},
			Functions: []string{"Gestión de ventas, mermas y presupuesto de la tienda", "Liderazgo de un equipo de 25 colaboradores", "Control de inventarios cíclicos"},
			Careers:   []string{"Administración", "Ingeniería Industrial"}, SalaryMin: 4500, SalaryMax: 8000, QtyMax: 2},
		{Code: "CASHIER", Name: "Cajero", Headlines: []string{"Cajero Polifuncional", "Cajera", "Asesor de Caja"},
			Skills: []string{"Caja y arqueo", "Atención al cliente", "POS", "Manejo de efectivo"}, Positions: []string{"Cajero Polifuncional", "Cajero"},
			Functions: []string{"Cobro en caja y arqueo de fin de turno", "Atención y orientación al cliente", "Reposición de productos en góndolas"},
			Careers:   []string{"Administración"}, SalaryMin: 1130, SalaryMax: 1500, QtyMax: 12, TechnicalEd: true},
		{Code: "WAREHOUSE_SUP", Name: "Supervisor de almacén", Headlines: []string{"Supervisor de Almacén", "Jefe de Almacén", "Supervisor de Operaciones Logísticas"},
			Skills: []string{"WMS", "Gestión de inventarios", "SAP MM", "Excel avanzado", "5S"}, Positions: []string{"Supervisor de Almacén", "Asistente de Almacén"},
			Functions: []string{"Supervisión de recepción, almacenamiento y despacho", "Control de inventarios en WMS", "Indicadores de exactitud y nivel de servicio"},
			Careers:   []string{"Ingeniería Industrial", "Administración", "Logística"}, SalaryMin: 3500, SalaryMax: 6500, QtyMax: 2},
		{Code: "FORKLIFT", Name: "Operario de montacargas", Headlines: []string{"Operador de Montacargas", "Operario de Almacén"},
			Skills: []string{"Montacargas", "Picking", "Gestión de inventarios", "Seguridad industrial"}, Positions: []string{"Operador de Montacargas", "Operario de Almacén"},
			Functions: []string{"Operación de montacargas y apilador eléctrico", "Picking y packing de pedidos", "Carga y descarga de unidades"},
			Careers:   []string{"Logística"}, SalaryMin: 1300, SalaryMax: 2200, QtyMax: 10, TechnicalEd: true},
		{Code: "SALES_ADVISOR", Name: "Asesor de ventas", Headlines: []string{"Asesor de Ventas", "Vendedor de Piso", "Ejecutivo de Ventas Retail"},
			Skills: []string{"Ventas consultivas", "Atención al cliente", "Visual merchandising", "Cumplimiento de metas"}, Positions: []string{"Asesor de Ventas", "Vendedor"},
			Functions: []string{"Asesoría y venta de productos en piso", "Cumplimiento de cuota mensual", "Orden y exhibición del área"},
			Careers:   []string{"Administración", "Marketing"}, SalaryMin: 1300, SalaryMax: 2800, QtyMax: 10, TechnicalEd: true},
		{Code: "DRIVER", Name: "Chofer repartidor", Headlines: []string{"Chofer Repartidor", "Conductor de Reparto"},
			Skills: []string{"Licencia A-IIB", "Manejo defensivo", "Rutas de reparto", "Atención al cliente"}, Positions: []string{"Chofer Repartidor", "Conductor"},
			Functions: []string{"Reparto de pedidos en Lima Metropolitana", "Cobranza contra entrega y liquidación diaria", "Cuidado y mantenimiento básico de la unidad"},
			Careers:   []string{"Mecánica Automotriz"}, SalaryMin: 1600, SalaryMax: 2600, QtyMax: 6, TechnicalEd: true},
	},
	Clients: []Client{{"Supermercados Peruanos (Plaza Vea)", "20100070970", "Calle Morelli 181, San Borja"}, {"Hipermercados Tottus", "20508565934", "Av. Angamos Este 1805, Surquillo"},
		{"Sodimac Perú", "20389230724", "Av. Angamos Este 1805, Surquillo"}, {"Ripley Perú", "20337564373", "Calle Las Begonias 545, San Isidro"},
		{"Saga Falabella", "20100128056", "Av. Paseo de la República 3220, San Isidro"}, {"Promart Homecenter", "20536557858", "Av. Aviación 2405, San Borja"},
		{"Tiendas Mass", "20608280333", "Calle Morelli 181, San Borja"}, {"Oechsle", "20493020618", "Calle Morelli 181, San Borja"},
		{"Ransa", "20100039207", "Av. Argentina 2833, Callao"}, {"Inkafarma", "20331061655", "Av. Defensa del Morro 1277, Chorrillos"},
		{"Cencosud Retail (Wong y Metro)", "20109072177", "Calle Augusto Angulo 130, Miraflores"}, {"Real Plaza", "20512715511", "Av. Paseo de la República 3220, San Isidro"}},
	Companies:    []string{"Plaza Vea", "Tottus", "Metro", "Wong", "Sodimac", "Maestro", "Ripley", "Saga Falabella", "Oechsle", "Ransa", "Dinet", "Inkafarma", "Mifarma", "Tambo+", "Oxxo"},
	Institutions: []string{"SENATI", "Instituto ISIL", "Instituto Cibertec", "Universidad Tecnológica del Perú", "Universidad San Martín de Porres", "Universidad César Vallejo", "Universidad Privada del Norte", "Instituto Toulouse Lautrec"},
	Cities:       []string{"Lima", "Lima", "Lima", "Lima", "Callao", "Arequipa", "Trujillo", "Chiclayo", "Piura", "Ica"},
}

var healthIndustry = &Industry{
	Profiles: []Profile{
		{Code: "NURSE", Name: "Licenciada(o) en enfermería", Headlines: []string{"Licenciada en Enfermería", "Enfermera de Emergencias", "Enfermero UCI"},
			Skills: []string{"Atención de emergencias", "Triaje", "BLS/ACLS", "Administración de medicamentos", "Historia clínica electrónica"}, Positions: []string{"Licenciada en Enfermería", "Enfermera Asistencial"},
			Functions: []string{"Atención de pacientes en hospitalización y emergencia", "Administración de medicamentos y control de signos vitales", "Registro en historia clínica electrónica"},
			Careers:   []string{"Enfermería"}, SalaryMin: 3200, SalaryMax: 5500, QtyMax: 6},
		{Code: "NURSING_TECH", Name: "Técnico en enfermería", Headlines: []string{"Técnica en Enfermería", "Técnico Asistencial"},
			Skills: []string{"Atención al paciente", "Toma de muestras", "Bioseguridad", "Triaje"}, Positions: []string{"Técnica en Enfermería", "Técnico de Enfermería"},
			Functions: []string{"Apoyo en la atención de pacientes hospitalizados", "Preparación de material y bioseguridad", "Traslado y movilización de pacientes"},
			Careers:   []string{"Técnica en Enfermería"}, SalaryMin: 1500, SalaryMax: 2500, QtyMax: 8, TechnicalEd: true},
		{Code: "GP", Name: "Médico general", Headlines: []string{"Médico General", "Médico Cirujano", "Médico de Emergencias"},
			Skills: []string{"Atención de emergencias", "BLS/ACLS", "Historia clínica electrónica", "Medicina ocupacional"}, Positions: []string{"Médico General", "Médico Asistente"},
			Functions: []string{"Consulta externa y atención de emergencias", "Evaluaciones médicas ocupacionales", "Elaboración de informes médicos"},
			Careers:   []string{"Medicina Humana"}, SalaryMin: 7000, SalaryMax: 13000, QtyMax: 3},
		{Code: "PHARMACIST", Name: "Químico farmacéutico", Headlines: []string{"Químico Farmacéutico", "Director Técnico de Farmacia"},
			Skills: []string{"Farmacovigilancia", "Buenas prácticas de almacenamiento", "Atención farmacéutica", "SAP"}, Positions: []string{"Químico Farmacéutico", "Director Técnico"},
			Functions: []string{"Dirección técnica de botica y control de psicotrópicos", "Atención farmacéutica al paciente", "Cumplimiento de BPA y BPD"},
			Careers:   []string{"Farmacia y Bioquímica"}, SalaryMin: 4000, SalaryMax: 7000, QtyMax: 4},
		{Code: "MED_TECHNOLOGIST", Name: "Tecnólogo médico", Headlines: []string{"Tecnólogo Médico en Laboratorio", "Tecnólogo Médico en Radiología", "Tecnólogo en Terapia Física"},
			Skills: []string{"Toma de muestras", "Laboratorio clínico", "Control de calidad", "Bioseguridad"}, Positions: []string{"Tecnólogo Médico", "Tecnólogo de Laboratorio"},
			Functions: []string{"Procesamiento de muestras de hematología y bioquímica", "Control de calidad interno", "Validación de resultados"},
			Careers:   []string{"Tecnología Médica"}, SalaryMin: 3500, SalaryMax: 6000, QtyMax: 3},
	},
	Clients: []Client{{"Clínica Internacional", "20100054184", "Av. Garcilaso de la Vega 1420, Lima"}, {"Clínica Ricardo Palma", "20100121809", "Av. Javier Prado Este 1066, San Isidro"},
		{"Auna", "20601184266", "Av. República de Panamá 3461, San Isidro"}, {"Clínica Sanna", "20100208830", "Av. Paseo de la República 3058, San Isidro"},
		{"Clínica San Pablo", "20100039991", "Av. El Polo 789, Santiago de Surco"}, {"Inkafarma", "20331061655", "Av. Defensa del Morro 1277, Chorrillos"},
		{"Mifarma", "20512002090", "Av. Defensa del Morro 1277, Chorrillos"}, {"Laboratorio Roe", "20100101921", "Av. Petit Thouars 3765, San Isidro"},
		{"Oncosalud", "20101039910", "Av. Guardia Civil 571, San Borja"}, {"Clínica Delgado", "20538349624", "Av. Angamos Oeste 450, Miraflores"}},
	Companies:    []string{"Hospital Rebagliati (EsSalud)", "Hospital Dos de Mayo", "Clínica Internacional", "Clínica Ricardo Palma", "Clínica San Felipe", "Inkafarma", "Boticas BTL", "Laboratorio Suiza Lab", "Clínica Javier Prado", "Hospital Cayetano Heredia"},
	Institutions: []string{"Universidad Peruana Cayetano Heredia", "Universidad Nacional Mayor de San Marcos", "Universidad San Martín de Porres", "Universidad Norbert Wiener", "Universidad Científica del Sur", "Universidad Nacional Federico Villarreal", "Instituto Daniel Alcides Carrión", "Instituto Arzobispo Loayza"},
	Cities:       []string{"Lima", "Lima", "Lima", "Arequipa", "Trujillo", "Chiclayo", "Cusco", "Piura"},
}

var tenants = []TenantSpec{
	{Slug: "fractal", Name: "Fractal", Existing: true, Analysts: 4, Commercial: 1, Talents: 500, RQs: 100, CandMin: 2, CandMax: 10, Industry: itIndustry},
	{Slug: "explomint", Name: "Explomint", Legal: "Explomint Perforaciones S.A.C.", RUC: "20608712345", Primary: "#C2410C", Secondary: "#1F2937",
		Analysts: 7, Commercial: 1, Talents: 4200, RQs: 120, CandMin: 15, CandMax: 50, Industry: miningIndustry},
	{Slug: "pacifico-staffing", Name: "Pacífico Staffing", Legal: "Pacífico Staffing Perú S.A.C.", RUC: "20601234567", Primary: "#1D4ED8", Secondary: "#0F766E",
		Analysts: 3, Commercial: 1, Talents: 500, RQs: 100, CandMin: 2, CandMax: 10, Industry: financeIndustry},
	{Slug: "andes-retail", Name: "Andes Retail Talent", Legal: "Andes Retail Talent E.I.R.L.", RUC: "20605555123", Primary: "#16A34A", Secondary: "#CA8A04",
		Analysts: 3, Commercial: 1, Talents: 500, RQs: 100, CandMin: 3, CandMax: 12, Industry: retailIndustry},
	{Slug: "medica-talent", Name: "Médica Talent", Legal: "Médica Talent Selección S.A.C.", RUC: "20609876543", Primary: "#0891B2", Secondary: "#7C3AED",
		Analysts: 3, Commercial: 1, Talents: 500, RQs: 100, CandMin: 2, CandMax: 8, Industry: healthIndustry},
}
