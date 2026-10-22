// Package service is the application's behaviour, independent of HTTP: what
// it means to connect an account, reserve an upload, or create a post. The
// handlers in httpapi translate to and from the wire format and nothing else.
package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/media"
	"github.com/FACorreiaa/postitheka/internal/publisher"
	"github.com/FACorreiaa/postitheka/internal/secretbox"
)

var ErrNotFound = errors.New("not found")

// BadRequest is input the caller can fix. Its message is shown to the user.
type BadRequest struct{ Message string }

func (e *BadRequest) Error() string { return e.Message }

// Storage is the slice of media.Store the service uses.
type Storage interface {
	PresignUpload(ctx context.Context, key string, ttl time.Duration) (string, error)
	Stat(ctx context.Context, key string) (media.Object, error)
}

// DiscordConnector checks a webhook URL with Discord. The Discord publisher
// implements it.
type DiscordConnector interface {
	Connect(ctx context.Context, webhookURL string) (publisher.Connection, error)
}

type Service struct {
	pool     *pgxpool.Pool
	q        *db.Queries
	river    *river.Client[pgx.Tx]
	box      *secretbox.Box
	registry *publisher.Registry
	storage  Storage
	discord  DiscordConnector
	now      func() time.Time
}

type Deps struct {
	Pool     *pgxpool.Pool
	River    *river.Client[pgx.Tx]
	Box      *secretbox.Box
	Registry *publisher.Registry
	Storage  Storage
	Discord  DiscordConnector
}

func New(d Deps) *Service {
	return &Service{
		pool:     d.Pool,
		q:        db.New(d.Pool),
		river:    d.River,
		box:      d.Box,
		registry: d.Registry,
		storage:  d.Storage,
		discord:  d.Discord,
		now:      time.Now,
	}
}

func (s *Service) Platforms() []publisher.Publisher { return s.registry.All() }
