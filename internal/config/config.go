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

	// PublicURL is where users reach this server, without a trailing slash.
	// OAuth platforms redirect back to PublicURL + /oauth/{platform}/callback,
	// and each platform's developer app must list exactly that URL.
	PublicURL string

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

	// LinkedIn developer app ("Share on LinkedIn" and "Sign In with LinkedIn
	// using OpenID Connect"). Both empty: LinkedIn is not offered. The secret
	// is a credential like any other and comes only from the environment.
	LinkedInClientID     string
	LinkedInClientSecret string

	// Threads app (Threads API use case; its own app id and secret, not the
	// main Meta app's). Both empty: Threads is not offered.
	ThreadsAppID     string
	ThreadsAppSecret string

	// Instagram app (the Meta app's "API setup with Instagram login"; its own
	// app id and secret, not the main Meta app's and not Threads's). Both
	// empty: Instagram is not offered.
	InstagramAppID     string
	InstagramAppSecret string

	// Facebook Pages, through Facebook Login on the Meta app: here the id and
	// secret are the Meta app's own (App settings > Basic). Both empty:
	// Facebook is not offered.
	FacebookAppID     string
	FacebookAppSecret string

	// X developer app, OAuth 2.0 as a confidential client ("Web App,
	// Automated App or Bot"). Both empty: X is not offered. Every post is
	// billed to this app.
	XClientID     string
	XClientSecret string

	TikTokClientKey    string
	TikTokClientSecret string

	// PostHog product analytics. An empty key disables capture entirely:
	// nothing is sent, and no credential is needed to develop.
	PostHogAPIKey string
	PostHogHost   string

	// Mascot3DModelURL is where the web app's sign-in page loads the 3D mascot from
	// (BRD-001). A beta experiment: only a beta or local environment sets it, and empty
	// means the flat mascot. Not a secret; the page reads it from GET /client-config.
	Mascot3DModelURL string

	// Sign in with Apple (AUTH-001). AppleBundleIDs are the iOS app's bundle ids, comma
	// separated; AppleWebClientID is the web Services ID. Both are client ids an identity
	// token's audience may name, and neither is secret. Both empty turns Apple sign-in off. No
	// private key: verifying a token needs only Apple's published keys.
	AppleBundleIDs   []string
	AppleWebClientID string

	LogLevel  string
	LogFormat string

	// ShutdownTimeout is how long in-flight requests and publish jobs get after
	// SIGTERM. The deployment's terminationGracePeriodSeconds must be longer,
	// or the kubelet kills the process before a running upload can finish.
	ShutdownTimeout time.Duration
}

func (c Config) IsProduction() bool { return c.Env == "production" }

// Load reads the environment into a Config and refuses to start without
// anything the app cannot work without. An app that boots without its
// encryption key would accept a connection and then fail to store it.
func Load() (Config, error) { return load(true) }

// LoadForMigrate reads only what applying the schema needs: the database and
// the logger. The migrate hook runs before the app's own secrets have to
// exist, and a schema change should not fail because a storage key is unset.
func LoadForMigrate() (Config, error) { return load(false) }

