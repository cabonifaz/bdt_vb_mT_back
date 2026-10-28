// Limpieza de staging y generación de datos de prueba realistas (tenants, analistas, clientes, talentos, RQ con candidatos,
// entrevistas y lista negra). Los datos se crean con los mismos servicios y SP de la app (validaciones, cifrado y auditoría).
//
// Uso (con el usuario de migraciones, nunca en producción):
//
//	DB_USER=<migrator> DB_PASSWORD=<...> go run ./cmd/seed -reset -seed -keep-admin <correo del admin de Fractal>
//
// Las credenciales generadas se escriben en -creds (fuera del repo).
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math/big"
	mrand "math/rand/v2"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/mailer"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/mysql"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/security"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/storage"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/app"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/config"
)

type env struct {
	db       *sql.DB
	cipher   ports.Cipher
	auth     *app.AuthService
	users    *app.UserService
	tenants  *app.TenantService
	masters  *app.MasterService
	talents  *app.TalentService
	rqs      *app.RequirementService
	ivs      *app.InterviewService
	bl       *app.BlacklistService
	workers  int
	errCount atomic.Int64
	rnd      *mrand.Rand
	mu       sync.Mutex // protege rnd
}

func main() {
	reset := flag.Bool("reset", false, "borrar los datos de staging (conserva Super Admin, tenant Fractal con su marca, el admin indicado y los catálogos globales)")
	seed := flag.Bool("seed", false, "generar los datos de prueba")
	topup := flag.Bool("topup", false, "completar los RQ que falten en los tenants ya generados")
	leads := flag.Bool("leads", false, "agregar el jefe de selección a los tenants ya generados (sus claves se añaden a -creds)")
	keepAdmin := flag.String("keep-admin", "", "correo del admin de Fractal que se conserva")
	creds := flag.String("creds", "../_secrets/seed_users.env", "archivo (fuera del repo) donde se guardan las credenciales generadas")
	workers := flag.Int("workers", 16, "llamadas concurrentes a la BD")
	only := flag.String("only", "", "generar solo este tenant (slug)")
	scale := flag.Float64("scale", 1, "factor de volumen (p. ej. 0.02 para una prueba rápida)")
	flag.Parse()

	config.LoadDotEnv(".env")
	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}
	if cfg.Env == "production" || strings.Contains(cfg.DBName, "prod") {
		fail(fmt.Errorf("el generador no se ejecuta contra producción (%s)", cfg.DBName))
	}
	ctx := context.Background()
	// la red hacia la BD puede cortarse unos segundos: las conexiones nuevas se reintentan hasta 5 minutos
	driver.RegisterDialContext("tcp", retryDial)
	db, err := mysql.Open(ctx, mysql.ConnConfig{Host: cfg.DBHost, Port: cfg.DBPort, Name: cfg.DBName, User: cfg.DBUser, Password: cfg.DBPassword,
		TLS: cfg.DBTLS, MaxOpen: *workers + 4})
	if err != nil {
		fail(err)
	}
	cipher, err := security.NewAESCipher(cfg.DataKey, cfg.BlindIndexKey)
	if err != nil {
		fail(err)
	}
	issuer, err := security.NewJWTIssuer(cfg.JWTSecret, cfg.AccessTTL)
	if err != nil {
		fail(err)
	}
	files, err := storage.NewLocal(cfg.StoragePath)
	if err != nil {
		fail(err)
	}
	store := mysql.NewStore(db)
	tz, _ := time.LoadLocation(cfg.DefaultTimezone)
	hasher := security.NewArgon2Hasher()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	users := &app.UserService{Users: mysql.UserRepo{Store: store}, Invitations: mysql.InvitationRepo{Store: store}, Cipher: cipher, Hasher: hasher,
		Mailer: mailer.LogMailer{Log: log}, AppURL: cfg.PublicAppURL, InviteTTL: cfg.InviteTTL, ExposeLink: true}
	e := &env{
		db: db, cipher: cipher, users: users, workers: *workers, rnd: mrand.New(mrand.NewPCG(20261028, 7)),
		auth: &app.AuthService{Repo: mysql.AuthRepo{Store: store}, Cipher: cipher, Hasher: hasher, Tokens: issuer,
			Cfg: app.AuthConfig{RefreshTTL: cfg.RefreshTTL, RememberTTL: cfg.RememberTTL, ReuseGrace: cfg.ReuseGrace, MaxAttempts: 5, LockMinutes: 15}},
		tenants: &app.TenantService{Repo: mysql.TenantRepo{Store: store}, Users: users},
		masters: &app.MasterService{Repo: mysql.MasterRepo{Store: store}},
		// sin notificador: la carga masiva no genera miles de notificaciones
		talents: &app.TalentService{Repo: mysql.TalentRepo{Store: store}, Cipher: cipher, Storage: files},
		rqs:     &app.RequirementService{Repo: mysql.RequirementRepo{Store: store}, Cipher: cipher},
		ivs:     &app.InterviewService{Repo: mysql.InterviewRepo{Store: store}, Cipher: cipher, Storage: files, Timezone: tz},
		bl:      &app.BlacklistService{Repo: mysql.BlacklistRepo{Store: store}, Cipher: cipher},
	}

	if *reset {
		if *keepAdmin == "" {
			fail(fmt.Errorf("-reset requiere -keep-admin"))
		}
		start := time.Now()
		if err := e.reset(ctx, *keepAdmin); err != nil {
			fail(err)
		}
		fmt.Printf("limpieza completa en %s\n", time.Since(start).Round(time.Second))
	}
	if *topup {
		for _, spec := range tenants {
			if *only != "" && spec.Slug != *only {
				continue
			}
			if err := e.topUpRQs(ctx, spec); err != nil {
				fail(err)
			}
		}
		fmt.Printf("completado (avisos: %d)\n", e.errCount.Load())
	}
	if *leads {
		var lines []string
		for _, spec := range tenants {
			if *only != "" && spec.Slug != *only {
				continue
			}
			line, err := e.addLead(ctx, spec)
			if err != nil {
				fail(err)
			}
			if line != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) > 0 {
			f, err := os.OpenFile(*creds, os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				fail(err)
			}
			_, err = f.WriteString("\n# Jefes de selección\n" + strings.Join(lines, "\n") + "\n")
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				fail(err)
			}
		}
	}
	if *seed {
		start := time.Now()
		var lines []string
		for _, spec := range tenants {
			if *only != "" && spec.Slug != *only {
				continue
			}
			if *scale != 1 {
				spec.Talents, spec.RQs = max(20, int(float64(spec.Talents)**scale)), max(3, int(float64(spec.RQs)**scale))
			}
			t0 := time.Now()
			out, err := e.seedTenant(ctx, spec, *keepAdmin)
			if err != nil {
				fail(fmt.Errorf("%s: %w", spec.Slug, err))
			}
			lines = append(lines, out...)
			fmt.Printf("%-18s listo en %s (errores acumulados: %d)\n", spec.Slug, time.Since(t0).Round(time.Second), e.errCount.Load())
			if err := writeCreds(*creds, lines); err != nil {
				fail(err)
			}
		}
		fmt.Printf("generación completa en %s; credenciales en %s\n", time.Since(start).Round(time.Second), *creds)
	}
}

