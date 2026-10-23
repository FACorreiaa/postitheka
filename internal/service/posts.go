package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/riverqueue/river"

	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/jobs"
	"github.com/FACorreiaa/postitheka/internal/media"
	"github.com/FACorreiaa/postitheka/internal/publisher"
)

type CreatePostInput struct {
	Text       string
	MediaIDs   []uuid.UUID
	AccountIDs []uuid.UUID
	// Assisted platforms the client hands the post to itself (WhatsApp).
	Assisted    []string
	ScheduledAt *time.Time
	// IdempotencyKey, when set, makes the call safe to retry: a second call
	// with the same key returns the post the first one created.
	IdempotencyKey *string
}

const maxIdempotencyKeyLength = 255

// Issue is one reason a post cannot be created, tied to the target that would
// reject it when there is one.
type Issue struct {
	AccountID *uuid.UUID
	Platform  string
	Field     string
	Message   string
}

// ValidationError lists every issue at once, so a client can mark each
// platform that needs changes in one round trip.
type ValidationError struct{ Issues []Issue }

func (e *ValidationError) Error() string {
	return fmt.Sprintf("post rejected: %d issue(s)", len(e.Issues))
}

// PostView is a post with what a client needs to show its progress.
type PostView struct {
	Post     db.Post
	MediaIDs []uuid.UUID
	Targets  []db.PostTarget
}

