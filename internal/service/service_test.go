package service_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"github.com/FACorreiaa/postitheka/internal/analytics"
	"github.com/FACorreiaa/postitheka/internal/database/testdb"
	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/jobs"
	"github.com/FACorreiaa/postitheka/internal/media"
	"github.com/FACorreiaa/postitheka/internal/publisher"
	"github.com/FACorreiaa/postitheka/internal/publisher/discord"
	"github.com/FACorreiaa/postitheka/internal/secretbox"
	"github.com/FACorreiaa/postitheka/internal/service"
)

const webhookURL = "https://discord.com/api/webhooks/42/secret-token"

// fakeStorage is object storage held in memory. put stands in for the
// client's PUT to the presigned URL.
type fakeStorage struct {
	mu      sync.Mutex
	objects map[string]fakeObject
}

type fakeObject struct {
	contentType string
	data        []byte
}

func (f *fakeStorage) put(key, contentType string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = fakeObject{contentType, data}
}

func (f *fakeStorage) PresignUpload(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.test/" + key + "?signed", nil
}

func (f *fakeStorage) Stat(_ context.Context, key string) (media.Object, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.objects[key]
	if !ok {
		return media.Object{}, media.ErrNotFound
	}
	return media.Object{Size: int64(len(o.data)), ContentType: o.contentType}, nil
}

func (f *fakeStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.objects[key]
	if !ok {
		return nil, media.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(o.data)), nil
}

func (f *fakeStorage) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	return nil
}

func (f *fakeStorage) has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[key]
	return ok
}

func (f *fakeStorage) PublicURL(context.Context, string, time.Duration) (string, error) {
	return "", errors.New("unused")
}

// discordStub answers as Discord would. postStatus decides how the next
// message is received; files records what arrived.
type discordStub struct {
	postStatus atomic.Int32
	posts      atomic.Int32
	mu         sync.Mutex
	files      []string
}

func (d *discordStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, `{"id":"42","name":"general","guild_id":"g","channel_id":"c"}`)
		return
	}
	d.posts.Add(1)
	if mt, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "multipart/form-data" {
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(part)
			if part.FileName() != "" {
				d.mu.Lock()
				d.files = append(d.files, string(data))
				d.mu.Unlock()
			}
		}
	}
	status := int(d.postStatus.Load())
	w.WriteHeader(status)
	if status == http.StatusOK {
		_, _ = io.WriteString(w, `{"id":"m1","channel_id":"c"}`)
	} else {
		_, _ = io.WriteString(w, `{"message":"stub failure"}`)
	}
}

type env struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	svc     *service.Service
	worker  *jobs.PublishWorker
	workers *river.Workers
	storage *fakeStorage
	discord *discordStub
	events  *recordedEvents
}

// recordedEvents is an analytics client that keeps what it is given.
type recordedEvents struct {
	mu     sync.Mutex
	events []analytics.Event
}

func (r *recordedEvents) Capture(e analytics.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recordedEvents) Close(context.Context) error { return nil }

// names lists events as "name" or "name:reason", in order.
func (r *recordedEvents) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []string{}
	for _, e := range r.events {
		n := e.Name
		if reason, ok := e.Props["reason"]; ok {
			n += ":" + reason.(string)
		}
		out = append(out, n)
	}
	return out
}

func (r *recordedEvents) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}

