package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/publisher"
	"github.com/FACorreiaa/postitheka/internal/service"
)

const refreshPlatform publisher.Platform = "refreshstub"

type refreshCreds struct {
	Token   string `json:"token"`
	Expired bool   `json:"expired"`
}

// refreshStub stands for a platform whose refresh token rotates: spending
// the same one twice would fail on the real platform, so the test counts.
type refreshStub struct {
	refreshes atomic.Int32
	fail      error
	mu        sync.Mutex
	usedToken []string
}

func (r *refreshStub) Platform() publisher.Platform { return refreshPlatform }
func (r *refreshStub) Rules() publisher.Rules {
	return publisher.Rules{MaxTextLength: 100, RequiresText: true}
}

func (r *refreshStub) NeedsRefresh(credentials []byte, _ time.Time) bool {
	var c refreshCreds
	return json.Unmarshal(credentials, &c) != nil || c.Expired
}

func (r *refreshStub) Refresh(_ context.Context, credentials []byte) ([]byte, error) {
	if r.fail != nil {
		return nil, r.fail
	}
	n := r.refreshes.Add(1)
	// Wide enough for a second job to read the old credentials if the lock
	// were missing.
	time.Sleep(50 * time.Millisecond)
	return json.Marshal(refreshCreds{Token: "token-" + string(rune('0'+n))})
}

func (r *refreshStub) Publish(_ context.Context, credentials []byte, _ publisher.Post, _ publisher.MediaSource) (publisher.Result, error) {
	var c refreshCreds
	if err := json.Unmarshal(credentials, &c); err != nil {
		return publisher.Result{}, publisher.Permanent(err)
	}
	if c.Expired {
		return publisher.Result{}, publisher.Permanent(errors.New("published with expired credentials"))
	}
	r.mu.Lock()
	r.usedToken = append(r.usedToken, c.Token)
	r.mu.Unlock()
	return publisher.Result{RemoteID: "remote"}, nil
}

// expiredAccount stores an account whose credentials need refreshing, as an
// OAuth connect would have left it once the token aged.
func expiredAccount(t *testing.T, e *env, userID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	creds, _ := json.Marshal(refreshCreds{Token: "token-0", Expired: true})
	if _, err := e.q.CreateAccount(context.Background(), db.CreateAccountParams{
		ID: id, UserID: userID, Platform: string(refreshPlatform), DisplayName: "Ada",
		Credentials: e.box.Seal(creds, id[:]),
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestTokenRefresh(t *testing.T) {
	ctx := context.Background()

	t.Run("concurrent posts refresh once and publish with the new token", func(t *testing.T) {
		stub := &refreshStub{}
		e := setup(t, stub)
		alice := e.user(t, "alice@example.com")
		acct := expiredAccount(t, e, alice)

		var posts []uuid.UUID
		for range 3 {
			view, err := e.svc.CreatePost(ctx, alice, service.CreatePostInput{Text: "hello", AccountIDs: []uuid.UUID{acct}})
			if err != nil {
				t.Fatal(err)
			}
			posts = append(posts, view.Post.ID)
		}

		var wg sync.WaitGroup
		for _, p := range posts {
			tgt := e.target(t, alice, p)
			wg.Go(func() {
				if err := e.runJob(t, tgt.ID, 1); err != nil {
					t.Errorf("job: %v", err)
				}
			})
		}
		wg.Wait()

		if n := stub.refreshes.Load(); n != 1 {
			t.Errorf("%d refreshes, want 1", n)
		}
		for _, p := range posts {
			if got := e.target(t, alice, p).Status; got != "published" {
				t.Errorf("target %s, want published", got)
			}
		}
		for _, tok := range stub.usedToken {
			if tok != "token-1" {
				t.Errorf("published with %q, want the refreshed token-1", tok)
			}
		}

		var sealed []byte
		if err := e.pool.QueryRow(ctx, "SELECT credentials FROM accounts WHERE id = $1", acct).Scan(&sealed); err != nil {
			t.Fatal(err)
		}
		plain, err := e.box.Open(sealed, acct[:])
		if err != nil {
			t.Fatal(err)
		}
		var stored refreshCreds
		_ = json.Unmarshal(plain, &stored)
		if stored.Token != "token-1" || stored.Expired {
			t.Errorf("stored %+v, want the refreshed credentials", stored)
		}
	})

	t.Run("a revoked grant fails the target and asks for a reconnect", func(t *testing.T) {
		stub := &refreshStub{fail: publisher.Permanent(errors.New("refresh token revoked"))}
		e := setup(t, stub)
		alice := e.user(t, "alice@example.com")
		acct := expiredAccount(t, e, alice)
		view, err := e.svc.CreatePost(ctx, alice, service.CreatePostInput{Text: "hello", AccountIDs: []uuid.UUID{acct}})
		if err != nil {
			t.Fatal(err)
		}
		tgt := e.target(t, alice, view.Post.ID)

		_ = e.runJob(t, tgt.ID, 1)
		got := e.target(t, alice, view.Post.ID)
		if got.Status != "failed" || got.LastError == nil || *got.LastError != "refresh token revoked; reconnect the account" {
			t.Errorf("target %s %v, want failed asking for a reconnect", got.Status, got.LastError)
		}
		if len(stub.usedToken) != 0 {
			t.Error("published despite the failed refresh")
		}
	})

	t.Run("a refresh that fails for now leaves the target pending", func(t *testing.T) {
		stub := &refreshStub{fail: errors.New("connection reset")}
		e := setup(t, stub)
		alice := e.user(t, "alice@example.com")
		acct := expiredAccount(t, e, alice)
		view, err := e.svc.CreatePost(ctx, alice, service.CreatePostInput{Text: "hello", AccountIDs: []uuid.UUID{acct}})
		if err != nil {
			t.Fatal(err)
		}
		tgt := e.target(t, alice, view.Post.ID)

		if err := e.runJob(t, tgt.ID, 1); err == nil {
			t.Fatal("job succeeded; want an error so River retries")
		}
		if got := e.target(t, alice, view.Post.ID); got.Status != "pending" {
			t.Errorf("target %s, want pending for the retry", got.Status)
		}
	})
}
