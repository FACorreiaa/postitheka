// Command server is the Postitheka API and its publish workers, in one
// process.
//
//	server                     serve the API and run publish jobs
//	server migrate             apply the schema and exit
//	server user create EMAIL   create a user (or find one) and print a new API token
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/FACorreiaa/postitheka/internal/analytics"
	"github.com/FACorreiaa/postitheka/internal/config"
	"github.com/FACorreiaa/postitheka/internal/database"
	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/httpapi"
	"github.com/FACorreiaa/postitheka/internal/jobs"
	"github.com/FACorreiaa/postitheka/internal/media"
	"github.com/FACorreiaa/postitheka/internal/publisher"
	"github.com/FACorreiaa/postitheka/internal/publisher/discord"
	"github.com/FACorreiaa/postitheka/internal/secretbox"
	"github.com/FACorreiaa/postitheka/internal/service"
	"github.com/FACorreiaa/postitheka/web"
)

func main() {
	// os.Exit only here. Calling it from deeper down would skip every
	// deferred close, including the connection pool's.
	if err := dispatch(os.Args[1:]); err != nil {
		slog.Error("fatal", slog.Any("error", err))
		os.Exit(1)
	}
}

func dispatch(args []string) error {
	// A missing .env is normal in-cluster, where the environment is set by
	// the deployment.
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.SetDefault(newLogger(cfg))

	switch {
	case len(args) == 0:
		return serve(cfg)
	case args[0] == "migrate":
		// A subcommand rather than a second binary, so the schema applied is
		// always the one this image was built with. In-cluster it runs as a
		// PreSync hook with AUTO_MIGRATE=false.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := database.Migrate(ctx, cfg.DatabaseURL); err != nil {
			return err
		}
		slog.Info("migrations applied")
		return nil
	case len(args) == 3 && args[0] == "user" && args[1] == "create":
		return createUser(cfg, args[2])
	default:
		return fmt.Errorf("unknown command %q; want no arguments, \"migrate\", or \"user create EMAIL\"", strings.Join(args, " "))
	}
}

func serve(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.AutoMigrate {
		if err := database.Migrate(ctx, cfg.DatabaseURL); err != nil {
			return err
		}
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	box, err := secretbox.NewFromBase64(cfg.TokenEncryptionKey)
	if err != nil {
		return fmt.Errorf("TOKEN_ENCRYPTION_KEY: %w", err)
	}

	store, err := media.New(media.Options{
		Endpoint:  cfg.S3Endpoint,
		Region:    cfg.S3Region,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		UseSSL:    cfg.S3UseSSL,
	})
	if err != nil {
		return err
	}
	if !cfg.IsProduction() {
		// The Vite dev server and the API itself, the two places the web app
		// is served from locally.
		if err := store.EnsureBucket(ctx, []string{"http://localhost:5173", "http://localhost:8095"}); err != nil {
			return err
		}
	}

	// Every platform this server publishes to is registered here, and only
	// here. GET /v1/platforms lists exactly this set.
	discordPub := discord.New(nil)
	registry := publisher.NewRegistry(discordPub)

	workers := river.NewWorkers()
	events := analytics.New(cfg.PostHogAPIKey, cfg.PostHogHost, slog.Default())
	river.AddWorker(workers, jobs.NewPublishWorker(db.New(pool), box, registry, store, events, slog.Default()))
	river.AddWorker(workers, jobs.NewSweepMediaWorker(pool, store, slog.Default()))
	river.AddWorker(workers, jobs.NewExpireHandoffsWorker(db.New(pool), slog.Default()))

	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.PublishWorkers}},
		Workers:      workers,
		PeriodicJobs: []*river.PeriodicJob{jobs.PeriodicSweepMedia(), jobs.PeriodicExpireHandoffs()},
		Logger:       slog.Default(),
	})
	if err != nil {
		return fmt.Errorf("river: %w", err)
	}
	if err := rc.Start(ctx); err != nil {
		return fmt.Errorf("river start: %w", err)
	}

	svc := service.New(service.Deps{
		Pool:      pool,
		River:     rc,
		Box:       box,
		Registry:  registry,
		Storage:   store,
		Discord:   discordPub,
		Analytics: events,
	})

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           routes(pool, svc),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", slog.String("addr", cfg.ListenAddr), slog.String("env", cfg.Env))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	httpErr := srv.Shutdown(shutdownCtx)
	// River's Stop waits for running jobs. A job cut off mid-request would
	// leave its target 'publishing' and be failed as interrupted, so letting
	// in-flight posts finish is worth most of the shutdown budget.
	riverErr := rc.Stop(shutdownCtx)
	// After River, so the outcomes of the last jobs are flushed too. Bounded
	// by the same budget: a hung analytics endpoint cannot hold up a deploy.
	_ = events.Close(shutdownCtx)
	return errors.Join(httpErr, riverErr)
}

func routes(pool *pgxpool.Pool, svc *service.Service) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)

	// Outside the API spec and its authentication: the kubelet probes this.
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			http.Error(w, "database unreachable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})

	httpapi.Mount(r, svc, slog.Default())

	// An unknown API path is a JSON 404, not the web app's HTML: a client
	// calling a route that does not exist should hear so in its own format.
	r.Handle("/v1/*", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"no such endpoint"}`))
	}))
	r.Handle("/*", web.Handler())
	return r
}

func createUser(cfg config.Config, email string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// No river client or storage: minting a token touches neither. Analytics
	// is flushed before exit, so a sign-up from the CLI is not lost.
	events := analytics.New(cfg.PostHogAPIKey, cfg.PostHogHost, slog.Default())
	defer func() { _ = events.Close(ctx) }()
	svc := service.New(service.Deps{Pool: pool, Analytics: events})
	id, token, err := svc.CreateUserWithToken(ctx, email, "cli")
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "user %s\n", id)
	// The token alone on stdout, so it can be piped straight into a secret.
	fmt.Println(token)
	return nil
}

func newLogger(cfg config.Config) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	if cfg.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