// retryDial abre la conexión TCP reintentando ante cortes de red (cada 5 s, hasta 5 minutos).
func retryDial(ctx context.Context, addr string) (net.Conn, error) {
	var last error
	for i := 0; i < 60; i++ {
		c, err := (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", addr)
		if err == nil {
			return c, nil
		}
		last = err
		if i == 0 || i%6 == 0 {
			fmt.Fprintf(os.Stderr, "  red: reintentando conexión a la BD (%v)\n", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return nil, last
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "ERROR:", err)
	os.Exit(1)
}

func writeCreds(path string, lines []string) error {
	head := "# Usuarios generados por cmd/seed (staging). NO subir a ningún repositorio.\n"
	return os.WriteFile(path, []byte(head+strings.Join(lines, "\n")+"\n"), 0o600)
}

// reset borra los datos de staging conservando el Super Admin, el tenant Fractal (marca, plantilla de CV y sus archivos),
// el admin indicado y los catálogos globales. Usa una sola conexión con las verificaciones de FK desactivadas
// mientras borra en orden (hijos primero).
func (e *env) reset(ctx context.Context, keepEmail string) error {
	conn, err := e.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	keepBidx := e.cipher.BlindIndex(strings.ToLower(strings.TrimSpace(keepEmail)))
	var fractal int64
	if err := conn.QueryRowContext(ctx, "SELECT id FROM tenant WHERE slug = 'fractal'").Scan(&fractal); err != nil {
		return fmt.Errorf("tenant fractal: %w", err)
	}
	var keepAdmin int64
	if err := conn.QueryRowContext(ctx, "SELECT id FROM app_user WHERE email_bidx = ? AND tenant_id = ?", keepBidx, fractal).Scan(&keepAdmin); err != nil {
		return fmt.Errorf("admin a conservar (%s): %w", keepEmail, err)
	}
	keepUsers := fmt.Sprintf("(SELECT id FROM (SELECT u.id FROM app_user u JOIN user_role ur ON ur.user_id = u.id JOIN role r ON r.id = ur.role_id WHERE r.code = 'SUPER_ADMIN' UNION SELECT %d) k)", keepAdmin)
	brandingFiles := fmt.Sprintf("(SELECT id FROM (SELECT logo_file_id id FROM tenant_branding WHERE tenant_id = %[1]d UNION SELECT logo_dark_file_id FROM tenant_branding WHERE tenant_id = %[1]d"+
		" UNION SELECT favicon_file_id FROM tenant_branding WHERE tenant_id = %[1]d UNION SELECT cv_header_file_id FROM tenant_branding WHERE tenant_id = %[1]d"+
		" UNION SELECT cv_footer_file_id FROM tenant_branding WHERE tenant_id = %[1]d) f WHERE id IS NOT NULL)", fractal)
	stmts := []string{
		"SET FOREIGN_KEY_CHECKS = 0",
		"DELETE FROM notification_recipient", "DELETE FROM notification", "DELETE FROM push_subscription", "DELETE FROM notification_pref",
		"DELETE FROM requirement_candidate", "DELETE FROM vacancy_career", "DELETE FROM vacancy_skill", "DELETE FROM requirement_vacancy",
		"DELETE FROM requirement_billing", "DELETE FROM requirement_contact", "DELETE FROM interview_answer", "DELETE FROM interview_recording",
		"DELETE FROM interview_interviewer", "DELETE FROM interview", "DELETE FROM requirement", "DELETE FROM requirement_sequence", "DELETE FROM client_contact",
		"DELETE FROM talent_blacklist_history", "DELETE FROM talent_blacklist", "DELETE FROM favorite_collection_talent", "DELETE FROM favorite_collection_share",
		"DELETE FROM favorite_collection", "DELETE FROM talent_feedback", "DELETE FROM talent_language", "DELETE FROM talent_education",
		"DELETE FROM talent_experience", "DELETE FROM talent_soft_skill", "DELETE FROM talent_tech_skill", "DELETE FROM talent",
		"DELETE FROM file_object WHERE id NOT IN " + brandingFiles, "DELETE FROM tenant_usage_daily", "DELETE FROM sync_operation", "DELETE FROM audit_log",
		"DELETE FROM user_session", "DELETE FROM user_invitation", "DELETE FROM api_key",
		fmt.Sprintf("DELETE FROM tenant_translation WHERE tenant_id <> %d", fractal),
		fmt.Sprintf("DELETE FROM report_template WHERE tenant_id IS NOT NULL AND tenant_id <> %d", fractal),
		"DELETE FROM master_item WHERE tenant_id IS NOT NULL",
		"DELETE FROM user_role WHERE user_id NOT IN " + keepUsers, "DELETE FROM app_user WHERE id NOT IN " + keepUsers,
		fmt.Sprintf("DELETE FROM tenant_branding WHERE tenant_id <> %d", fractal), fmt.Sprintf("DELETE FROM tenant WHERE id <> %d", fractal),
		"SET FOREIGN_KEY_CHECKS = 1",
	}
	for _, s := range stmts {
		res, err := conn.ExecContext(ctx, s)
		if err != nil {
			_, _ = conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1")
			return fmt.Errorf("%s: %w", s, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			fmt.Printf("  %-60.60s %d filas\n", s, n)
		}
	}
	return nil
}

// ---------- utilidades ----------

func (e *env) intn(n int) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.rnd.IntN(n)
}

func (e *env) float() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.rnd.Float64()
}

func pick[T any](e *env, xs []T) T { return xs[e.intn(len(xs))] }

func (e *env) chance(p float64) bool { return e.float() < p }

func (e *env) between(a, b int) int {
	if b <= a {
		return a
	}
	return a + e.intn(b-a+1)
}

// fails registra un error sin abortar la generación (se reporta el total al final de cada tenant).
func (e *env) fails(what string, err error) bool {
	if err == nil {
		return false
	}
	if n := e.errCount.Add(1); n <= 25 {
		fmt.Fprintf(os.Stderr, "  aviso: %s: %v\n", what, err)
	}
	return true
}

// parallel ejecuta f(i) para i en [0,n) con e.workers llamadas a la vez.
func (e *env) parallel(n int, f func(i int)) {
	sem := make(chan struct{}, e.workers)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer func() { <-sem; wg.Done() }()
			f(i)
		}(i)
	}
	wg.Wait()
}

