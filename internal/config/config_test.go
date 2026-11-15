package config

import (
	"strings"
	"testing"
	"time"
)

// clearEnv blanks every setting Load reads, so a developer's own .env or shell
// cannot decide whether a test passes.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"GO_ENV", "PUBLIC_URL", "DATABASE_URL", "TOKEN_ENCRYPTION_KEY",
		"S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY",
		"LINKEDIN_CLIENT_ID", "LINKEDIN_CLIENT_SECRET", "THREADS_APP_ID", "THREADS_APP_SECRET",
		"AUTO_MIGRATE", "S3_USE_SSL", "PUBLISH_WORKERS", "SHUTDOWN_TIMEOUT", "LOG_FORMAT",
	} {
		t.Setenv(k, "")
	}
}

func TestMigrateNeedsOnlyTheDatabase(t *testing.T) {
	clearEnv(t)
	t.Setenv("GO_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://example/db")

	if _, err := LoadForMigrate(); err != nil {
		t.Fatalf("LoadForMigrate with only DATABASE_URL: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load accepted a production environment with no storage, key or public URL")
	}
	for _, name := range []string{"PUBLIC_URL", "S3_BUCKET", "TOKEN_ENCRYPTION_KEY"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("Load error %q does not name %s", err, name)
		}
	}
}

func TestMigrateStillNeedsTheDatabase(t *testing.T) {
	clearEnv(t)
	if _, err := LoadForMigrate(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("LoadForMigrate without DATABASE_URL: got %v, want an error naming it", err)
	}
}

func TestShutdownTimeout(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://example/db")

	c, err := LoadForMigrate()
	if err != nil {
		t.Fatal(err)
	}
	if c.ShutdownTimeout != 30*time.Second {
		t.Errorf("default ShutdownTimeout = %s, want 30s", c.ShutdownTimeout)
	}

	t.Setenv("SHUTDOWN_TIMEOUT", "90s")
	if c, err = LoadForMigrate(); err != nil || c.ShutdownTimeout != 90*time.Second {
		t.Errorf("SHUTDOWN_TIMEOUT=90s: got %s, %v", c.ShutdownTimeout, err)
	}

	for _, bad := range []string{"soon", "0s", "-5s"} {
		t.Setenv("SHUTDOWN_TIMEOUT", bad)
		if _, err := LoadForMigrate(); err == nil {
			t.Errorf("SHUTDOWN_TIMEOUT=%q was accepted", bad)
		}
	}
}
