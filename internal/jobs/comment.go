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

	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/publisher"
)

// CommentArgs posts one target's first comment (POST-002). The publish job
// queues it once the post is out, so the comment has its own attempts and its
// own failure: retrying a comment never sends the post again.
type CommentArgs struct {
	TargetID uuid.UUID `json:"target_id"`
}

func (CommentArgs) Kind() string { return "comment_target" }

func (CommentArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		// As for the post: ten attempts ride out roughly a day of outage.
		MaxAttempts: 10,
		// One comment job per target. A second would post the comment twice.
		UniqueOpts: river.UniqueOpts{ByArgs: true},
	}
}

// commentTimeout is our allowance for one comment: a single small request.
const commentTimeout = time.Minute

// CommentWorker shares the publish worker's database, credentials and
// registry, and its refresh: a comment retried hours after its post may find
// an OAuth token that needs renewing.
type CommentWorker struct {
	river.WorkerDefaults[CommentArgs]
	p *PublishWorker
}

func NewCommentWorker(p *PublishWorker) *CommentWorker { return &CommentWorker{p: p} }

func (w *CommentWorker) Timeout(*river.Job[CommentArgs]) time.Duration {
	return commentTimeout + time.Minute
}

// Work follows the publish job's rules: claim before sending, record every
// outcome, and stop rather than retry when an earlier attempt died with the
// request in flight, since a second comment is worse than none.
func (w *CommentWorker) Work(ctx context.Context, job *river.Job[CommentArgs]) error {
	id := job.Args.TargetID
	q := w.p.q

	row, err := q.GetTargetForComment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return river.JobCancel(fmt.Errorf("target %s no longer exists", id))
	}
	if err != nil {
		return err
	}
	if row.CommentStatus == nil {
		return nil
	}
	switch *row.CommentStatus {
	case "posted", "failed":
		return nil
	case "posting":
		return w.fail(ctx, id, errors.New("an earlier attempt was interrupted mid-request; check the platform before commenting again"))
	}
	if row.Status != "published" || row.RemoteID == nil || row.FirstComment == nil {
		return w.fail(ctx, id, errors.New("the post has no published copy to comment under"))
	}
	if !row.AccountID.Valid {
		return w.fail(ctx, id, errors.New("the account was disconnected"))
	}
	if row.NeedsReconnectAt.Valid {
		return w.fail(ctx, id, errors.New("the account needs reconnecting"))
	}
	pub, ok := w.p.registry.Get(publisher.Platform(row.Platform))
	commenter, canComment := pub.(publisher.Commenter)
	if !ok || !canComment {
		return w.fail(ctx, id, fmt.Errorf("%s comments are not available on this server", row.Platform))
	}
	creds, err := w.p.box.Open(row.Credentials, row.AccountID.UUID[:])
	if err != nil {
		return w.fail(ctx, id, errors.New("stored credentials cannot be decrypted; reconnect the account"))
	}

	creds, err = w.p.refresh(ctx, pub, row.AccountID.UUID, creds)
	if err != nil {
		if publisher.IsPermanent(err) {
			return w.fail(ctx, id, fmt.Errorf("%w; reconnect the account", err))
		}
		return w.retry(ctx, job, id, err)
	}

	claimed, err := q.MarkCommentPosting(ctx, id)
	if err != nil {
		return err
	}
	if claimed == 0 {
		return nil // another run of this job got here first
	}

	cctx, cancel := context.WithTimeout(ctx, commentTimeout)
	res, sendErr := commenter.Comment(cctx, creds, *row.RemoteID, *row.FirstComment)
	cancel()
	// As in the publish job: a definite answer is recorded even past the
	// job's deadline, or the next attempt would take it for interrupted.
	ctx = context.WithoutCancel(ctx)

	if sendErr == nil {
		w.p.log.Info("commented", slog.String("target", id.String()), slog.String("platform", row.Platform))
		return q.MarkCommentPosted(ctx, db.MarkCommentPostedParams{ID: id, RemoteID: nilIfEmpty(res.RemoteID), Url: nilIfEmpty(res.URL)})
	}
	w.p.log.Warn("comment failed",
		slog.String("target", id.String()),
		slog.String("platform", row.Platform),
		slog.Int("attempt", job.Attempt),
		slog.Any("error", sendErr))
	if publisher.NeedsReconnect(sendErr) {
		if err := q.MarkAccountNeedsReconnect(ctx, row.AccountID.UUID); err != nil {
			w.p.log.Error("account not marked as needing reconnection", slog.String("account", row.AccountID.UUID.String()), slog.Any("error", err))
		}
		return w.fail(ctx, id, sendErr)
	}
	if publisher.IsPermanent(sendErr) {
		return w.fail(ctx, id, sendErr)
	}
	return w.retry(ctx, job, id, sendErr)
}

// retry puts the comment back to pending with the error, so a client polling
// the post sees why it waits, or fails it once the attempts are spent.
func (w *CommentWorker) retry(ctx context.Context, job *river.Job[CommentArgs], id uuid.UUID, cause error) error {
	if job.Attempt >= job.MaxAttempts {
		if err := w.p.q.MarkCommentFailed(ctx, db.MarkCommentFailedParams{ID: id, CommentError: new(cause.Error())}); err != nil {
			return err
		}
		return cause
	}
	if err := w.p.q.MarkCommentPending(ctx, db.MarkCommentPendingParams{ID: id, CommentError: new(cause.Error())}); err != nil {
		return err
	}
	if ra := (*publisher.RetryAfterError)(nil); errors.As(cause, &ra) {
		return river.JobSnooze(ra.After)
	}
	return cause
}

// fail records a comment that cannot succeed and cancels the job.
func (w *CommentWorker) fail(ctx context.Context, id uuid.UUID, cause error) error {
	if err := w.p.q.MarkCommentFailed(ctx, db.MarkCommentFailedParams{ID: id, CommentError: new(cause.Error())}); err != nil {
		return err
	}
	return river.JobCancel(cause)
}