func password() string {
	const chars = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 14)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return "Bdt-" + string(b) + "!"
}

func label(es string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"es": es, "en": es})
	return b
}

func ptr[T any](v T) *T { return &v }

// masterMap: código y etiqueta (es) -> uuid de los ítems de catálogo visibles para el tenant.
type masterMap struct {
	byCode  map[string]map[string]string
	byLabel map[string]map[string]string
}

func (m masterMap) id(table, code string) string { return m.byCode[table][code] }

func (m masterMap) ids(table string) []string {
	out := make([]string, 0, len(m.byCode[table]))
	for _, v := range m.byCode[table] {
		out = append(out, v)
	}
	return out
}

func (e *env) loadMasters(ctx context.Context, tid uint64) (masterMap, error) {
	items, err := e.masters.Items(ctx, &tid, "COUNTRY,DOC_IDENTITY_TYPE,CURRENCY,BILLING_MODALITY,TALENT_AVAILABILITY,LANGUAGE,LANGUAGE_LEVEL,"+
		"SOFT_SKILL,TECH_SKILL,ACADEMIC_GRADE,RQ_STATUS,RQ_PROFILE,TIME_UNIT,WORK_MODALITY,CONTRACT_MODALITY,CANDIDATE_STAGE,CLIENT,"+
		"INTERVIEW_TYPE,INTERVIEW_STAGE,INTERVIEW_STATUS,INTERVIEW_DURATION", false)
	if err != nil {
		return masterMap{}, err
	}
	m := masterMap{byCode: map[string]map[string]string{}, byLabel: map[string]map[string]string{}}
	for _, it := range items {
		if m.byCode[it.TableCode] == nil {
			m.byCode[it.TableCode], m.byLabel[it.TableCode] = map[string]string{}, map[string]string{}
		}
		m.byCode[it.TableCode][it.Code] = it.UUID
		var l map[string]string
		if json.Unmarshal(it.Label, &l) == nil {
			m.byLabel[it.TableCode][strings.ToLower(l["es"])] = it.UUID
		}
	}
	return m, nil
}

func codeOf(s string) string {
	r := strings.NewReplacer("á", "A", "é", "E", "í", "I", "ó", "O", "ú", "U", "ñ", "N", "Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ñ", "N")
	s = strings.ToUpper(r.Replace(s))
	var b strings.Builder
	for _, c := range s {
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "_") {
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}
