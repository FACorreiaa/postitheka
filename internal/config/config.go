// Package config loads the application's settings from the environment.
//
// There is deliberately no package-level Config variable. Everything that needs
// a setting is handed one at construction, which keeps the dependency visible
// in the signature and lets a test build a Config without touching the process
// environment.
package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// Env is "development" or "production". It decides defaults (log format,
	// auto-migrate, creating the local bucket), never behaviour a user sees.
	Env string

	ListenAddr string

	DatabaseURL string
	AutoMigrate bool

	// TokenEncryptionKey seals platform credentials at rest: base64 of 32
	// random bytes (`openssl rand -base64 32`). Losing it orphans every
	// connected account, so it lives in a sealed secret, not in the database.
	TokenEncryptionKey string

	// S3-compatible object storage for uploads: Cloudflare R2 in production,
	// MinIO from docker-compose locally.
	S3Endpoint  string
	S3Region    string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3UseSSL    bool

	// PublishWorkers caps how many posts are sent to platforms at once, per
	// process. Each one can be streaming up to ~200 MB of video to Discord,
	// so this is a bandwidth knob more than a CPU one.
	PublishWorkers int

	LogLevel  string
	LogFormat string

	ShutdownTimeout time.Duration
}

func (c Config) IsProduction() bool { return c.Env == "production" }

// Load reads the environment into a Config and refuses to start without
// anything the app cannot work without. An app that boots without its
// encryption key would accept a connection and then fail to store it.
func Load() (Config, error) {
	c := Config{
		Env:                env("GO_ENV", "development"),
		ListenAddr:         env("LISTEN_ADDR", ":8095"),
		DatabaseURL:        env("DATABASE_URL", ""),
		TokenEncryptionKey: env("TOKEN_ENCRYPTION_KEY", ""),
		S3Endpoint:         env("S3_ENDPOINT", ""),
		// R2 accepts "auto" and nothing else meaningful; MinIO ignores it.
		S3Region:        env("S3_REGION", "auto"),
		S3Bucket:        env("S3_BUCKET", ""),
		S3AccessKey:     env("S3_ACCESS_KEY", ""),
		S3SecretKey:     env("S3_SECRET_KEY", ""),
		LogLevel:        env("LOG_LEVEL", "info"),
		LogFormat:       env("LOG_FORMAT", ""),
		ShutdownTimeout: 30 * time.Second,
	}

	var err error
	if c.AutoMigrate, err = envBool("AUTO_MIGRATE", !c.IsProduction()); err != nil {
		return Config{}, err
	}
	if c.S3UseSSL, err = envBool("S3_USE_SSL", true); err != nil {
		return Config{}, err
	}
	if c.PublishWorkers, err = envInt("PUBLISH_WORKERS", 4); err != nil {
		return Config{}, err
	}
	if c.PublishWorkers < 1 {
		return Config{}, fmt.Errorf("config: PUBLISH_WORKERS must be at least 1, got %d", c.PublishWorkers)
	}

	if c.LogFormat == "" {
		// Text is readable in a terminal; JSON is what a log collector can
		// index. Defaulting by environment means neither has to be configured.
		if c.IsProduction() {
			c.LogFormat = "json"
		} else {
			c.LogFormat = "text"
		}
	}

	var missing []string
	for name, v := range map[string]string{
		"DATABASE_URL":         c.DatabaseURL,
		"TOKEN_ENCRYPTION_KEY": c.TokenEncryptionKey,
		"S3_ENDPOINT":          c.S3Endpoint,
		"S3_BUCKET":            c.S3Bucket,
		"S3_ACCESS_KEY":        c.S3AccessKey,
		"S3_SECRET_KEY":        c.S3SecretKey,
	} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		// Every missing name at once, so a fresh setup is one round of edits.
		slices.Sort(missing)
		return Config{}, fmt.Errorf("config: required but unset: %s", strings.Join(missing, ", "))
	}

	return c, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer, got %q", key, raw)
	}
	return v, nil
}

func envBool(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("config: %s must be a boolean, got %q", key, raw)
	}
	return v, nil
}
