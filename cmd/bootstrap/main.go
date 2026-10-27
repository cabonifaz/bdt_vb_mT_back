// bootstrap crea el primer Super Admin. Solo funciona si todavía no existe ninguno.
// Uso: BOOTSTRAP_EMAIL=... BOOTSTRAP_NAME=... BOOTSTRAP_PASSWORD=... go run ./cmd/bootstrap
// (la contraseña se lee de variable de entorno para que no quede en el historial de la terminal).
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/config"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/wire"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config.LoadDotEnv(".env")
	cfg, err := config.Load()
	if err != nil {
		log.Error("configuración inválida", "err", err)
		os.Exit(1)
	}
	email, name, pw := os.Getenv("BOOTSTRAP_EMAIL"), os.Getenv("BOOTSTRAP_NAME"), os.Getenv("BOOTSTRAP_PASSWORD")
	if email == "" || name == "" || pw == "" {
		log.Error("defina BOOTSTRAP_EMAIL, BOOTSTRAP_NAME y BOOTSTRAP_PASSWORD")
		os.Exit(1)
	}
	ctx := context.Background()
	a, err := wire.Build(ctx, cfg, log)
	if err != nil {
		log.Error("no se pudo iniciar", "err", err)
		os.Exit(1)
	}
	defer a.DB.Close()
	if err := a.Auth.BootstrapSuperAdmin(ctx, email, name, pw); err != nil {
		log.Error("no se pudo crear el Super Admin", "err", err)
		os.Exit(1)
	}
	log.Info("Super Admin creado")
}
