package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // base de zonas horarias embebida (Windows/contenedores mínimos)

	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/httpapi"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/config"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/wire"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	config.LoadDotEnv(".env")
	cfg, err := config.Load()
	if err != nil {
		log.Error("configuración inválida", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := wire.Build(ctx, cfg, log)
	if err != nil {
		log.Error("no se pudo iniciar", "err", err)
		os.Exit(1)
	}
	defer a.DB.Close()

	srv := httpapi.NewHTTPServer(":"+cfg.HTTPPort, a.Server.Handler())
	go func() {
		log.Info("API escuchando", "port", cfg.HTTPPort, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("servidor detenido", "err", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
