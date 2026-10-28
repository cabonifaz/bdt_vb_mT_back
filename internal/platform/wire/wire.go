// Package wire arma la aplicación: conecta adaptadores con casos de uso (composition root).
package wire

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/ai"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/httpapi"
	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/identity"
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
	// zona horaria por defecto de los tenants (las entrevistas se agendan en hora local)
	tz, err := time.LoadLocation(cfg.DefaultTimezone)
	if err != nil {
		return nil, err
	}
	hasher := security.NewArgon2Hasher()
	tenantRepo := mysql.TenantRepo{Store: store}

	auth := &app.AuthService{Repo: mysql.AuthRepo{Store: store}, Cipher: cipher, Hasher: hasher, Tokens: issuer,
		Cfg: app.AuthConfig{RefreshTTL: cfg.RefreshTTL, RememberTTL: cfg.RememberTTL, ReuseGrace: cfg.ReuseGrace, MaxAttempts: 5, LockMinutes: 15}}
	users := &app.UserService{Users: mysql.UserRepo{Store: store}, Invitations: mysql.InvitationRepo{Store: store},
		Cipher: cipher, Hasher: hasher, Mailer: mailer.LogMailer{Log: log}, AppURL: cfg.PublicAppURL,
		InviteTTL: cfg.InviteTTL, ExposeLink: cfg.Env == "local" || cfg.Env == "staging"}
	fileRepo := mysql.FileRepo{Store: store}

	usageRepo := mysql.UsageRepo{Store: store}
	srv := httpapi.NewServer(httpapi.Config{CORSOrigins: cfg.CORSOrigins, TrustProxy: cfg.TrustProxy,
		CookieSecure: !cfg.IsLocal(), RateGeneral: cfg.RateGeneralPerMin, RateAuth: cfg.RateAuthPerMin}, log, httpapi.Deps{
		Auth:    auth,
		Tenants: &app.TenantService{Repo: tenantRepo, Users: users},
		Users:   users,
		ApiKeys: &app.ApiKeyService{Repo: mysql.ApiKeyRepo{Store: store}, Pepper: cfg.APIKeyPepper},
		Masters: &app.MasterService{Repo: mysql.MasterRepo{Store: store}},
		Files:   &app.FileService{Repo: fileRepo, Storage: files, Tenants: tenantRepo},
		Talents: &app.TalentService{Repo: mysql.TalentRepo{Store: store}, Cipher: cipher, Storage: files,
			Translator: ai.NewOpenAI(cfg.OpenAIKey, cfg.OpenAIModel), Usage: usageRepo},
		Sync:       mysql.SyncRepo{Store: store},
		Blacklist:  &app.BlacklistService{Repo: mysql.BlacklistRepo{Store: store}, Cipher: cipher},
		Interviews: &app.InterviewService{Repo: mysql.InterviewRepo{Store: store}, Cipher: cipher, Storage: files, Timezone: tz},
		Home:       &app.HomeService{Repo: mysql.HomeRepo{Store: store}, Timezone: tz},
		Usage:      &app.UsageService{Repo: usageRepo},
		Identity: &app.IdentityService{Repo: mysql.IdentityRepo{Store: store}, Usage: usageRepo,
			Lookup: identity.New(identity.Config{Provider: cfg.DNIProvider, Token: cfg.DNIToken, BaseURL: cfg.DNIURL})},
		Audit:        mysql.AuditRepo{Store: store},
		Translations: mysql.TranslationRepo{Store: store},
		Tokens:       issuer,
		Health:       func() error { return store.Ping(context.Background()) },
	})
	return &App{DB: db, Auth: auth, Server: srv}, nil
}