// CreatePost validates a post against every target and, only if all of them
// would accept it, stores it and queues one publish job per target.
//
// The post, its targets and its jobs commit in one transaction. That is the
// point of a Postgres-backed queue: there is no window where a post exists
// with no job to send it, or a job exists for a post that rolled back.
//
// With an idempotency key, a repeat is answered before validation: the first
// post may have been accepted under conditions that no longer hold (an
// account since disconnected), and the client is asking about that post, not
// submitting a new one.
func (s *Service) CreatePost(ctx context.Context, userID uuid.UUID, in CreatePostInput) (PostView, error) {
	var hash []byte
	if in.IdempotencyKey != nil {
		if n := len(*in.IdempotencyKey); n == 0 || n > maxIdempotencyKeyLength {
			return PostView{}, &BadRequest{Message: fmt.Sprintf("Idempotency-Key must be 1 to %d characters", maxIdempotencyKeyLength)}
		}
		hash = requestHash(in)
		if view, found, err := s.replay(ctx, userID, *in.IdempotencyKey, hash); found || err != nil {
			return view, err
		}
	}

	var issues []Issue
	addIssue := func(i Issue) { issues = append(issues, i) }

	accountIDs := dedupe(in.AccountIDs)
	assisted := dedupeStrings(in.Assisted)
	if len(accountIDs) == 0 && len(assisted) == 0 {
		return PostView{}, &ValidationError{Issues: []Issue{{Field: "account_ids", Message: "choose at least one account"}}}
	}
	for _, p := range assisted {
		if !publisher.IsAssisted(publisher.Platform(p)) {
			addIssue(Issue{Platform: p, Field: "assisted", Message: p + " is not an assisted platform"})
		}
	}
	// A handoff happens on the device when Publish is pressed; there is no
	// one to hand a scheduled post over at its time.
	if len(assisted) > 0 && in.ScheduledAt != nil && in.ScheduledAt.After(s.now()) {
		addIssue(Issue{Field: "assisted", Message: "a post handed to another app cannot be scheduled"})
	}
	if len(in.MediaIDs) != len(dedupe(in.MediaIDs)) {
		addIssue(Issue{Field: "media_ids", Message: "the same file is attached twice"})
	}

	// No rules are checked for assisted platforms: the server never sends to
	// them, and their own apps enforce their limits.
	accounts, err := s.q.GetAccountsByIDs(ctx, db.GetAccountsByIDsParams{UserID: userID, Ids: accountIDs})
	if err != nil {
		return PostView{}, err
	}
	if len(accounts) != len(accountIDs) {
		addIssue(Issue{Field: "account_ids", Message: "one or more accounts do not exist"})
	}

	pubMedia, mediaIssues, err := s.resolveMedia(ctx, userID, in.MediaIDs)
	if err != nil {
		return PostView{}, err
	}
	issues = append(issues, mediaIssues...)

	post := publisher.Post{Text: in.Text, Media: pubMedia}
	for _, a := range accounts {
		accountID := a.ID
		pub, ok := s.registry.Get(publisher.Platform(a.Platform))
		if !ok {
			addIssue(Issue{AccountID: &accountID, Platform: a.Platform, Field: "account_ids", Message: a.Platform + " is not available"})
			continue
		}
		// Media issues are already reported once above; validating a post
		// whose media could not be resolved would only add noise.
		if len(mediaIssues) > 0 {
			continue
		}
		for _, vi := range publisher.Validate(pub.Rules(), post) {
			addIssue(Issue{AccountID: &accountID, Platform: a.Platform, Field: vi.Field, Message: vi.Message})
		}
	}

	if len(issues) > 0 {
		return PostView{}, &ValidationError{Issues: issues}
	}

	scheduledAt := s.now()
	if in.ScheduledAt != nil && in.ScheduledAt.After(scheduledAt) {
		scheduledAt = *in.ScheduledAt
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PostView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	row, err := qtx.CreatePost(ctx, db.CreatePostParams{
		UserID:         userID,
		Text:           in.Text,
		ScheduledAt:    scheduledAt,
		IdempotencyKey: in.IdempotencyKey,
		RequestHash:    hash,
	})
	if isIdempotencyConflict(err) {
		// A concurrent request with the same key committed first; this
		// insert waited for it and lost. Answer with its post.
		_ = tx.Rollback(ctx)
		view, found, err := s.replay(ctx, userID, *in.IdempotencyKey, hash)
		if err == nil && !found {
			err = errors.New("create post: idempotency key conflict with no post")
		}
		return view, err
	}
	if err != nil {
		return PostView{}, fmt.Errorf("create post: %w", err)
	}
	for i, id := range in.MediaIDs {
		if err := qtx.AddPostMedia(ctx, db.AddPostMediaParams{PostID: row.ID, MediaID: id, Position: int32(i)}); err != nil {
			return PostView{}, fmt.Errorf("attach media: %w", err)
		}
	}

	view := PostView{Post: row, MediaIDs: in.MediaIDs}
	if view.MediaIDs == nil {
		view.MediaIDs = []uuid.UUID{}
	}
	for _, a := range accounts {
		target, err := qtx.CreatePostTarget(ctx, db.CreatePostTargetParams{
			PostID:    row.ID,
			AccountID: uuid.NullUUID{UUID: a.ID, Valid: true},
			Platform:  a.Platform,
		})
		if err != nil {
			return PostView{}, fmt.Errorf("create target: %w", err)
		}
		if _, err := s.river.InsertTx(ctx, tx, jobs.PublishArgs{TargetID: target.ID}, &river.InsertOpts{ScheduledAt: scheduledAt}); err != nil {
			return PostView{}, fmt.Errorf("queue target: %w", err)
		}
		view.Targets = append(view.Targets, target)
	}
	for _, p := range assisted {
		target, err := qtx.CreateAssistedTarget(ctx, db.CreateAssistedTargetParams{PostID: row.ID, Platform: p})
		if err != nil {
			return PostView{}, fmt.Errorf("create assisted target: %w", err)
		}
		view.Targets = append(view.Targets, target)
	}

	if err := tx.Commit(ctx); err != nil {
		return PostView{}, fmt.Errorf("commit post: %w", err)
	}
	return view, nil
}

// replay returns the post created earlier with this key, if there is one. A key
// sent with a different request is refused: answering with the earlier post
// would tell the client its new post was accepted.
func (s *Service) replay(ctx context.Context, userID uuid.UUID, key string, hash []byte) (PostView, bool, error) {
	row, err := s.q.GetPostByIdempotencyKey(ctx, db.GetPostByIdempotencyKeyParams{UserID: userID, IdempotencyKey: &key})
	if errors.Is(err, pgx.ErrNoRows) {
		return PostView{}, false, nil
	}
	if err != nil {
		return PostView{}, false, err
	}
	if !bytes.Equal(row.RequestHash, hash) {
		return PostView{}, true, &ValidationError{Issues: []Issue{{
			Field:   "Idempotency-Key",
			Message: "this key was already used for a different post",
		}}}
	}
	views, err := s.withDetails(ctx, []db.Post{row})
	if err != nil {
		return PostView{}, true, err
	}
	return views[0], true, nil
}

// requestHash is a digest of what the client asked for, used to tell a retry
// from a different post sent with the same key.
func requestHash(in CreatePostInput) []byte {
	var scheduledAt *time.Time
	if in.ScheduledAt != nil {
		t := in.ScheduledAt.UTC()
		scheduledAt = &t
	}
	b, _ := json.Marshal(struct {
		Text        string      `json:"text"`
		MediaIDs    []uuid.UUID `json:"media_ids"`
		AccountIDs  []uuid.UUID `json:"account_ids"`
		Assisted    []string    `json:"assisted,omitempty"`
		ScheduledAt *time.Time  `json:"scheduled_at"`
	}{in.Text, in.MediaIDs, in.AccountIDs, in.Assisted, scheduledAt})
	sum := sha256.Sum256(b)
	return sum[:]
}

func isIdempotencyConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "posts_user_id_idempotency_key_idx"
}

