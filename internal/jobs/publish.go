// Package jobs holds the background work: one River job per post target, so
// each platform is retried, rate-limited and failed independently of the
// others.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/FACorreiaa/postitheka/internal/analytics"
	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/media"
	"github.com/FACorreiaa/postitheka/internal/publisher"
	"github.com/FACorreiaa/postitheka/internal/secretbox"
)

type PublishArgs struct {
	TargetID uuid.UUID `json:"target_id"`
}

func (PublishArgs) Kind() string { return "publish_target" }

func (PublishArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// River's backoff grows with the attempt number; ten attempts span
		// roughly a day, long enough to ride out a platform outage.
		MaxAttempts: 10,
		// One job per target, ever. A second insert for the same target is a
		// bug that would post twice.
		UniqueOpts: river.UniqueOpts{ByArgs: true},
	}
}

type PublishWorker struct {
	river.WorkerDefaults[PublishArgs]

	q        *db.Queries
	box      *secretbox.Box
	registry *publisher.Registry
	media    publisher.MediaSource
	events   analytics.Client
	log      *slog.Logger
}

// NewPublishWorker builds the worker. events may be nil, which records nothing.
func NewPublishWorker(q *db.Queries, box *secretbox.Box, registry *publisher.Registry, src publisher.MediaSource, events analytics.Client, log *slog.Logger) *PublishWorker {
	if events == nil {
		events = analytics.Nop()
	}
	return &PublishWorker{q: q, box: box, registry: registry, media: src, events: events, log: log}
}

// Why a target failed, as sent to analytics. Coarse on purpose: the error
// text can echo what a platform returned, and the funnel only needs to know
// which kind of failure is common.
const (
	failInterrupted  = "interrupted"
	failDisconnected = "disconnected"
	failUnavailable  = "platform_unavailable"
	failCredentials  = "credentials"
	failRejected     = "rejected"
	failExhausted    = "retries_exhausted"
)

// Timeout covers the slowest legitimate job: ten 20 MiB files streamed from
// storage to Discord.
func (w *PublishWorker) Timeout(*river.Job[PublishArgs]) time.Duration { return 5 * time.Minute }

func (w *PublishWorker) Work(ctx context.Context, job *river.Job[PublishArgs]) error {
	id := job.Args.TargetID

	row, err := w.q.GetTargetForPublish(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(fmt.Errorf("target %s no longer exists", id))
	}
	if err != nil {
		return err
	}

	switch row.Status {
	case "published", "failed":
		return nil
	case "publishing":
		// The previous attempt claimed the target and never recorded an
		// outcome: the process died with a request in flight. The post may
		// well be live. A duplicate on someone's timeline is worse than a
		// failure they can retry by hand, so stop here and say why.
		return w.fail(ctx, row, failInterrupted, errors.New("an earlier attempt was interrupted mid-request; check the platform before posting again"))
	}

	if !row.AccountID.Valid {
		return w.fail(ctx, row, failDisconnected, errors.New("the account was disconnected"))
	}
	pub, ok := w.registry.Get(publisher.Platform(row.Platform))
	if !ok {
		return w.fail(ctx, row, failUnavailable, fmt.Errorf("%s publishing is not available on this server", row.Platform))
	}
	creds, err := w.box.Open(row.Credentials, row.AccountID.UUID[:])
	if err != nil {
		return w.fail(ctx, row, failCredentials, errors.New("stored credentials cannot be decrypted; reconnect the account"))
	}

	mediaRows, err := w.q.ListPostMedia(ctx, []uuid.UUID{row.PostID})
	if err != nil {
		return err
	}
	post := publisher.Post{Text: row.Text}
	for _, m := range mediaRows {
		post.Media = append(post.Media, publisher.Media{
			Key:         m.ObjectKey,
			ContentType: m.ContentType,
			Size:        m.SizeBytes,
			Filename:    media.Filename(m.ID, m.ContentType),
		})
	}

	claimed, err := w.q.MarkTargetPublishing(ctx, id)
	if err != nil {
		return err
	}
	if claimed == 0 {
		return nil // another run of this job got here first
	}

	res, pubErr := pub.Publish(ctx, creds, post, w.media)

	// The outcome is recorded even if the job's own deadline has just passed:
	// a target left in 'publishing' after a definite answer would be treated
	// as interrupted on the next attempt.
	ctx = context.WithoutCancel(ctx)

	if pubErr == nil {
		w.log.Info("published", slog.String("target", id.String()), slog.String("platform", row.Platform))
		if err := w.q.MarkTargetPublished(ctx, db.MarkTargetPublishedParams{
			ID:        id,
			RemoteID:  nilIfEmpty(res.RemoteID),
			RemoteUrl: nilIfEmpty(res.URL),
		}); err != nil {
			return err
		}
		w.capture(row, analytics.EventTargetPublished, "attempt", job.Attempt)
		return nil
	}

	w.log.Warn("publish failed",
		slog.String("target", id.String()),
		slog.String("platform", row.Platform),
		slog.Int("attempt", job.Attempt),
		slog.Any("error", pubErr))

	var ra *publisher.RetryAfterError
	switch {
	case publisher.IsPermanent(pubErr):
		return w.fail(ctx, row, failRejected, pubErr)
	case job.Attempt >= job.MaxAttempts:
		if err := w.q.MarkTargetFailed(ctx, db.MarkTargetFailedParams{ID: id, LastError: new(pubErr.Error())}); err != nil {
			return err
		}
		w.capture(row, analytics.EventTargetFailed, "reason", failExhausted, "attempt", job.Attempt)
		return pubErr
	case errors.As(pubErr, &ra):
		if err := w.retryLater(ctx, id, pubErr); err != nil {
			return err
		}
		return river.JobSnooze(ra.After)
	default:
		if err := w.retryLater(ctx, id, pubErr); err != nil {
			return err
		}
		return pubErr
	}
}

// fail records a terminal failure and cancels the job, so River does not
// retry what cannot succeed.
func (w *PublishWorker) fail(ctx context.Context, row db.GetTargetForPublishRow, reason string, cause error) error {
	if err := w.q.MarkTargetFailed(ctx, db.MarkTargetFailedParams{ID: row.ID, LastError: new(cause.Error())}); err != nil {
		return err
	}
	w.capture(row, analytics.EventTargetFailed, "reason", reason)
	return river.JobCancel(cause)
}

func (w *PublishWorker) capture(row db.GetTargetForPublishRow, name string, props ...any) {
	w.events.Capture(analytics.Event{
		Name:       name,
		DistinctID: row.UserID.String(),
		Props:      analytics.Props(append([]any{"platform", row.Platform}, props...)...),
	})
}

// retryLater puts the target back to pending with the error, so a client
// polling the post sees why it is waiting.
func (w *PublishWorker) retryLater(ctx context.Context, id uuid.UUID, cause error) error {
	return w.q.MarkTargetPending(ctx, db.MarkTargetPendingParams{ID: id, LastError: new(cause.Error())})
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