func setup(t *testing.T) *env {
	t.Helper()
	pool := testdb.New(t)

	stub := &discordStub{}
	stub.postStatus.Store(http.StatusOK)
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.URL.Scheme, r.URL.Host = target.Scheme, target.Host
		return http.DefaultTransport.RoundTrip(r)
	})}

	box, err := secretbox.New(bytes.Repeat([]byte{7}, secretbox.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	discordPub := discord.New(client)
	registry := publisher.NewRegistry(discordPub)
	storage := &fakeStorage{objects: map[string]fakeObject{}}
	q := db.New(pool)

	events := &recordedEvents{}
	worker := jobs.NewPublishWorker(q, box, registry, storage, events, slog.New(slog.DiscardHandler))
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)

	// Not started: tests that want jobs worked start their own client, and
	// the rest call the worker directly so each outcome is deterministic.
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Workers: workers})
	if err != nil {
		t.Fatal(err)
	}

	svc := service.New(service.Deps{
		Pool: pool, River: rc, Box: box, Registry: registry, Storage: storage, Discord: discordPub, Analytics: events,
	})
	return &env{pool: pool, q: q, svc: svc, worker: worker, workers: workers, storage: storage, discord: stub, events: events}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func (e *env) user(t *testing.T, email string) uuid.UUID {
	t.Helper()
	id, token, err := e.svc.CreateUserWithToken(context.Background(), email, "test")
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.Authenticate(context.Background(), token)
	if err != nil || got != id {
		t.Fatalf("token does not authenticate its own user: %v", err)
	}
	return id
}

func (e *env) account(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	acct, err := e.svc.ConnectDiscord(context.Background(), userID, webhookURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return acct.ID
}

// upload reserves an upload and completes it the way a client would.
func (e *env) upload(t *testing.T, userID uuid.UUID, contentType string, data []byte) uuid.UUID {
	t.Helper()
	up, err := e.svc.ReserveUpload(context.Background(), userID, contentType, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	key := strings.TrimSuffix(strings.TrimPrefix(up.URL, "https://storage.test/"), "?signed")
	e.storage.put(key, contentType, data)
	return up.MediaID
}

func (e *env) runJob(t *testing.T, targetID uuid.UUID, attempt int) error {
	t.Helper()
	return e.worker.Work(context.Background(), &river.Job[jobs.PublishArgs]{
		JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: 10},
		Args:   jobs.PublishArgs{TargetID: targetID},
	})
}

func (e *env) target(t *testing.T, userID, postID uuid.UUID) db.PostTarget {
	t.Helper()
	view, err := e.svc.GetPost(context.Background(), userID, postID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Targets) != 1 {
		t.Fatalf("%d targets, want 1", len(view.Targets))
	}
	return view.Targets[0]
}

// The whole path: connect, upload, post, and a real River client picking the
// job up from the same transaction that created the post.
func TestPostIsPublishedEndToEnd(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)
	img := e.upload(t, alice, "image/png", []byte("PNG-BYTES"))

	view, err := e.svc.CreatePost(ctx, alice, service.CreatePostInput{
		Text: "launch day", MediaIDs: []uuid.UUID{img}, AccountIDs: []uuid.UUID{acct},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Targets) != 1 || view.Targets[0].Status != "pending" {
		t.Fatalf("targets %+v", view.Targets)
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
	var tgt db.PostTarget
	for time.Now().Before(deadline) {
		if tgt = e.target(t, alice, view.Post.ID); tgt.Status == "published" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if tgt.Status != "published" {
		t.Fatalf("target is %q after 15s (last error %v)", tgt.Status, deref(tgt.LastError))
	}
	if deref(tgt.RemoteUrl) != "https://discord.com/channels/g/c/m1" {
		t.Errorf("remote url %q", deref(tgt.RemoteUrl))
	}
	e.discord.mu.Lock()
	defer e.discord.mu.Unlock()
	if len(e.discord.files) != 1 || e.discord.files[0] != "PNG-BYTES" {
		t.Errorf("discord received files %q", e.discord.files)
	}
}

func TestCreatePostReportsEveryIssueAndCreatesNothing(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)

	_, err := e.svc.CreatePost(ctx, alice, service.CreatePostInput{
		Text:       strings.Repeat("a", 2001),
		AccountIDs: []uuid.UUID{acct, uuid.New()},
	})
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("got %v, want a ValidationError", err)
	}
	var got []string
	for _, i := range ve.Issues {
		got = append(got, i.Field)
	}
	if strings.Join(got, ",") != "account_ids,text" {
		t.Errorf("issues on %v, want the unknown account and the text", got)
	}

	var posts, queued int
	_ = e.pool.QueryRow(ctx, "SELECT count(*) FROM posts").Scan(&posts)
	_ = e.pool.QueryRow(ctx, "SELECT count(*) FROM river_job").Scan(&queued)
	if posts != 0 || queued != 0 {
		t.Errorf("rejected post left %d posts and %d jobs behind", posts, queued)
	}
}

func TestCreatePostChecksUploadsAgainstStorage(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)

	notUploaded, err := e.svc.ReserveUpload(ctx, alice, "image/png", 10)
	if err != nil {
		t.Fatal(err)
	}

	// Reserved as an image, uploaded as something else.
	disguised, err := e.svc.ReserveUpload(ctx, alice, "image/png", 10)
	if err != nil {
		t.Fatal(err)
	}
	key := strings.TrimSuffix(strings.TrimPrefix(disguised.URL, "https://storage.test/"), "?signed")
	e.storage.put(key, "application/x-msdownload", []byte("MZ"))

	_, err = e.svc.CreatePost(ctx, alice, service.CreatePostInput{
		Text: "hi", MediaIDs: []uuid.UUID{notUploaded.MediaID, disguised.MediaID}, AccountIDs: []uuid.UUID{acct},
	})
	var ve *service.ValidationError
	if !errors.As(err, &ve) || len(ve.Issues) != 2 {
		t.Fatalf("got %v, want two media issues", err)
	}
	if !strings.Contains(ve.Issues[0].Message, "not finished uploading") || !strings.Contains(ve.Issues[1].Message, "declared as image/png") {
		t.Errorf("issues %+v", ve.Issues)
	}
}

