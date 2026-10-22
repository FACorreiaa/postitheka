// Package database owns the connection pool and the schema.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/FACorreiaa/postitheka/migrations"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Connect opens the pool and proves it works before returning.
//
// pgxpool connects lazily, so without the ping a misconfigured DATABASE_URL
// produces a process that starts cleanly and then fails on the first request
// that needs data.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	return connect(ctx, url, 10, 2)
}

// ConnectSmall opens a pool sized for one test rather than a whole
// application, so parallel test packages cannot exhaust Postgres's
// connection limit between them.
func ConnectSmall(ctx context.Context, url string) (*pgxpool.Pool, error) {
	return connect(ctx, url, 4, 0)
}

func connect(ctx context.Context, url string, maxConns, minConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("database: parse config: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database: connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}
	return pool, nil
}

// Migrate applies the application schema and then River's.
//
// River's tables come from River's own migrator rather than being copied into
// migrations/, so upgrading River upgrades its schema with it. Both run from
// the same binary, so a container never applies a schema from another commit.
func Migrate(ctx context.Context, url string) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("migrate: open: %w", err)
	}
	defer func() { _ = db.Close() }()

	// A session-level advisory lock, so two replicas starting at the same
	// moment cannot both apply the same migration.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("migrate: locker: %w", err)
	}
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		migrations.FS,
		goose.WithSessionLocker(locker),
		// Migrations written on parallel branches can merge with timestamps
		// older than one already applied; refusing them blocks a deploy over
		// a filename.
		goose.WithAllowOutofOrder(true),
	)
	if err != nil {
		return fmt.Errorf("migrate: provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: up: %w", err)
	}

	pool, err := ConnectSmall(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("migrate: river: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("migrate: river up: %w", err)
	}
	return nil
}