func load(serving bool) (Config, error) {
	c := Config{
		Env:                env("GO_ENV", "development"),
		ListenAddr:         env("LISTEN_ADDR", ":8095"),
		PublicURL:          strings.TrimSuffix(env("PUBLIC_URL", ""), "/"),
		DatabaseURL:        env("DATABASE_URL", ""),
		TokenEncryptionKey: env("TOKEN_ENCRYPTION_KEY", ""),
		S3Endpoint:         env("S3_ENDPOINT", ""),
		// R2 accepts "auto" and nothing else meaningful; MinIO ignores it.
		S3Region:             env("S3_REGION", "auto"),
		S3Bucket:             env("S3_BUCKET", ""),
		S3AccessKey:          env("S3_ACCESS_KEY", ""),
		S3SecretKey:          env("S3_SECRET_KEY", ""),
		LinkedInClientID:     env("LINKEDIN_CLIENT_ID", ""),
		LinkedInClientSecret: env("LINKEDIN_CLIENT_SECRET", ""),
		ThreadsAppID:         env("THREADS_APP_ID", ""),
		ThreadsAppSecret:     env("THREADS_APP_SECRET", ""),
		InstagramAppID:       env("INSTAGRAM_APP_ID", ""),
		InstagramAppSecret:   env("INSTAGRAM_APP_SECRET", ""),
		FacebookAppID:        env("FACEBOOK_APP_ID", ""),
		FacebookAppSecret:    env("FACEBOOK_APP_SECRET", ""),
		XClientID:            env("X_CLIENT_ID", ""),
		XClientSecret:        env("X_CLIENT_SECRET", ""),
		TikTokClientKey:      env("TIKTOK_CLIENT_KEY", ""),
		TikTokClientSecret:   env("TIKTOK_CLIENT_SECRET", ""),
		PostHogAPIKey:        env("POSTHOG_API_KEY", ""),
		PostHogHost:          strings.TrimSuffix(env("POSTHOG_HOST", "https://eu.i.posthog.com"), "/"),
		Mascot3DModelURL:     env("MASCOT_3D_MODEL_URL", ""),
		AppleBundleIDs:       envList("APPLE_BUNDLE_IDS"),
		AppleWebClientID:     env("APPLE_WEB_CLIENT_ID", ""),
		LogLevel:             env("LOG_LEVEL", "info"),
		LogFormat:            env("LOG_FORMAT", ""),
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
	if c.ShutdownTimeout, err = envDuration("SHUTDOWN_TIMEOUT", 30*time.Second); err != nil {
		return Config{}, err
	}
	if c.ShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("config: SHUTDOWN_TIMEOUT must be positive, got %s", c.ShutdownTimeout)
	}
	if c.PublishWorkers < 1 {
		return Config{}, fmt.Errorf("config: PUBLISH_WORKERS must be at least 1, got %d", c.PublishWorkers)
	}

	if (c.LinkedInClientID == "") != (c.LinkedInClientSecret == "") {
		return Config{}, fmt.Errorf("config: set both LINKEDIN_CLIENT_ID and LINKEDIN_CLIENT_SECRET, or neither")
	}
	if (c.ThreadsAppID == "") != (c.ThreadsAppSecret == "") {
		return Config{}, fmt.Errorf("config: set both THREADS_APP_ID and THREADS_APP_SECRET, or neither")
	}
	if (c.InstagramAppID == "") != (c.InstagramAppSecret == "") {
		return Config{}, fmt.Errorf("config: set both INSTAGRAM_APP_ID and INSTAGRAM_APP_SECRET, or neither")
	}
	if (c.FacebookAppID == "") != (c.FacebookAppSecret == "") {
		return Config{}, fmt.Errorf("config: set both FACEBOOK_APP_ID and FACEBOOK_APP_SECRET, or neither")
	}
	if (c.XClientID == "") != (c.XClientSecret == "") {
		return Config{}, fmt.Errorf("config: set both X_CLIENT_ID and X_CLIENT_SECRET, or neither")
	}
	if (c.TikTokClientKey == "") != (c.TikTokClientSecret == "") {
		return Config{}, fmt.Errorf("config: set both TIKTOK_CLIENT_KEY and TIKTOK_CLIENT_SECRET, or neither")
	}

	if c.PublicURL == "" && !c.IsProduction() {
		// Production has no sensible default: a wrong one would send every
		// consent screen back to a host the platform rejects.
		c.PublicURL = "http://localhost:8095"
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

	required := map[string]string{"DATABASE_URL": c.DatabaseURL}
	if serving {
		required["TOKEN_ENCRYPTION_KEY"] = c.TokenEncryptionKey
		required["S3_ENDPOINT"] = c.S3Endpoint
		required["S3_BUCKET"] = c.S3Bucket
		required["S3_ACCESS_KEY"] = c.S3AccessKey
		required["S3_SECRET_KEY"] = c.S3SecretKey
		required["PUBLIC_URL"] = c.PublicURL
	}
	var missing []string
	for name, v := range required {
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

// AppleAudiences are every client id an Apple identity token may be issued to.
func (c Config) AppleAudiences() []string {
	out := append([]string{}, c.AppleBundleIDs...)
	if c.AppleWebClientID != "" {
		out = append(out, c.AppleWebClientID)
	}
	return out
}

// envList reads a comma-separated list, dropping blanks.
func envList(key string) []string {
	var out []string
	for _, v := range strings.Split(os.Getenv(key), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
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

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be a duration such as 30s or 2m, got %q", key, raw)
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
