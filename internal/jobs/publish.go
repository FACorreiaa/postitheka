// Package jobs holds the background work: one River job per post target, so
// each platform is retried, rate-limited and failed independently of the
// others.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/FACorreiaa/postitheka/internal/analytics"
	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/media"
	"github.com/FACorreiaa/postitheka/internal/publisher"
	"github.com/FACorreiaa/postitheka/internal/secretbox"
)

type PublishArgs struct {
	TargetID uuid.UUID `json:"target_id"`
	// Retry is how many times the user has sent the target again (WEB-001).
	// Each retry is a new job, and River keeps one job per set of arguments,
	// completed ones included, so without it a retry would be dropped as a
	// duplicate of the job that already ran. Omitted when zero, so a first
	// attempt's arguments are what they always were.
	Retry int32 `json:"retry,omitempty"`
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

	pool     *pgxpool.Pool
	q        *db.Queries
	box      *secretbox.Box
	registry *publisher.Registry
	media    publisher.MediaSource
	events   analytics.Client
	log      *slog.Logger
	now      func() time.Time

	// Inserter queues the first comment's job in the transaction that marks
	// the post published. River puts its client in a job's context, and that
	// is what is used; tests that call Work directly set this instead.
	Inserter TxInserter
}

// TxInserter is the part of River's client that queues a job in a
// transaction of the caller's.
type TxInserter interface {
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// NewPublishWorker builds the worker. events may be nil, which records nothing.
func NewPublishWorker(pool *pgxpool.Pool, box *secretbox.Box, registry *publisher.Registry, src publisher.MediaSource, events analytics.Client, log *slog.Logger) *PublishWorker {
	if events == nil {
		events = analytics.Nop()
	}
	return &PublishWorker{pool: pool, q: db.New(pool), box: box, registry: registry, media: src, events: events, log: log, now: time.Now}
}

// Why a target failed, as sent to analytics. Coarse on purpose: the error
// text can echo what a platform returned, and the funnel only needs to know
// which kind of failure is common.
const (
	failInterrupted  = "interrupted"
	failDisconnected = "disconnected"
	failUnavailable  = "platform_unavailable"
	failCredentials  = "credentials"
	failReconnect    = "needs_reconnect"
	failRejected     = "rejected"
	failExhausted    = "retries_exhausted"
)

// How long one publish may take. These are our own allowances, not platform
// limits, and no real upload has been timed yet (2026-11-15).
const (
	// publishBase covers everything that does not grow with the files: the
	// platform's own processing (the publishers wait up to four minutes for
	// a video) and the requests around it.
	publishBase = 5 * time.Minute
	// publishBytesPerSecond is the slowest transfer still treated as healthy.
	// LinkedIn and Discord are sent the bytes by this server; the platforms
	// that fetch from a URL get the same allowance, which costs nothing.
	publishBytesPerSecond = 1 << 20
	// maxPublishTime is the ceiling for any post. It must stay well under
	// River's hour before a running job is taken for stuck and run again.
	maxPublishTime = 30 * time.Minute
)

// publishBudget is how long a post's publish may run: a text post that hangs
// frees its worker after publishBase, while a 500 MiB video gets the minutes
// its upload needs.
func publishBudget(post publisher.Post) time.Duration {
	var size int64
	for _, m := range post.Media {
		size += m.Size
	}
	budget := publishBase + time.Duration(size/publishBytesPerSecond)*time.Second
	if budget > maxPublishTime || budget < publishBase { // the second guards overflow
		return maxPublishTime
	}
	return budget
}

// Timeout is River's limit on the whole job: the longest publish, plus the
// database work before and after it. Work sets the tighter limit per post.
func (w *PublishWorker) Timeout(*river.Job[PublishArgs]) time.Duration {
	return maxPublishTime + time.Minute
}

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
	case "published", "drafted", "failed":
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
	if row.NeedsReconnectAt.Valid {
		// The platform already refused these credentials. Asking again would
		// fail the same way, and repeated bad logins are what gets an app
		// throttled.
		return w.fail(ctx, row, failReconnect, errors.New("the account needs reconnecting; connect it again, then send this again"))
	}
	pub, ok := w.registry.Get(publisher.Platform(row.Platform))
	if !ok {
		return w.fail(ctx, row, failUnavailable, fmt.Errorf("%s publishing is not available on this server", row.Platform))
	}
	creds, err := w.box.Open(row.Credentials, row.AccountID.UUID[:])
	if err != nil {
		w.needsReconnect(ctx, row)
		return w.fail(ctx, row, failCredentials, errors.New("stored credentials cannot be decrypted; reconnect the account"))
	}

	// Before the claim: a refresh that fails leaves the target pending, not
	// 'publishing', since nothing was sent to the platform yet.
	creds, err = w.refresh(ctx, pub, row.AccountID.UUID, creds)
	switch {
	case err == nil:
	case publisher.IsPermanent(err):
		// A refresh the platform refused for good: the grant is gone.
		w.needsReconnect(ctx, row)
		return w.fail(ctx, row, failCredentials, fmt.Errorf("%w; reconnect the account", err))
	default:
		w.log.Warn("refresh failed", slog.String("target", id.String()), slog.String("platform", row.Platform), slog.Any("error", err))
		if job.Attempt >= job.MaxAttempts {
			return w.fail(ctx, row, failExhausted, err)
		}
		if markErr := w.retryLater(ctx, id, err); markErr != nil {
			return markErr
		}
		if ra := (*publisher.RetryAfterError)(nil); errors.As(err, &ra) {
			return river.JobSnooze(ra.After)
		}
		return err
	}

	mediaRows, err := w.q.ListPostMedia(ctx, []uuid.UUID{row.PostID})
	if err != nil {
		return err
	}
	post := publisher.Post{Text: row.Text}
	// Checked when the post was made; a value that cannot be read now is
	// published without, as the post would be had it never been set.
	_ = json.Unmarshal(row.Options, &post.Options)
	for _, m := range mediaRows {
		length, width, height := media.Declared(m.DurationMs, m.Width, m.Height)
		post.Media = append(post.Media, publisher.Media{
			Key:         m.ObjectKey,
			ContentType: m.ContentType,
			Size:        m.SizeBytes,
			Filename:    media.Filename(m.ID, m.ContentType),
			Duration:    length,
			Width:       width,
			Height:      height,
		})
	}

	claimed, err := w.q.MarkTargetPublishing(ctx, id)
	if err != nil {
		return err
	}
	if claimed == 0 {
		return nil // another run of this job got here first
	}

	// Reaching the limit is an unclassified error, so the target is retried;
	// a publisher that cannot tell whether the post went out says so itself.
	pubCtx, cancel := context.WithTimeout(ctx, publishBudget(post))
	res, pubErr := pub.Publish(pubCtx, creds, post, w.media)
	cancel()

	// The outcome is recorded even if the job's own deadline has just passed:
	// a target left in 'publishing' after a definite answer would be treated
	// as interrupted on the next attempt.
	ctx = context.WithoutCancel(ctx)

	if pubErr == nil {
		w.log.Info("published", slog.String("target", id.String()), slog.String("platform", row.Platform))
		if err := w.published(ctx, id, res); err != nil {
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
	case publisher.NeedsReconnect(pubErr):
		w.needsReconnect(ctx, row)
		return w.fail(ctx, row, failReconnect, pubErr)
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

// published records the post as out and, when a first comment waits for it,
// queues the comment's job in the same transaction: there is no moment where
// the post is published and its comment has no job to send it.
//
// A post the platform holds as a draft (TikTok) has nothing public to
// comment under, so its comment fails here with the reason.
func (w *PublishWorker) published(ctx context.Context, id uuid.UUID, res publisher.Result) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := w.q.WithTx(tx)

	comment, err := qtx.MarkTargetPublished(ctx, db.MarkTargetPublishedParams{
		ID:        id,
		RemoteID:  nilIfEmpty(res.RemoteID),
		RemoteUrl: nilIfEmpty(res.URL),
		Draft:     res.Draft,
	})
	if err != nil {
		return err
	}
	if comment != nil && *comment == "pending" {
		if res.Draft {
			err = qtx.MarkCommentFailed(ctx, db.MarkCommentFailedParams{ID: id, CommentError: new("the platform holds the post as a draft, so there is nothing to comment under yet")})
		} else {
			err = w.queueComment(ctx, tx, id)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (w *PublishWorker) queueComment(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var ins TxInserter = w.Inserter
	if client, err := river.ClientFromContextSafely[pgx.Tx](ctx); err == nil {
		ins = client
	}
	if ins == nil {
		return errors.New("queue comment: no River client")
	}
	if _, err := ins.InsertTx(ctx, tx, CommentArgs{TargetID: id}, nil); err != nil {
		return fmt.Errorf("queue comment: %w", err)
	}
	return nil
}

// refresh renews an account's credentials when its publisher says they need
// it, and returns the credentials to publish with.
//
// The account row stays locked from reading the credentials to storing the
// new ones. A job that waited for the lock reads what the first one stored,
// finds it fresh, and does not refresh again. If storing fails after the
// platform issued new credentials, a rotating refresh token is lost and the
// account must be reconnected; holding the lock keeps that to this one
// failure instead of a race on every concurrent post.
func (w *PublishWorker) refresh(ctx context.Context, pub publisher.Publisher, accountID uuid.UUID, creds []byte) ([]byte, error) {
	r, ok := pub.(publisher.Refresher)
	if !ok || !r.NeedsRefresh(creds, w.now()) {
		return creds, nil
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := w.q.WithTx(tx)

	sealed, err := qtx.LockAccountCredentials(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, publisher.Permanent(errors.New("the account was disconnected"))
	}
	if err != nil {
		return nil, err
	}
	current, err := w.box.Open(sealed, accountID[:])
	if err != nil {
		return nil, publisher.Permanent(errors.New("stored credentials cannot be decrypted"))
	}
	if !r.NeedsRefresh(current, w.now()) {
		return current, tx.Commit(ctx)
	}

	fresh, err := r.Refresh(ctx, current)
	if err != nil {
		return nil, err
	}
	if err := qtx.UpdateAccountCredentials(ctx, db.UpdateAccountCredentialsParams{ID: accountID, Credentials: w.box.Seal(fresh, accountID[:])}); err != nil {
		return nil, fmt.Errorf("store refreshed credentials: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		w.log.Error("refreshed credentials were not stored; the account may need reconnecting", slog.String("account", accountID.String()), slog.Any("error", err))
		return nil, fmt.Errorf("store refreshed credentials: %w", err)
	}
	return fresh, nil
}

// needsReconnect marks the target's account as needing reconnection. Best
// effort: the target fails either way, and the next post to the account marks
// it if this did not.
func (w *PublishWorker) needsReconnect(ctx context.Context, row db.GetTargetForPublishRow) {
	if err := w.q.MarkAccountNeedsReconnect(ctx, row.AccountID.UUID); err != nil {
		w.log.Error("account not marked as needing reconnection", slog.String("account", row.AccountID.UUID.String()), slog.Any("error", err))
	}
}

// fail records a terminal failure and cancels the job, so River does not
// retry what cannot succeed.
func (w *PublishWorker) fail(ctx context.Context, row db.GetTargetForPublishRow, reason string, cause error) error {
	if err := w.q.MarkTargetFailed(ctx, db.MarkTargetFailedParams{ID: row.ID, LastError: new(cause.Error()), Interrupted: reason == failInterrupted}); err != nil {
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
