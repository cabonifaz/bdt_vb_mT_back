// Package wire arma la aplicación: conecta adaptadores con casos de uso (composition root).
package wire

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/httpapi"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/mailer"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/mysql"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/security"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/storage"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/app"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/platform/config"
)

type App struct {
	DB     *sql.DB
	Auth   *app.AuthService
	Server *httpapi.Server
}

func Build(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	db, err := mysql.Open(ctx, mysql.ConnConfig{Host: cfg.DBHost, Port: cfg.DBPort, Name: cfg.DBName,
		User: cfg.DBUser, Password: cfg.DBPassword, TLS: cfg.DBTLS})
	if err != nil {
		return nil, err
	}
	cipher, err := security.NewAESCipher(cfg.DataKey, cfg.BlindIndexKey)
	if err != nil {
		return nil, err
	}
	issuer, err := security.NewJWTIssuer(cfg.JWTSecret, cfg.AccessTTL)
	if err != nil {
		return nil, err
	}
	files, err := storage.NewLocal(cfg.StoragePath)
	if err != nil {
		return nil, err
	}
	store := mysql.NewStore(db)
	hasher := security.NewArgon2Hasher()
	tenantRepo := mysql.TenantRepo{Store: store}

	auth := &app.AuthService{Repo: mysql.AuthRepo{Store: store}, Cipher: cipher, Hasher: hasher, Tokens: issuer,
		Cfg: app.AuthConfig{RefreshTTL: cfg.RefreshTTL, MaxAttempts: 5, LockMinutes: 15}}
	users := &app.UserService{Users: mysql.UserRepo{Store: store}, Invitations: mysql.InvitationRepo{Store: store},
		Cipher: cipher, Hasher: hasher, Mailer: mailer.LogMailer{Log: log}, AppURL: cfg.PublicAppURL,
		InviteTTL: cfg.InviteTTL, ExposeLink: cfg.Env == "local" || cfg.Env == "staging"}
	fileRepo := mysql.FileRepo{Store: store}

	srv := httpapi.NewServer(httpapi.Config{CORSOrigins: cfg.CORSOrigins, TrustProxy: cfg.TrustProxy,
		CookieSecure: !cfg.IsLocal(), RateGeneral: cfg.RateGeneralPerMin, RateAuth: cfg.RateAuthPerMin}, log, httpapi.Deps{
		Auth:         auth,
		Tenants:      &app.TenantService{Repo: tenantRepo, Users: users},
		Users:        users,
		ApiKeys:      &app.ApiKeyService{Repo: mysql.ApiKeyRepo{Store: store}, Pepper: cfg.APIKeyPepper},
		Masters:      &app.MasterService{Repo: mysql.MasterRepo{Store: store}},
		Files:        &app.FileService{Repo: fileRepo, Storage: files, Tenants: tenantRepo},
		Talents:      &app.TalentService{Repo: mysql.TalentRepo{Store: store}, Cipher: cipher, Storage: files},
		Sync:         mysql.SyncRepo{Store: store},
		Audit:        mysql.AuditRepo{Store: store},
		Translations: mysql.TranslationRepo{Store: store},
		Tokens:       issuer,
		Health:       func() error { return store.Ping(context.Background()) },
	})
	return &App{DB: db, Auth: auth, Server: srv}, nil
}
