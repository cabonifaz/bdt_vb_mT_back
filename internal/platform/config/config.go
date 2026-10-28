// Package config: toda la configuración y los secretos vienen de variables de entorno.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env          string // local | staging | preprod | production
	HTTPPort     string
	PublicAppURL string
	CORSOrigins  []string
	TrustProxy   bool

	DBHost, DBPort, DBName, DBUser, DBPassword, DBTLS string

	JWTSecret     string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	RememberTTL   time.Duration
	ReuseGrace    int
	DataKey       string
	BlindIndexKey string
	APIKeyPepper  []byte
	InviteTTL     time.Duration

	StoragePath     string
	DefaultTimezone string

	// Consulta de DNI (RENIEC): decolecta | factiliza; sin token la función queda deshabilitada
	DNIProvider, DNIToken, DNIURL string

	// Traducción del CV (mismo proveedor que el sistema heredado)
	OpenAIKey, OpenAIModel string

	// Push del sistema operativo (Web Push): claves VAPID generadas con go run ./cmd/vapid; sin claves queda apagado
	VAPIDPublic, VAPIDPrivate, VAPIDSubject string

	RateGeneralPerMin int
	RateAuthPerMin    int
}

func (c Config) IsLocal() bool { return c.Env == "local" }

func Load() (Config, error) {
	c := Config{
		Env:          get("APP_ENV", "local"),
		HTTPPort:     get("PORT", get("HTTP_PORT", "8080")), // Railway inyecta PORT
		PublicAppURL: get("PUBLIC_APP_URL", "http://localhost:5173"),
		CORSOrigins:  split(get("CORS_ORIGINS", "http://localhost:5173")),
		TrustProxy:   get("TRUST_PROXY", "false") == "true",
		DBHost:       os.Getenv("DB_HOST"), DBPort: get("DB_PORT", "3306"), DBName: os.Getenv("DB_NAME"),
		DBUser: os.Getenv("DB_USER"), DBPassword: os.Getenv("DB_PASSWORD"), DBTLS: os.Getenv("DB_TLS"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		AccessTTL:         minutes("ACCESS_TOKEN_MINUTES", 15),
		RefreshTTL:        minutes("REFRESH_TOKEN_MINUTES", 60*12),
		RememberTTL:       minutes("REMEMBER_ME_MINUTES", 60*24*30),
		ReuseGrace:        intVal("REFRESH_REUSE_GRACE_SECONDS", 30),
		DataKey:           os.Getenv("DATA_ENCRYPTION_KEY"),
		BlindIndexKey:     os.Getenv("BLIND_INDEX_KEY"),
		InviteTTL:         minutes("INVITATION_MINUTES", 60*72),
		StoragePath:       get("STORAGE_PATH", "./data/files"),
		DefaultTimezone:   get("DEFAULT_TIMEZONE", "America/Lima"),
		DNIProvider:       get("DNI_API_PROVIDER", "decolecta"),
		DNIToken:          os.Getenv("DNI_API_TOKEN"),
		DNIURL:            os.Getenv("DNI_API_URL"),
		OpenAIKey:         os.Getenv("OPENAI_API_KEY"),
		VAPIDPublic:       os.Getenv("VAPID_PUBLIC_KEY"),
		VAPIDPrivate:      os.Getenv("VAPID_PRIVATE_KEY"),
		VAPIDSubject:      os.Getenv("VAPID_SUBJECT"),
		OpenAIModel:       get("OPENAI_MODEL", "gpt-4.1-mini"),
		RateGeneralPerMin: intVal("RATE_LIMIT_PER_MIN", 600),
		RateAuthPerMin:    intVal("RATE_LIMIT_AUTH_PER_MIN", 10),
	}
	var missing []string
	for k, v := range map[string]string{"DB_HOST": c.DBHost, "DB_NAME": c.DBName, "DB_USER": c.DBUser,
		"DB_PASSWORD": c.DBPassword, "JWT_SECRET": c.JWTSecret, "DATA_ENCRYPTION_KEY": c.DataKey,
		"BLIND_INDEX_KEY": c.BlindIndexKey, "APIKEY_PEPPER": os.Getenv("APIKEY_PEPPER")} {
		if v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("faltan variables de entorno: %s", strings.Join(missing, ", "))
	}
	p, err := base64.StdEncoding.DecodeString(os.Getenv("APIKEY_PEPPER"))
	if err != nil || len(p) < 32 {
		return c, errors.New("APIKEY_PEPPER debe ser base64 de al menos 32 bytes")
	}
	c.APIKeyPepper = p
	if !c.IsLocal() {
		for _, o := range c.CORSOrigins {
			if !strings.HasPrefix(o, "https://") {
				return c, fmt.Errorf("CORS_ORIGINS fuera de local debe usar https: %s", o)
			}
		}
	}
	return c, nil
}

func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.TrimRight(p, "/"))
		}
	}
	return out
}

func minutes(k string, def int) time.Duration {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return time.Duration(v) * time.Minute
	}
	return time.Duration(def) * time.Minute
}

func intVal(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}