func TestCreatePostIsIdempotent(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)

	counts := func() (posts, queued int) {
		_ = e.pool.QueryRow(ctx, "SELECT count(*) FROM posts").Scan(&posts)
		_ = e.pool.QueryRow(ctx, "SELECT count(*) FROM river_job").Scan(&queued)
		return posts, queued
	}
	create := func(key, text string) (service.PostView, error) {
		return e.svc.CreatePost(ctx, alice, service.CreatePostInput{
			Text: text, AccountIDs: []uuid.UUID{acct}, IdempotencyKey: &key,
		})
	}

	first, err := create("key-1", "hello")
	if err != nil {
		t.Fatal(err)
	}
	again, err := create("key-1", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if again.Post.ID != first.Post.ID || len(again.Targets) != 1 {
		t.Errorf("retry returned post %s with %d targets, want %s with 1", again.Post.ID, len(again.Targets), first.Post.ID)
	}
	if posts, queued := counts(); posts != 1 || queued != 1 {
		t.Errorf("one key made %d posts and %d jobs, want 1 and 1", posts, queued)
	}

	if _, err := create("key-2", "hello"); err != nil {
		t.Fatal(err)
	}
	if posts, queued := counts(); posts != 2 || queued != 2 {
		t.Errorf("two keys made %d posts and %d jobs, want 2 and 2", posts, queued)
	}

	t.Run("a key reused for a different post is refused", func(t *testing.T) {
		_, err := create("key-1", "something else")
		var ve *service.ValidationError
		if !errors.As(err, &ve) || ve.Issues[0].Field != "Idempotency-Key" {
			t.Fatalf("got %v, want an Idempotency-Key issue", err)
		}
	})

	t.Run("keys belong to their user", func(t *testing.T) {
		bob := e.user(t, "bob@example.com")
		key := "key-1"
		view, err := e.svc.CreatePost(ctx, bob, service.CreatePostInput{
			Text: "hello", AccountIDs: []uuid.UUID{e.account(t, bob)}, IdempotencyKey: &key,
		})
		if err != nil || view.Post.ID == first.Post.ID {
			t.Fatalf("bob's post with alice's key: %v, id %s", err, view.Post.ID)
		}
	})

	t.Run("concurrent retries make one post", func(t *testing.T) {
		const n = 8
		ids := make([]uuid.UUID, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() {
				view, err := create("key-race", "race")
				ids[i], errs[i] = view.Post.ID, err
			})
		}
		wg.Wait()
		for i := range n {
			if errs[i] != nil || ids[i] != ids[0] {
				t.Fatalf("request %d: %v, post %s; want every request to get %s", i, errs[i], ids[i], ids[0])
			}
		}
		var posts int
		_ = e.pool.QueryRow(ctx, "SELECT count(*) FROM posts WHERE idempotency_key = 'key-race'").Scan(&posts)
		if posts != 1 {
			t.Errorf("%d concurrent requests made %d posts, want 1", n, posts)
		}
	})

	t.Run("a retry finds the post after its account is gone", func(t *testing.T) {
		if err := e.svc.DeleteAccount(ctx, alice, acct); err != nil {
			t.Fatal(err)
		}
		view, err := create("key-1", "hello")
		if err != nil || view.Post.ID != first.Post.ID {
			t.Fatalf("got %v, post %s; want the original post", err, view.Post.ID)
		}
	})

	t.Run("an empty key is refused", func(t *testing.T) {
		var br *service.BadRequest
		if _, err := create("", "hello"); !errors.As(err, &br) {
			t.Fatalf("got %v, want a BadRequest", err)
		}
	})
}

