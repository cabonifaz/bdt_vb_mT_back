// migrate aplica el esquema y los procedimientos con el usuario migrator.
// Uso: go run ./cmd/migrate   (lee DB_HOST, DB_PORT, DB_NAME, DB_MIGRATOR_USER, DB_MIGRATOR_PASSWORD)
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/cabonifaz/bdt_vb_mT_back/db"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/mysql"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/config"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config.LoadDotEnv(".env")
	for _, k := range []string{"DB_HOST", "DB_PORT", "DB_NAME", "DB_MIGRATOR_USER", "DB_MIGRATOR_PASSWORD"} {
		if os.Getenv(k) == "" {
			log.Error("falta variable de entorno", "var", k)
			os.Exit(1)
		}
	}
	ctx := context.Background()
	conn, err := mysql.Open(ctx, mysql.ConnConfig{
		Host: os.Getenv("DB_HOST"), Port: os.Getenv("DB_PORT"), Name: os.Getenv("DB_NAME"),
		User: os.Getenv("DB_MIGRATOR_USER"), Password: os.Getenv("DB_MIGRATOR_PASSWORD"),
		TLS: os.Getenv("DB_TLS"), MaxOpen: 2,
	})
	if err != nil {
		log.Error("no se pudo conectar", "err", err)
		os.Exit(1)
	}
	defer conn.Close()
	if err := mysql.Migrate(ctx, conn, db.FS, log); err != nil {
		log.Error("migración fallida", "err", err)
		os.Exit(1)
	}
	log.Info("esquema al día", "db", os.Getenv("DB_NAME"))
}
