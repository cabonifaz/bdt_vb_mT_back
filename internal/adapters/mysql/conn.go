package mysql

import (
	"context"
	"database/sql"
	"fmt"
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
	if c.TLS != "" {
		cfg.TLSConfig = c.TLS
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	if c.MaxOpen == 0 {
		c.MaxOpen = 15
	}
	db.SetMaxOpenConns(c.MaxOpen)
	db.SetMaxIdleConns(c.MaxOpen / 3)
	db.SetConnMaxLifetime(30 * time.Minute)
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("conexión MySQL %s@%s/%s: %w", c.User, cfg.Addr, c.Name, err)
	}
	return db, nil
}