func TestSweepDeletesOnlyOldUnattachedUploads(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)

	png := []byte("\x89PNG fake")
	oldLoose := e.upload(t, alice, "image/png", png)
	oldAttached := e.upload(t, alice, "image/png", png)
	newLoose := e.upload(t, alice, "image/png", png)
	if _, err := e.svc.CreatePost(ctx, alice, service.CreatePostInput{
		MediaIDs: []uuid.UUID{oldAttached}, AccountIDs: []uuid.UUID{acct},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, "UPDATE media SET created_at = now() - interval '25 hours' WHERE id = ANY($1)",
		[]uuid.UUID{oldLoose, oldAttached}); err != nil {
		t.Fatal(err)
	}
	keys := map[uuid.UUID]string{}
	rows, _ := e.pool.Query(ctx, "SELECT id, object_key FROM media")
	for rows.Next() {
		var id uuid.UUID
		var key string
		_ = rows.Scan(&id, &key)
		keys[id] = key
	}
	rows.Close()

	sweep := jobs.NewSweepMediaWorker(e.pool, e.storage, slog.New(slog.DiscardHandler))
	if err := sweep.Work(ctx, &river.Job[jobs.SweepMediaArgs]{JobRow: &rivertype.JobRow{}}); err != nil {
		t.Fatal(err)
	}

	for id, want := range map[uuid.UUID]bool{oldLoose: false, oldAttached: true, newLoose: true} {
		var exists bool
		_ = e.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM media WHERE id = $1)", id).Scan(&exists)
		if exists != want || e.storage.has(keys[id]) != want {
			t.Errorf("media %s: row %v, object %v; want both %v", id, exists, e.storage.has(keys[id]), want)
		}
	}
}

// The funnel the P1 gate is measured by: a sign-up is counted once, not each
// time a token is minted, and nothing a person wrote reaches analytics.
func TestProductEventsCarryNoPersonalData(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alice := e.user(t, "alice@example.com")
	if _, _, err := e.svc.CreateUserWithToken(ctx, "alice@example.com", "second"); err != nil {
		t.Fatal(err)
	}
	acct := e.account(t, alice)
	if _, err := e.svc.CreatePost(ctx, alice, service.CreatePostInput{Text: "secret words", AccountIDs: []uuid.UUID{acct}}); err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(e.events.names(), ","); got != "signed_up,account_connected" {
		t.Errorf("events %s, want signed_up,account_connected", got)
	}
	for _, ev := range e.events.events {
		if ev.DistinctID != alice.String() {
			t.Errorf("%s: distinct id %q, want alice's user id", ev.Name, ev.DistinctID)
		}
		for k, v := range ev.Props {
			if s, ok := v.(string); ok && (strings.Contains(s, "alice") || strings.Contains(s, "secret") || strings.Contains(s, "webhook")) {
				t.Errorf("%s carries %s=%q", ev.Name, k, s)
			}
		}
	}
}

