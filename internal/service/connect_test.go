package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/FACorreiaa/postitheka/internal/httpapi"
	"github.com/FACorreiaa/postitheka/internal/publisher"
	"github.com/FACorreiaa/postitheka/internal/service"
)

const stubPlatform publisher.Platform = "oauthstub"

// oauthStub plays a platform's consent page and token endpoint. It checks
// what a real platform would: that the redirect URL matches the one the flow
// started with, and that the verifier hashes to the challenge it was shown.
type oauthStub struct {
	mu         sync.Mutex
	challenges map[string]string // state -> code_challenge
	redirect   string
	remoteID   string
	name       string
	failNext   bool
}

func newOAuthStub() *oauthStub {
	return &oauthStub{challenges: map[string]string{}, remoteID: "member-1", name: "Ada"}
}

func (o *oauthStub) Platform() publisher.Platform { return stubPlatform }
func (o *oauthStub) Rules() publisher.Rules {
	return publisher.Rules{MaxTextLength: 100, RequiresText: true}
}

func (o *oauthStub) Publish(context.Context, []byte, publisher.Post, publisher.MediaSource) (publisher.Result, error) {
	return publisher.Result{}, errors.New("not used")
}

func (o *oauthStub) AuthorizeURL(state, verifier, redirectURL string) string {
	sum := sha256.Sum256([]byte(verifier))
	o.mu.Lock()
	o.challenges[state] = base64.RawURLEncoding.EncodeToString(sum[:])
	o.redirect = redirectURL
	o.mu.Unlock()
	return "https://consent.example/authorize?" + url.Values{"state": {state}, "redirect_uri": {redirectURL}}.Encode()
}

func (o *oauthStub) Exchange(_ context.Context, code, verifier, redirectURL string) (publisher.Connection, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.failNext {
		o.failNext = false
		return publisher.Connection{}, errors.New("invalid_grant")
	}
	if redirectURL != o.redirect {
		return publisher.Connection{}, errors.New("redirect_uri mismatch")
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	for _, c := range o.challenges {
		if c == challenge {
			return publisher.Connection{DisplayName: o.name, Credentials: []byte("token-for-" + code), RemoteID: o.remoteID}, nil
		}
	}
	return publisher.Connection{}, errors.New("verifier does not match any challenge")
}

// consent starts a connect and returns the state the platform was given.
func consent(t *testing.T, e *env, user [16]byte, client service.ConnectClient) string {
	t.Helper()
	authURL, err := e.svc.StartConnect(context.Background(), user, stubPlatform, client)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	if got := u.Query().Get("redirect_uri"); got != "https://postitheka.test/oauth/oauthstub/callback" {
		t.Fatalf("redirect_uri %q", got)
	}
	return u.Query().Get("state")
}

// callback is the platform redirecting the browser back, answered by the
// real handler.
func callback(t *testing.T, e *env, query url.Values) string {
	t.Helper()
	r := chi.NewRouter()
	r.Get("/oauth/{platform}/callback", httpapi.OAuthCallback(e.svc, slog.New(slog.DiscardHandler)))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/oauth/oauthstub/callback?"+query.Encode(), nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("callback answered %d, want a redirect", rec.Code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("callback response may be cached")
	}
	return rec.Header().Get("Location")
}

