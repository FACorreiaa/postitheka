// Package testdb gives a test its own migrated database.
//
// Tests run against real Postgres rather than a mock, because the things worth
// testing here are the things a mock would invent: that a query is scoped to
// its owner, that the job row and the post commit together, that a conditional
// update really refuses a second claim.
package testdb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FACorreiaa/postitheka/internal/database"
)

// New creates a uniquely named database, migrates it, and drops it when the
// test finishes.
//
// It skips when no database is configured, so `go test ./...` stays useful on
// a machine without Postgres, except in CI, where a skip would pass the build
// with none of these tests run. Once one is configured it fails rather than
// skips on a connection error: silently skipping is how the tests that check
// row scoping quietly stop running.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()

	adminURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if adminURL == "" {
		adminURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if adminURL == "" && os.Getenv("CI") != "" {
		t.Fatal("testdb: CI is set but no TEST_DATABASE_URL or DATABASE_URL; database tests would be skipped")
	}
	if adminURL == "" {
		t.Skip("no TEST_DATABASE_URL or DATABASE_URL set; skipping database test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	name := "postitheka_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("testdb: cannot reach the database (is `task db:up` running?): %v", err)
	}
	if _, err := admin.Exec(ctx, fmt.Sprintf("CREATE DATABASE %q", name)); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("testdb: create database: %v", err)
	}
	_ = admin.Close(ctx)

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("testdb: parse url: %v", err)
	}
	u.Path = "/" + name
	dbURL := u.String()

	t.Cleanup(func() { drop(t, adminURL, name) })

	if err := database.Migrate(ctx, dbURL); err != nil {
		t.Fatalf("testdb: migrate: %v", err)
	}
	pool, err := database.ConnectSmall(ctx, dbURL)
	if err != nil {
		t.Fatalf("testdb: connect: %v", err)
	}
	// Registered after drop, so it runs first: cleanups are LIFO.
	t.Cleanup(pool.Close)
	return pool
}

func drop(t *testing.T, adminURL, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Logf("testdb: drop %s: %v", name, err)
		return
	}
	defer func() { _ = admin.Close(ctx) }()
	if _, err := admin.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %q WITH (FORCE)", name)); err != nil {
		t.Logf("testdb: drop %s: %v", name, err)
	}
}
