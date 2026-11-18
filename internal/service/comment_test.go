package service_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"github.com/FACorreiaa/postitheka/internal/jobs"
	"github.com/FACorreiaa/postitheka/internal/service"
)

func (e *env) runComment(t *testing.T, targetID uuid.UUID, attempt int) error {
	t.Helper()
	return e.comment.Work(context.Background(), &river.Job[jobs.CommentArgs]{
		JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: 10},
		Args:   jobs.CommentArgs{TargetID: targetID},
	})
}

func (e *env) commentJobs(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM river_job WHERE kind = 'comment_target'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// postWithComment creates a post with a first comment to one Discord account
// and publishes it.
func (e *env) postWithComment(t *testing.T) (uuid.UUID, uuid.UUID) {
	t.Helper()
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)
	view, err := e.svc.CreatePost(context.Background(), alice, service.CreatePostInput{
		Text: "launch day", FirstComment: "  #launch https://example.com  ", AccountIDs: []uuid.UUID{acct},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := deref(view.Post.FirstComment); got != "#launch https://example.com" {
		t.Errorf("stored comment %q, want it trimmed", got)
	}
	if got := deref(view.Targets[0].CommentStatus); got != "pending" {
		t.Errorf("new target's comment is %q, want pending", got)
	}
	if n := e.commentJobs(t); n != 0 {
		t.Errorf("%d comment jobs before the post is out, want 0", n)
	}
	if err := e.runJob(t, view.Targets[0].ID, 1); err != nil {
		t.Fatal(err)
	}
	return alice, view.Post.ID
}

func TestFirstCommentFollowsThePublishedPost(t *testing.T) {
	e := setup(t)
	alice, postID := e.postWithComment(t)

	tgt := e.target(t, alice, postID)
	if tgt.Status != "published" || deref(tgt.CommentStatus) != "pending" {
		t.Fatalf("after publishing: status %q, comment %q", tgt.Status, deref(tgt.CommentStatus))
	}
	if n := e.commentJobs(t); n != 1 {
		t.Fatalf("%d comment jobs queued with the publish, want 1", n)
	}

	if err := e.runComment(t, tgt.ID, 1); err != nil {
		t.Fatal(err)
	}
	tgt = e.target(t, alice, postID)
	if deref(tgt.CommentStatus) != "posted" || deref(tgt.CommentUrl) != "https://discord.com/channels/g/c/m1" {
		t.Errorf("comment %q at %q", deref(tgt.CommentStatus), deref(tgt.CommentUrl))
	}
	if n := e.discord.posts.Load(); n != 2 {
		t.Errorf("discord got %d messages, want the post and its comment", n)
	}

	// Run again, as at-least-once delivery may: nothing more is sent.
	if err := e.runComment(t, tgt.ID, 2); err != nil {
		t.Fatal(err)
	}
	if n := e.discord.posts.Load(); n != 2 {
		t.Errorf("a repeated comment job sent again (%d messages)", n)
	}
}

func TestAFailedCommentNeverSendsThePostAgain(t *testing.T) {
	e := setup(t)
	alice, postID := e.postWithComment(t)
	tgt := e.target(t, alice, postID)

	e.discord.postStatus.Store(http.StatusBadGateway)
	if err := e.runComment(t, tgt.ID, 1); err == nil {
		t.Fatal("a 502 should be retried")
	}
	tgt = e.target(t, alice, postID)
	if deref(tgt.CommentStatus) != "pending" || deref(tgt.CommentError) == "" {
		t.Errorf("after a 502: comment %q, error %q; want pending with the reason", deref(tgt.CommentStatus), deref(tgt.CommentError))
	}

	e.discord.postStatus.Store(http.StatusBadRequest)
	err := e.runComment(t, tgt.ID, 2)
	if cancel := (*river.JobCancelError)(nil); !errors.As(err, &cancel) {
		t.Errorf("a 400 should end the job, got %v", err)
	}
	tgt = e.target(t, alice, postID)
	if deref(tgt.CommentStatus) != "failed" || tgt.Status != "published" {
		t.Errorf("comment %q, post %q; want the comment failed and the post still published", deref(tgt.CommentStatus), tgt.Status)
	}
	// The post, then two comment attempts. Never the post a second time.
	if n := e.discord.posts.Load(); n != 3 {
		t.Errorf("discord got %d messages, want 3", n)
	}
}

func TestAnInterruptedCommentIsNotSentAgain(t *testing.T) {
	e := setup(t)
	alice, postID := e.postWithComment(t)
	tgt := e.target(t, alice, postID)
	if _, err := e.pool.Exec(context.Background(), "UPDATE post_targets SET comment_status = 'posting' WHERE id = $1", tgt.ID); err != nil {
		t.Fatal(err)
	}

	_ = e.runComment(t, tgt.ID, 2)
	tgt = e.target(t, alice, postID)
	if deref(tgt.CommentStatus) != "failed" || !strings.Contains(deref(tgt.CommentError), "check the platform") {
		t.Errorf("comment %q, error %q", deref(tgt.CommentStatus), deref(tgt.CommentError))
	}
	if n := e.discord.posts.Load(); n != 1 {
		t.Errorf("discord got %d messages, want only the post", n)
	}
}

func TestATooLongCommentIsRefusedAtTheDoor(t *testing.T) {
	e := setup(t)
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)

	_, err := e.svc.CreatePost(context.Background(), alice, service.CreatePostInput{
		Text: "hi", FirstComment: strings.Repeat("#", 2001), AccountIDs: []uuid.UUID{acct},
	})
	var ve *service.ValidationError
	if !errors.As(err, &ve) || len(ve.Issues) != 1 || ve.Issues[0].Field != "first_comment" {
		t.Fatalf("got %v, want one issue on first_comment", err)
	}
}

func TestAPostWithoutACommentQueuesNone(t *testing.T) {
	e := setup(t)
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)
	view, err := e.svc.CreatePost(context.Background(), alice, service.CreatePostInput{
		Text: "hi", FirstComment: "   ", AccountIDs: []uuid.UUID{acct},
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Post.FirstComment != nil || view.Targets[0].CommentStatus != nil {
		t.Errorf("a blank comment was kept: %v, %v", view.Post.FirstComment, view.Targets[0].CommentStatus)
	}
	if err := e.runJob(t, view.Targets[0].ID, 1); err != nil {
		t.Fatal(err)
	}
	if n := e.commentJobs(t); n != 0 {
		t.Errorf("%d comment jobs for a post without a comment", n)
	}
}

// Through a running River client, with no Inserter set: the publish job
// queues the comment with the client River puts in its context.
func TestFirstCommentEndToEnd(t *testing.T) {
	e := setup(t)
	e.worker.Inserter = nil
	ctx := context.Background()
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)
	view, err := e.svc.CreatePost(ctx, alice, service.CreatePostInput{
		Text: "launch day", FirstComment: "#launch", AccountIDs: []uuid.UUID{acct},
	})
	if err != nil {
		t.Fatal(err)
	}

	rc, err := river.NewClient(riverpgxv5.New(e.pool), &river.Config{
		Queues:            map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 2}},
		Workers:           e.workers,
		FetchPollInterval: 200 * time.Millisecond,
		Logger:            slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := rc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rc.Stop(context.Background()) })

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if tgt := e.target(t, alice, view.Post.ID); deref(tgt.CommentStatus) == "posted" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	tgt := e.target(t, alice, view.Post.ID)
	t.Fatalf("after 15s: post %q, comment %q (%s)", tgt.Status, deref(tgt.CommentStatus), deref(tgt.CommentError))
}