func TestOAuthConnect(t *testing.T) {
	stub := newOAuthStub()
	e := setup(t, stub)
	ctx := context.Background()
	alice := e.user(t, "alice@example.com")

	state := consent(t, e, alice, service.ConnectFromWeb)
	if got := callback(t, e, url.Values{"state": {state}, "code": {"c1"}}); got != "/?connected=oauthstub" {
		t.Fatalf("redirect %q", got)
	}
	accounts, _ := e.svc.ListAccounts(ctx, alice)
	var connected []service.Account
	for _, a := range accounts {
		if a.Platform == string(stubPlatform) {
			connected = append(connected, a)
		}
	}
	if len(connected) != 1 || connected[0].DisplayName != "Ada" {
		t.Fatalf("accounts %+v, want one named Ada", connected)
	}
	first := connected[0].ID

	t.Run("a state works once", func(t *testing.T) {
		if got := callback(t, e, url.Values{"state": {state}, "code": {"c1"}}); got != "/?connect_error=expired" {
			t.Errorf("replayed state: %q", got)
		}
	})

	t.Run("reconnecting renews the same account", func(t *testing.T) {
		stub.name = "Ada L."
		s := consent(t, e, alice, service.ConnectFromWeb)
		callback(t, e, url.Values{"state": {s}, "code": {"c2"}})
		accounts, _ := e.svc.ListAccounts(ctx, alice)
		n := 0
		for _, a := range accounts {
			if a.Platform == string(stubPlatform) {
				n++
				if a.ID != first || a.DisplayName != "Ada L." {
					t.Errorf("reconnected account %+v, want id %s renamed", a, first)
				}
			}
		}
		if n != 1 {
			t.Errorf("%d accounts after reconnecting, want 1", n)
		}
		var sealed []byte
		_ = e.pool.QueryRow(ctx, "SELECT credentials FROM accounts WHERE id = $1", first).Scan(&sealed)
		if len(sealed) == 0 || string(sealed) == "token-for-c2" {
			t.Error("credentials are not stored sealed")
		}
	})

	t.Run("declined on the platform, from iOS", func(t *testing.T) {
		s := consent(t, e, alice, service.ConnectFromIOS)
		got := callback(t, e, url.Values{"state": {s}, "error": {"access_denied"}})
		if got != "postitheka://connected?error=cancelled" {
			t.Errorf("redirect %q", got)
		}
	})

	t.Run("iOS success goes back to the app", func(t *testing.T) {
		s := consent(t, e, alice, service.ConnectFromIOS)
		if got := callback(t, e, url.Values{"state": {s}, "code": {"c3"}}); got != "postitheka://connected?platform=oauthstub" {
			t.Errorf("redirect %q", got)
		}
	})

	t.Run("a consent page left open too long", func(t *testing.T) {
		s := consent(t, e, alice, service.ConnectFromWeb)
		if _, err := e.pool.Exec(ctx, "UPDATE oauth_states SET created_at = now() - interval '16 minutes'"); err != nil {
			t.Fatal(err)
		}
		if got := callback(t, e, url.Values{"state": {s}, "code": {"c4"}}); got != "/?connect_error=expired" {
			t.Errorf("redirect %q", got)
		}
	})

	t.Run("the platform refuses the code", func(t *testing.T) {
		s := consent(t, e, alice, service.ConnectFromWeb)
		stub.failNext = true
		if got := callback(t, e, url.Values{"state": {s}, "code": {"c5"}}); got != "/?connect_error=failed" {
			t.Errorf("redirect %q", got)
		}
	})

	t.Run("an unknown state", func(t *testing.T) {
		if got := callback(t, e, url.Values{"state": {"made-up"}, "code": {"c6"}}); got != "/?connect_error=expired" {
			t.Errorf("redirect %q", got)
		}
	})

	t.Run("the same platform user for two of our users", func(t *testing.T) {
		bob := e.user(t, "bob@example.com")
		s := consent(t, e, bob, service.ConnectFromWeb)
		callback(t, e, url.Values{"state": {s}, "code": {"c7"}})
		bobs, _ := e.svc.ListAccounts(ctx, bob)
		if len(bobs) != 1 || bobs[0].ID == first {
			t.Errorf("bob's accounts %+v, want his own", bobs)
		}
	})

	t.Run("abandoned states are cleared", func(t *testing.T) {
		consent(t, e, alice, service.ConnectFromWeb)
		if _, err := e.pool.Exec(ctx, "UPDATE oauth_states SET created_at = now() - interval '1 hour'"); err != nil {
			t.Fatal(err)
		}
		consent(t, e, alice, service.ConnectFromWeb)
		var n int
		_ = e.pool.QueryRow(ctx, "SELECT count(*) FROM oauth_states WHERE user_id = $1", alice).Scan(&n)
		if n != 1 {
			t.Errorf("%d states, want only the new one", n)
		}
	})

	t.Run("platforms that do not use OAuth", func(t *testing.T) {
		if _, err := e.svc.StartConnect(ctx, alice, publisher.Discord, service.ConnectFromWeb); !errors.Is(err, service.ErrNotFound) {
			t.Errorf("discord: %v, want ErrNotFound", err)
		}
		if !e.svc.ConnectsByOAuth(stubPlatform) || e.svc.ConnectsByOAuth(publisher.Discord) {
			t.Error("ConnectsByOAuth disagrees with the registry")
		}
	})
}