// resolveMedia loads the attached files in order and confirms each one is in
// storage with the size and type it claims. A file is checked against storage
// once; after that its verified row is trusted.
func (s *Service) resolveMedia(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) ([]publisher.Media, []Issue, error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}

	rows, err := s.q.GetMediaByIDs(ctx, db.GetMediaByIDsParams{UserID: userID, Ids: ids})
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[uuid.UUID]db.Medium, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}

	var out []publisher.Media
	var issues []Issue
	for i, id := range ids {
		field := fmt.Sprintf("media_ids[%d]", i)
		m, ok := byID[id]
		if !ok {
			issues = append(issues, Issue{Field: field, Message: "no such file"})
			continue
		}

		if !m.VerifiedAt.Valid {
			obj, err := s.storage.Stat(ctx, m.ObjectKey)
			if errors.Is(err, media.ErrNotFound) {
				issues = append(issues, Issue{Field: field, Message: "the file has not finished uploading"})
				continue
			}
			if err != nil {
				return nil, nil, err
			}
			// The type comes from the PUT's Content-Type header, which a
			// presigned URL does not pin; it has to match what was declared,
			// or a client could reserve an image and upload anything.
			if !strings.EqualFold(obj.ContentType, m.ContentType) {
				issues = append(issues, Issue{Field: field, Message: fmt.Sprintf("uploaded as %s, declared as %s", obj.ContentType, m.ContentType)})
				continue
			}
			if err := s.q.MarkMediaVerified(ctx, db.MarkMediaVerifiedParams{ID: m.ID, SizeBytes: obj.Size, ContentType: m.ContentType}); err != nil {
				return nil, nil, err
			}
			m.SizeBytes = obj.Size
		}

		out = append(out, publisher.Media{
			Key:         m.ObjectKey,
			ContentType: m.ContentType,
			Size:        m.SizeBytes,
			Filename:    media.Filename(m.ID, m.ContentType),
		})
	}
	return out, issues, nil
}

func (s *Service) GetPost(ctx context.Context, userID, postID uuid.UUID) (PostView, error) {
	row, err := s.q.GetPost(ctx, db.GetPostParams{ID: postID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return PostView{}, ErrNotFound
	}
	if err != nil {
		return PostView{}, err
	}
	views, err := s.withDetails(ctx, []db.Post{row})
	if err != nil {
		return PostView{}, err
	}
	return views[0], nil
}

func (s *Service) ListPosts(ctx context.Context, userID uuid.UUID, limit int) ([]PostView, error) {
	rows, err := s.q.ListPosts(ctx, db.ListPostsParams{UserID: userID, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	return s.withDetails(ctx, rows)
}

// withDetails attaches targets and media to posts with two queries in total,
// not two per post.
func (s *Service) withDetails(ctx context.Context, posts []db.Post) ([]PostView, error) {
	ids := make([]uuid.UUID, len(posts))
	views := make([]PostView, len(posts))
	index := make(map[uuid.UUID]int, len(posts))
	for i, p := range posts {
		ids[i] = p.ID
		views[i] = PostView{Post: p, MediaIDs: []uuid.UUID{}, Targets: []db.PostTarget{}}
		index[p.ID] = i
	}
	if len(posts) == 0 {
		return views, nil
	}

	targets, err := s.q.ListPostTargets(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		v := &views[index[t.PostID]]
		v.Targets = append(v.Targets, t)
	}

	files, err := s.q.ListPostMedia(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		v := &views[index[f.PostID]]
		v.MediaIDs = append(v.MediaIDs, f.ID)
	}
	return views, nil
}

// CompleteHandoff records what happened to an assisted target once the share
// sheet closed. Repeating the outcome already recorded is accepted, so a
// client retrying after a lost response does not see an error; a different
// outcome is a conflict.
func (s *Service) CompleteHandoff(ctx context.Context, userID, targetID uuid.UUID, sent bool) (PostView, error) {
	status, lastError := "handed_off", (*string)(nil)
	if !sent {
		status, lastError = "failed", new("the share sheet closed without sending")
	}
	row, err := s.q.CompleteHandoff(ctx, db.CompleteHandoffParams{ID: targetID, UserID: userID, Status: status, LastError: lastError})
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := s.q.GetTargetForUser(ctx, db.GetTargetForUserParams{ID: targetID, UserID: userID})
		if errors.Is(getErr, pgx.ErrNoRows) {
			return PostView{}, ErrNotFound
		}
		if getErr != nil {
			return PostView{}, getErr
		}
		if current.Status != status || current.AccountID.Valid {
			return PostView{}, ErrConflict
		}
		row = current
	} else if err != nil {
		return PostView{}, err
	}
	return s.GetPost(ctx, userID, row.PostID)
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func dedupe(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]bool, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