func TestAssistedHandoff(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)

	view, err := e.svc.CreatePost(ctx, alice, service.CreatePostInput{Text: "for the group", Assisted: []string{"whatsapp", "whatsapp"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Targets) != 1 || view.Targets[0].Status != "handing_off" || view.Targets[0].AccountID.Valid {
		t.Fatalf("targets %+v, want one handing_off target with no account", view.Targets)
	}
	var queued int
	_ = e.pool.QueryRow(ctx, "SELECT count(*) FROM river_job").Scan(&queued)
	if queued != 0 {
		t.Errorf("%d publish jobs for a handoff, want none", queued)
	}
	target := view.Targets[0].ID

	done, err := e.svc.CompleteHandoff(ctx, alice, target, true)
	if err != nil || done.Targets[0].Status != "handed_off" || !done.Targets[0].PublishedAt.Valid {
		t.Fatalf("complete: %v, %+v; want handed_off with a time", err, done.Targets)
	}
	if _, err := e.svc.CompleteHandoff(ctx, alice, target, true); err != nil {
		t.Errorf("repeating the same outcome: %v, want it accepted", err)
	}
	if _, err := e.svc.CompleteHandoff(ctx, alice, target, false); !errors.Is(err, service.ErrConflict) {
		t.Errorf("changing the outcome: %v, want ErrConflict", err)
	}

	t.Run("cancelled share sheet", func(t *testing.T) {
		v, _ := e.svc.CreatePost(ctx, alice, service.CreatePostInput{Text: "x", AccountIDs: []uuid.UUID{acct}, Assisted: []string{"whatsapp"}})
		var handoff, direct uuid.UUID
		for _, tg := range v.Targets {
			if tg.Platform == "whatsapp" {
				handoff = tg.ID
			} else {
				direct = tg.ID
			}
		}
		got, err := e.svc.CompleteHandoff(ctx, alice, handoff, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, tg := range got.Targets {
			if tg.ID == handoff && (tg.Status != "failed" || deref(tg.LastError) == "") {
				t.Errorf("cancelled handoff: %+v, want failed with a reason", tg)
			}
		}
		if _, err := e.svc.CompleteHandoff(ctx, alice, direct, true); !errors.Is(err, service.ErrConflict) {
			t.Errorf("completing a direct target: %v, want ErrConflict", err)
		}
	})

	t.Run("another user's target", func(t *testing.T) {
		bob := e.user(t, "bob@example.com")
		if _, err := e.svc.CompleteHandoff(ctx, bob, target, true); !errors.Is(err, service.ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("phone never reports", func(t *testing.T) {
		stale, _ := e.svc.CreatePost(ctx, alice, service.CreatePostInput{Text: "stale", Assisted: []string{"whatsapp"}})
		fresh, _ := e.svc.CreatePost(ctx, alice, service.CreatePostInput{Text: "fresh", Assisted: []string{"whatsapp"}})
		if _, err := e.pool.Exec(ctx, "UPDATE post_targets SET updated_at = now() - interval '61 minutes' WHERE id = $1",
			stale.Targets[0].ID); err != nil {
			t.Fatal(err)
		}
		expire := jobs.NewExpireHandoffsWorker(db.New(e.pool), slog.New(slog.DiscardHandler))
		if err := expire.Work(ctx, &river.Job[jobs.ExpireHandoffsArgs]{JobRow: &rivertype.JobRow{}}); err != nil {
			t.Fatal(err)
		}
		got, _ := e.svc.GetPost(ctx, alice, stale.Post.ID)
		if tg := got.Targets[0]; tg.Status != "failed" || deref(tg.LastError) != jobs.HandoffExpiredMessage {
			t.Errorf("stale handoff: %+v, want failed as never reported", tg)
		}
		got, _ = e.svc.GetPost(ctx, alice, fresh.Post.ID)
		if tg := got.Targets[0]; tg.Status != "handing_off" {
			t.Errorf("fresh handoff: %+v, want still handing_off", tg)
		}
		// Only handing_off expires: a finished handoff keeps its outcome
		// however old it is.
		if _, err := e.pool.Exec(ctx, "UPDATE post_targets SET updated_at = now() - interval '1 day' WHERE id = $1", target); err != nil {
			t.Fatal(err)
		}
		if err := expire.Work(ctx, &river.Job[jobs.ExpireHandoffsArgs]{JobRow: &rivertype.JobRow{}}); err != nil {
			t.Fatal(err)
		}
		got, _ = e.svc.GetPost(ctx, alice, view.Post.ID)
		if got.Targets[0].Status != "handed_off" {
			t.Errorf("finished handoff became %s", got.Targets[0].Status)
		}
		if _, err := e.svc.CompleteHandoff(ctx, alice, stale.Targets[0].ID, true); !errors.Is(err, service.ErrConflict) {
			t.Errorf("late report: %v, want ErrConflict", err)
		}
	})

	t.Run("rejected requests", func(t *testing.T) {
		later := time.Now().Add(time.Hour)
		for name, in := range map[string]service.CreatePostInput{
			"not assisted": {Text: "x", Assisted: []string{"discord"}},
			"scheduled":    {Text: "x", Assisted: []string{"whatsapp"}, ScheduledAt: &later},
		} {
			var ve *service.ValidationError
			if _, err := e.svc.CreatePost(ctx, alice, in); !errors.As(err, &ve) || ve.Issues[0].Field != "assisted" {
				t.Errorf("%s: got %v, want an issue on assisted", name, err)
			}
		}
	})
}

func TestUsersAreIsolated(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alice, bob := e.user(t, "alice@example.com"), e.user(t, "bob@example.com")
	aliceAcct := e.account(t, alice)
	aliceImg := e.upload(t, alice, "image/png", []byte("x"))

	view, err := e.svc.CreatePost(ctx, alice, service.CreatePostInput{Text: "mine", AccountIDs: []uuid.UUID{aliceAcct}})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.GetPost(ctx, bob, view.Post.ID); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("bob read alice's post: %v", err)
	}
	if posts, _ := e.svc.ListPosts(ctx, bob, 20); len(posts) != 0 {
		t.Errorf("bob lists %d posts", len(posts))
	}
	if accts, _ := e.svc.ListAccounts(ctx, bob); len(accts) != 0 {
		t.Errorf("bob lists %d accounts", len(accts))
	}
	if err := e.svc.DeleteAccount(ctx, bob, aliceAcct); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("bob deleted alice's account: %v", err)
	}

	bobAcct := e.account(t, bob)
	_, err = e.svc.CreatePost(ctx, bob, service.CreatePostInput{
		Text: "borrowed", MediaIDs: []uuid.UUID{aliceImg}, AccountIDs: []uuid.UUID{aliceAcct, bobAcct},
	})
	var ve *service.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("bob posted with alice's account and file: %v", err)
	}
}

// Deleting a user takes everything of theirs with it, including posts that
// reference their uploads. This failed once on a RESTRICT foreign key.
func TestDeletingAUserRemovesEverything(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)
	img := e.upload(t, alice, "image/png", []byte("PNG"))
	if _, err := e.svc.CreatePost(ctx, alice, service.CreatePostInput{
		Text: "bye", MediaIDs: []uuid.UUID{img}, AccountIDs: []uuid.UUID{acct},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := e.pool.Exec(ctx, "DELETE FROM users WHERE id = $1", alice); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var left int
	_ = e.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM posts) + (SELECT count(*) FROM media)
		+ (SELECT count(*) FROM accounts) + (SELECT count(*) FROM post_targets)`).Scan(&left)
	if left != 0 {
		t.Fatalf("%d rows survived their user", left)
	}
}

func TestCredentialsAreSealedAtRest(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)

	var raw []byte
	if err := e.pool.QueryRow(ctx, "SELECT credentials FROM accounts WHERE id = $1", acct).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("secret-token")) {
		t.Fatal("the webhook token is stored in plaintext")
	}
}

func TestConnectDiscordRejectsForeignURLs(t *testing.T) {
	e := setup(t)
	alice := e.user(t, "alice@example.com")
	_, err := e.svc.ConnectDiscord(context.Background(), alice, "https://internal.example/api/webhooks/1/x", nil)
	var br *service.BadRequest
	if !errors.As(err, &br) {
		t.Fatalf("got %v, want BadRequest", err)
	}
}

func TestWorkerOutcomes(t *testing.T) {
	type outcome struct {
		status    string
		cancelled bool
		errText   string
		event     string // what analytics records; empty while it will retry
	}
	cases := map[string]struct {
		prepare func(e *env, targetID, accountID uuid.UUID)
		attempt int
		want    outcome
	}{
		"published": {
			prepare: func(*env, uuid.UUID, uuid.UUID) {},
			attempt: 1,
			want:    outcome{status: "published", event: "target_published"},
		},
		"platform rejects: no retry": {
			prepare: func(e *env, _, _ uuid.UUID) { e.discord.postStatus.Store(http.StatusBadRequest) },
			attempt: 1,
			want:    outcome{status: "failed", cancelled: true, errText: "stub failure", event: "target_failed:rejected"},
		},
		"platform down: retry": {
			prepare: func(e *env, _, _ uuid.UUID) { e.discord.postStatus.Store(http.StatusBadGateway) },
			attempt: 1,
			want:    outcome{status: "pending", errText: "502"},
		},
		"platform down on the last attempt: give up": {
			prepare: func(e *env, _, _ uuid.UUID) { e.discord.postStatus.Store(http.StatusBadGateway) },
			attempt: 10,
			want:    outcome{status: "failed", errText: "502", event: "target_failed:retries_exhausted"},
		},
		"previous attempt died mid-request: do not risk a duplicate": {
			prepare: func(e *env, targetID, _ uuid.UUID) {
				_, _ = e.pool.Exec(context.Background(), "UPDATE post_targets SET status = 'publishing' WHERE id = $1", targetID)
			},
			attempt: 2,
			want:    outcome{status: "failed", cancelled: true, errText: "interrupted", event: "target_failed:interrupted"},
		},
		"account disconnected after posting": {
			prepare: func(e *env, _, accountID uuid.UUID) {
				_, _ = e.pool.Exec(context.Background(), "DELETE FROM accounts WHERE id = $1", accountID)
			},
			attempt: 1,
			want:    outcome{status: "failed", cancelled: true, errText: "disconnected", event: "target_failed:disconnected"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := setup(t)
			alice := e.user(t, "alice@example.com")
			acct := e.account(t, alice)
			view, err := e.svc.CreatePost(context.Background(), alice, service.CreatePostInput{Text: "hi", AccountIDs: []uuid.UUID{acct}})
			if err != nil {
				t.Fatal(err)
			}
			targetID := view.Targets[0].ID
			tc.prepare(e, targetID, acct)
			e.events.reset()

			jobErr := e.runJob(t, targetID, tc.attempt)

			var cancel *river.JobCancelError
			if errors.As(jobErr, &cancel) != tc.want.cancelled {
				t.Errorf("job error %v, cancelled want %v", jobErr, tc.want.cancelled)
			}
			tgt := e.target(t, alice, view.Post.ID)
			if tgt.Status != tc.want.status {
				t.Errorf("status %q, want %q", tgt.Status, tc.want.status)
			}
			if !strings.Contains(deref(tgt.LastError), tc.want.errText) {
				t.Errorf("last error %q, want it to mention %q", deref(tgt.LastError), tc.want.errText)
			}
			want := []string{}
			if tc.want.event != "" {
				want = append(want, tc.want.event)
			}
			if got := e.events.names(); strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("events %v, want %v", got, want)
			}
			for _, ev := range e.events.events {
				if ev.DistinctID != alice.String() || ev.Props["platform"] != "discord" {
					t.Errorf("event %+v: want alice's id and the platform", ev)
				}
			}
		})
	}
}

func TestWorkerDoesNotRepublish(t *testing.T) {
	e := setup(t)
	alice := e.user(t, "alice@example.com")
	acct := e.account(t, alice)
	view, err := e.svc.CreatePost(context.Background(), alice, service.CreatePostInput{Text: "once", AccountIDs: []uuid.UUID{acct}})
	if err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err := e.runJob(t, view.Targets[0].ID, 1); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.discord.posts.Load(); n != 1 {
		t.Fatalf("discord received %d posts, want 1", n)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
