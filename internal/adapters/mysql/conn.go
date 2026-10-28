package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

type ConnConfig struct {
	Host, Port, Name, User, Password string
	TLS                              string // "", "true", "skip-verify", "preferred"
	MaxOpen                          int
}

// Open abre el pool. La zona horaria de sesión es UTC; la app convierte a la del tenant.
func Open(ctx context.Context, c ConnConfig) (*sql.DB, error) {
	cfg := driver.NewConfig()
	cfg.User = c.User
	cfg.Passwd = c.Password
	cfg.Net = "tcp"
	cfg.Addr = c.Host + ":" + c.Port
	cfg.DBName = c.Name
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'", "charset": "utf8mb4"}
	cfg.Collation = "utf8mb4_0900_ai_ci"
	cfg.Timeout = 10 * time.Second
	cfg.ReadTimeout = 30 * time.Second
	cfg.WriteTimeout = 30 * time.Second
	// parámetros interpolados por el driver (escapados para utf8mb4): 1 viaje por consulta en vez de 2 (preparar + ejecutar).
	// Con la BD lejos (~200 ms por viaje) reduce a la mitad el tiempo de cada llamada.
	cfg.InterpolateParams = true
	// compresión zlib del protocolo: las respuestas grandes (tablero, búsquedas) necesitan menos viajes de red
	if err := cfg.Apply(driver.EnableCompression(true)); err != nil {
		return nil, err
	}
	if c.TLS != "" {
		cfg.TLSConfig = c.TLS
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	if c.MaxOpen == 0 {
		c.MaxOpen = 10 // el servidor es compartido (max_connections=151)
	}
	db.SetMaxOpenConns(c.MaxOpen)
	db.SetMaxIdleConns(c.MaxOpen/2 + 1)
	// el servidor cierra conexiones inactivas a los 60 s (wait_timeout): soltarlas antes; keepWarm las mantiene vivas
	db.SetConnMaxIdleTime(50 * time.Second)
	db.SetConnMaxLifetime(30 * time.Minute)
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("conexión MySQL %s@%s/%s: %w", c.User, cfg.Addr, c.Name, err)
	}
	go keepWarm(db, warmConns)
	return db, nil
}

// warmConns: conexiones que se mantienen abiertas aunque no haya tráfico. Abrir una nueva cuesta varios viajes de red
// (TCP + autenticación, ~800 ms con la BD lejos); sin esto, tras una pausa de 50 s la siguiente petición pagaba ese costo.
const warmConns = 3

// keepWarm hace ping cada 20 s a varias conexiones a la vez (así son distintas), antes del wait_timeout del servidor.
// Termina cuando se cierra el pool.
func keepWarm(db *sql.DB, n int) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conns := make([]*sql.Conn, 0, n)
		for i := 0; i < n; i++ {
			c, err := db.Conn(ctx)
			if err != nil && strings.Contains(err.Error(), "database is closed") { // database/sql no exporta ese error
				cancel()
				return
			}
			if err != nil {
				break
			}
			conns = append(conns, c)
		}
		for _, c := range conns {
			_ = c.PingContext(ctx)
			_ = c.Close() // vuelve al pool
		}
		cancel()
	}
}
