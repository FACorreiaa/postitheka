package threads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FACorreiaa/postitheka/internal/publisher"
)

const userID = "17841405793187218" // past 2^53: mangled if read as a float

// stub plays the Threads Graph API: the token endpoints, the profile,
// containers, their status and publishing. Every host is redirected to it.
type stub struct {
	mu       sync.Mutex
	requests []recorded
	nextID   int
	// inProgress is how many status checks a container answers IN_PROGRESS
	// before final (FINISHED when empty).
	inProgress  int
	final       string
	finalError  string
	published   map[string]bool
	polledFirst []string // containers in the order their status was first asked

	createStatus int // non-zero: container creation answers this with createBody
	createBody   string
	// publishMode: "" succeeds; "500-live" fails but the post went out;
	// "500" fails and nothing went out; "400" refuses.
	publishMode  string
	statusBroken bool // status checks fail once a publish has been tried
	refreshBody  string
	refreshCode  int
}

type recorded struct {
	Method, Host, Path, Query string
	Body                      []byte
}

func (s *stub) find(method, path string) []recorded {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []recorded
	for _, r := range s.requests {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

func (s *stub) graphError(w http.ResponseWriter, status, code int, msg string) {
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"message":%q,"type":"OAuthException","code":%d,"fbtrace_id":"x"}}`, msg, code)
}

func (s *stub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, recorded{r.Method, r.Host, r.URL.Path, r.URL.RawQuery, body})
	s.mu.Unlock()
	form, _ := url.ParseQuery(string(body))
	q := r.URL.Query()
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/oauth/access_token":
		_, _ = fmt.Fprintf(w, `{"access_token":"short-1","token_type":"bearer","user_id":%s}`, userID)
	case r.Method == http.MethodGet && r.URL.Path == "/access_token":
		if q.Get("access_token") != "short-1" || q.Get("client_secret") != "secret-1" || q.Get("grant_type") != "th_exchange_token" {
			s.graphError(w, http.StatusBadRequest, 100, "bad exchange")
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"long-1","token_type":"bearer","expires_in":5183944}`)
	case r.Method == http.MethodGet && r.URL.Path == "/refresh_access_token":
		if s.refreshCode != 0 {
			w.WriteHeader(s.refreshCode)
			_, _ = io.WriteString(w, s.refreshBody)
			return
		}
		if q.Get("grant_type") != "th_refresh_token" || q.Get("access_token") != "long-1" {
			s.graphError(w, http.StatusBadRequest, 100, "bad refresh")
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"long-2","token_type":"bearer","expires_in":5183944}`)
	case r.Method == http.MethodGet && r.URL.Path == "/v1.0/me":
		if q.Get("access_token") != "long-1" {
			s.graphError(w, http.StatusBadRequest, 190, "Invalid OAuth access token")
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":%q,"username":"ada","name":"Ada Lovelace"}`, userID)
	case r.Method == http.MethodPost && r.URL.Path == "/v1.0/"+userID+"/threads":
		if s.createStatus != 0 {
			w.WriteHeader(s.createStatus)
			_, _ = io.WriteString(w, s.createBody)
			return
		}
		s.mu.Lock()
		s.nextID++
		id := fmt.Sprintf("c-%d", s.nextID)
		s.mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"id":%q}`, id)
	case r.Method == http.MethodPost && r.URL.Path == "/v1.0/"+userID+"/threads_publish":
		id := form.Get("creation_id")
		switch s.publishMode {
		case "500-live":
			s.mu.Lock()
			s.published[id] = true
			s.mu.Unlock()
			s.graphError(w, http.StatusInternalServerError, 2, "Service temporarily unavailable")
		case "500":
			s.graphError(w, http.StatusInternalServerError, 2, "Service temporarily unavailable")
		case "400":
			s.graphError(w, http.StatusBadRequest, 100, "The parameter creation_id is invalid")
		default:
			s.mu.Lock()
			s.published[id] = true
			s.mu.Unlock()
			_, _ = io.WriteString(w, `{"id":"m-1"}`)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/v1.0/m-1":
		_, _ = io.WriteString(w, `{"permalink":"https://www.threads.com/@ada/post/ABC","id":"m-1"}`)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1.0/c-"):
		id := strings.TrimPrefix(r.URL.Path, "/v1.0/")
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.statusBroken && len(s.findLocked(http.MethodPost, "/v1.0/"+userID+"/threads_publish")) > 0 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		seen := false
		for _, c := range s.polledFirst {
			seen = seen || c == id
		}
		if !seen {
			s.polledFirst = append(s.polledFirst, id)
		}
		status := s.final
		switch {
		case s.published[id]:
			status = "PUBLISHED"
		case s.inProgress > 0:
			s.inProgress--
			status = "IN_PROGRESS"
		case status == "":
			status = "FINISHED"
		}
		_, _ = fmt.Fprintf(w, `{"id":%q,"status":%q,"error_message":%q}`, id, status, s.finalError)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *stub) findLocked(method, path string) []recorded {
	var out []recorded
	for _, r := range s.requests {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

var now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func newTest(t *testing.T) (*Publisher, *stub) {
	t.Helper()
	s := &stub{published: map[string]bool{}}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.Host = r.URL.Host
		r.URL.Scheme, r.URL.Host = target.Scheme, target.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
	p := New("app-1", "secret-1", client)
	p.pollEvery = time.Millisecond
	p.processingWait = 200 * time.Millisecond
	p.now = func() time.Time { return now }
	return p, s
}

// files hands out URLs on a storage host, as presigned GETs would be.
type files struct{}

func (files) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("threads never reads media itself")
}

func (files) PublicURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.test/" + key + "?signed", nil
}

func creds(issued, expires time.Time) []byte {
	raw, _ := json.Marshal(credentials{AccessToken: "long-1", IssuedAt: issued, ExpiresAt: expires, UserID: userID})
	return raw
}

var fresh = creds(now.Add(-time.Hour), now.Add(59*24*time.Hour))

func form(t *testing.T, r recorded) url.Values {
	t.Helper()
	v, err := url.ParseQuery(string(r.Body))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestConnect(t *testing.T) {
	p, s := newTest(t)
	const redirect = "https://postitheka.test/oauth/threads/callback"

	auth, _ := url.Parse(p.AuthorizeURL("st-1", "verifier-1", redirect))
	q := auth.Query()
	if auth.Host != "threads.com" || auth.Path != "/oauth/authorize" {
		t.Errorf("authorize URL %s", auth)
	}
	for k, want := range map[string]string{"client_id": "app-1", "redirect_uri": redirect, "state": "st-1", "scope": "threads_basic,threads_content_publish", "response_type": "code"} {
		if q.Get(k) != want {
			t.Errorf("authorize %s = %q, want %q", k, q.Get(k), want)
		}
	}
	if q.Has("code_challenge") {
		t.Error("sent a PKCE challenge Threads has no use for")
	}

	conn, err := p.Exchange(context.Background(), "code-1", "verifier-1", redirect)
	if err != nil {
		t.Fatal(err)
	}
	tok := s.find(http.MethodPost, "/oauth/access_token")
	if len(tok) != 1 {
		t.Fatalf("%d token requests", len(tok))
	}
	if tok[0].Host != "graph.threads.com" {
		t.Errorf("code exchanged at %s", tok[0].Host)
	}
	f := form(t, tok[0])
	for k, want := range map[string]string{"grant_type": "authorization_code", "code": "code-1", "redirect_uri": redirect, "client_id": "app-1", "client_secret": "secret-1"} {
		if f.Get(k) != want {
			t.Errorf("token %s = %q, want %q", k, f.Get(k), want)
		}
	}
	if tok[0].Query != "" {
		t.Errorf("code exchange put %q in the URL", tok[0].Query)
	}

	if conn.DisplayName != "@ada" || conn.RemoteID != userID {
		t.Errorf("connection %q %q", conn.DisplayName, conn.RemoteID)
	}
	var c credentials
	if err := json.Unmarshal(conn.Credentials, &c); err != nil {
		t.Fatal(err)
	}
	if c.AccessToken != "long-1" || c.UserID != userID || !c.IssuedAt.Equal(now) || !c.ExpiresAt.Equal(now.Add(5183944*time.Second)) {
		t.Errorf("stored %+v; want the long-lived token, exact user id, issued now", c)
	}
}

func TestPublishText(t *testing.T) {
	p, s := newTest(t)
	res, err := p.Publish(context.Background(), fresh, publisher.Post{Text: "hello 👋"}, files{})
	if err != nil {
		t.Fatal(err)
	}
	if res.RemoteID != "m-1" || res.URL != "https://www.threads.com/@ada/post/ABC" {
		t.Errorf("result %+v", res)
	}
	created := s.find(http.MethodPost, "/v1.0/"+userID+"/threads")
	if len(created) != 1 {
		t.Fatalf("%d containers", len(created))
	}
	f := form(t, created[0])
	if f.Get("media_type") != "TEXT" || f.Get("text") != "hello 👋" || f.Get("access_token") != "long-1" {
		t.Errorf("container form %v", f)
	}
	pub := s.find(http.MethodPost, "/v1.0/"+userID+"/threads_publish")
	if len(pub) != 1 || form(t, pub[0]).Get("creation_id") != "c-1" {
		t.Fatalf("publish requests %+v", pub)
	}
	for _, r := range s.requests {
		if r.Method == http.MethodPost && strings.Contains(r.Query, "access_token") {
			t.Errorf("POST %s carried the token in its URL", r.Path)
		}
		if r.Host != "graph.threads.com" {
			t.Errorf("request to %s", r.Host)
		}
	}
}

func TestPublishMedia(t *testing.T) {
	t.Run("one image", func(t *testing.T) {
		p, s := newTest(t)
		post := publisher.Post{Text: "look", Media: []publisher.Media{{Key: "a.jpg", ContentType: "image/jpeg"}}}
		if _, err := p.Publish(context.Background(), fresh, post, files{}); err != nil {
			t.Fatal(err)
		}
		f := form(t, s.find(http.MethodPost, "/v1.0/"+userID+"/threads")[0])
		if f.Get("media_type") != "IMAGE" || f.Get("image_url") != "https://storage.test/a.jpg?signed" || f.Get("text") != "look" {
			t.Errorf("container form %v", f)
		}
	})

	t.Run("one video, processed before publishing", func(t *testing.T) {
		p, s := newTest(t)
		s.inProgress = 3
		post := publisher.Post{Media: []publisher.Media{{Key: "v.mov", ContentType: "video/quicktime"}}}
		if _, err := p.Publish(context.Background(), fresh, post, files{}); err != nil {
			t.Fatal(err)
		}
		f := form(t, s.find(http.MethodPost, "/v1.0/"+userID+"/threads")[0])
		if f.Get("media_type") != "VIDEO" || f.Get("video_url") != "https://storage.test/v.mov?signed" || f.Has("text") {
			t.Errorf("container form %v", f)
		}
		if n := len(s.find(http.MethodGet, "/v1.0/c-1")); n != 4 {
			t.Errorf("%d status checks, want 3 in progress and 1 finished", n)
		}
	})

	t.Run("carousel", func(t *testing.T) {
		p, s := newTest(t)
		post := publisher.Post{Text: "three", Media: []publisher.Media{
			{Key: "1.jpg", ContentType: "image/jpeg"},
			{Key: "2.mp4", ContentType: "video/mp4"},
			{Key: "3.png", ContentType: "image/png"},
		}}
		if _, err := p.Publish(context.Background(), fresh, post, files{}); err != nil {
			t.Fatal(err)
		}
		created := s.find(http.MethodPost, "/v1.0/"+userID+"/threads")
		if len(created) != 4 {
			t.Fatalf("%d containers, want 3 items and the carousel", len(created))
		}
		for i, want := range []string{"IMAGE", "VIDEO", "IMAGE"} {
			f := form(t, created[i])
			if f.Get("media_type") != want || f.Get("is_carousel_item") != "true" || f.Has("text") {
				t.Errorf("item %d form %v", i, f)
			}
		}
		c := form(t, created[3])
		if c.Get("media_type") != "CAROUSEL" || c.Get("children") != "c-1,c-2,c-3" || c.Get("text") != "three" {
			t.Errorf("carousel form %v", c)
		}
		if got := strings.Join(s.polledFirst, ","); got != "c-1,c-2,c-3,c-4" {
			t.Errorf("status checked in order %s; every item must finish before the carousel is made", got)
		}
		if form(t, s.find(http.MethodPost, "/v1.0/"+userID+"/threads_publish")[0]).Get("creation_id") != "c-4" {
			t.Error("published something other than the carousel")
		}
	})
}

func TestContainerLifecycle(t *testing.T) {
	video := publisher.Post{Media: []publisher.Media{{Key: "v.mp4", ContentType: "video/mp4"}}}

	t.Run("ERROR is permanent and says why", func(t *testing.T) {
		p, s := newTest(t)
		s.final, s.finalError = "ERROR", "INVALID_DURATION"
		_, err := p.Publish(context.Background(), fresh, video, files{})
		if !publisher.IsPermanent(err) || !strings.Contains(err.Error(), "INVALID_DURATION") {
			t.Errorf("got %v, want a permanent INVALID_DURATION", err)
		}
		if len(s.find(http.MethodPost, "/v1.0/"+userID+"/threads_publish")) != 0 {
			t.Error("published a failed container")
		}
	})

	t.Run("EXPIRED is retried", func(t *testing.T) {
		p, s := newTest(t)
		s.final = "EXPIRED"
		_, err := p.Publish(context.Background(), fresh, video, files{})
		if err == nil || publisher.IsPermanent(err) {
			t.Errorf("got %v, want a retryable error", err)
		}
	})

	t.Run("still processing at the deadline is retried, never published", func(t *testing.T) {
		p, s := newTest(t)
		s.inProgress = 1 << 30
		clock := now
		p.now = func() time.Time { clock = clock.Add(time.Minute); return clock }
		p.processingWait = 5 * time.Minute
		_, err := p.Publish(context.Background(), fresh, video, files{})
		if err == nil || publisher.IsPermanent(err) || !strings.Contains(err.Error(), "still processing") {
			t.Errorf("got %v, want a retryable timeout", err)
		}
		if len(s.find(http.MethodPost, "/v1.0/"+userID+"/threads_publish")) != 0 {
			t.Error("published an unfinished container")
		}
	})
}

// A publish that fails without a clear answer may still have gone out. The
// container says which, and only "not published" may be retried.
func TestPublishWithoutAClearAnswer(t *testing.T) {
	text := publisher.Post{Text: "hi"}

	t.Run("failed but live counts as published", func(t *testing.T) {
		p, s := newTest(t)
		s.publishMode = "500-live"
		if _, err := p.Publish(context.Background(), fresh, text, files{}); err != nil {
			t.Errorf("got %v; the post is live, so this is a success", err)
		}
	})

	t.Run("failed and not live is retried", func(t *testing.T) {
		p, s := newTest(t)
		s.publishMode = "500"
		_, err := p.Publish(context.Background(), fresh, text, files{})
		if err == nil || publisher.IsPermanent(err) {
			t.Errorf("got %v, want a retryable error", err)
		}
	})

	t.Run("failed and unknowable stops and says check Threads", func(t *testing.T) {
		p, s := newTest(t)
		s.publishMode = "500"
		s.statusBroken = true
		_, err := p.Publish(context.Background(), fresh, text, files{})
		if !publisher.IsPermanent(err) || !strings.Contains(err.Error(), "check Threads before posting again") {
			t.Errorf("got %v, want a permanent 'check Threads'", err)
		}
	})

	t.Run("a refusal is believed without checking", func(t *testing.T) {
		p, s := newTest(t)
		s.publishMode = "400"
		_, err := p.Publish(context.Background(), fresh, text, files{})
		if !publisher.IsPermanent(err) {
			t.Errorf("got %v, want permanent", err)
		}
		if n := len(s.find(http.MethodGet, "/v1.0/c-1")); n != 1 {
			t.Errorf("%d status checks, want only the one before publishing", n)
		}
	})

	t.Run("a cancelled request still gets its check", func(t *testing.T) {
		p, s := newTest(t)
		s.publishMode = "500-live"
		ctx, cancel := context.WithCancel(context.Background())
		inner := p.client.Transport
		p.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			resp, err := inner.RoundTrip(r)
			if strings.HasSuffix(r.URL.Path, "/threads_publish") {
				cancel() // the job's deadline passes while Threads answers
			}
			return resp, err
		})}
		if _, err := p.Publish(ctx, fresh, text, files{}); err != nil {
			t.Errorf("got %v; the check must run on its own context and find the post live", err)
		}
	})
}

func TestErrorClassification(t *testing.T) {
	for _, c := range []struct {
		name      string
		status    int
		body      string
		permanent bool
		wait      time.Duration
	}{
		{"expired token", 400, `{"error":{"message":"Error validating access token","code":190}}`, true, 0},
		{"rate limited", 400, `{"error":{"message":"Application request limit reached","code":4}}`, false, time.Hour},
		{"user rate limited", 400, `{"error":{"message":"User request limit reached","code":17}}`, false, time.Hour},
		{"too many requests", 429, ``, false, time.Hour},
		{"temporary", 500, `{"error":{"message":"An unknown error occurred","code":1}}`, false, 0},
		{"bad gateway", 502, `<html>`, false, 0},
		{"too many links", 400, `{"error":{"message":"THREADS_API__LINK_LIMIT_EXCEEDED","code":100}}`, true, 0},
		{"no permission", 403, `{"error":{"message":"Permissions error","code":10}}`, true, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, s := newTest(t)
			s.createStatus, s.createBody = c.status, c.body
			_, err := p.Publish(context.Background(), fresh, publisher.Post{Text: "hi"}, files{})
			if err == nil {
				t.Fatal("no error")
			}
			if publisher.IsPermanent(err) != c.permanent {
				t.Errorf("permanent = %v, want %v: %v", publisher.IsPermanent(err), c.permanent, err)
			}
			var ra *publisher.RetryAfterError
			if got := errors.As(err, &ra); got != (c.wait > 0) || (got && ra.After != c.wait) {
				t.Errorf("retry after: %v, want %s", err, c.wait)
			}
		})
	}
}

func TestPublishRefusesBadCredentials(t *testing.T) {
	p, s := newTest(t)
	for name, raw := range map[string][]byte{
		"unreadable": []byte("{"),
		"no user":    []byte(`{"access_token":"long-1"}`),
		"expired":    creds(now.Add(-61*24*time.Hour), now.Add(-time.Hour)),
	} {
		_, err := p.Publish(context.Background(), raw, publisher.Post{Text: "hi"}, files{})
		if !publisher.IsPermanent(err) {
			t.Errorf("%s: got %v, want permanent", name, err)
		}
	}
	if len(s.requests) != 0 {
		t.Errorf("%d requests sent with bad credentials", len(s.requests))
	}
}

func TestRefresh(t *testing.T) {
	ctx := context.Background()
	p, s := newTest(t)

	for _, c := range []struct {
		name string
		raw  []byte
		want bool
	}{
		{"an hour old", fresh, false},
		{"eight days old", creds(now.Add(-8*24*time.Hour), now.Add(52*24*time.Hour)), true},
		{"expired", creds(now.Add(-61*24*time.Hour), now.Add(-time.Hour)), false},
		{"unreadable", []byte("{"), false},
	} {
		if got := p.NeedsRefresh(c.raw, now); got != c.want {
			t.Errorf("NeedsRefresh(%s) = %v, want %v", c.name, got, c.want)
		}
	}

	old := creds(now.Add(-8*24*time.Hour), now.Add(52*24*time.Hour))
	out, err := p.Refresh(ctx, old)
	if err != nil {
		t.Fatal(err)
	}
	var c credentials
	_ = json.Unmarshal(out, &c)
	if c.AccessToken != "long-2" || c.UserID != userID || !c.IssuedAt.Equal(now) || !c.ExpiresAt.Equal(now.Add(5183944*time.Second)) {
		t.Errorf("refreshed %+v", c)
	}
	if p.NeedsRefresh(out, now) {
		t.Error("a just-refreshed token still needs refreshing")
	}

	s.refreshCode, s.refreshBody = 500, `{"error":{"message":"unknown","code":1}}`
	out, err = p.Refresh(ctx, old)
	if err != nil || string(out) != string(old) {
		t.Errorf("transient failure with weeks left: got %v; want the current token kept", err)
	}
	nearlyGone := creds(now.Add(-60*24*time.Hour), now.Add(time.Hour))
	if _, err := p.Refresh(ctx, nearlyGone); err == nil || publisher.IsPermanent(err) {
		t.Errorf("transient failure with an hour left: got %v, want a retryable error", err)
	}

	s.refreshCode, s.refreshBody = 400, `{"error":{"message":"Session has expired","code":190}}`
	if _, err := p.Refresh(ctx, old); !publisher.IsPermanent(err) {
		t.Errorf("revoked token: got %v, want permanent", err)
	}
}

// net/http writes the whole request URL into a transport error, and the
// token and app secret travel in URLs here. None may reach an error.
func TestErrorsCarryNoSecrets(t *testing.T) {
	p, _ := newTest(t)
	p.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}
	_, errPub := p.Publish(context.Background(), fresh, publisher.Post{Text: "hi"}, files{})
	_, errRef := p.Refresh(context.Background(), creds(now.Add(-8*24*time.Hour), now.Add(time.Hour)))
	_, errEx := p.Exchange(context.Background(), "code-1", "", "https://postitheka.test/cb")
	for _, err := range []error{errPub, errRef, errEx} {
		if err == nil {
			t.Fatal("no error")
		}
		for _, secret := range []string{"long-1", "secret-1", "code-1"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error %q contains %s", err, secret)
			}
		}
	}
}

func TestRules(t *testing.T) {
	r := New("a", "b", nil).Rules()
	if r.TextCounting != publisher.CountEmojiBytes || r.MaxTextLength != 500 || r.MaxLinks != 5 {
		t.Errorf("text rules %+v", r)
	}
	issues := publisher.Validate(r, publisher.Post{Text: strings.Repeat("👍", 126)}) // 504 bytes
	if len(issues) != 1 || issues[0].Field != "text" {
		t.Errorf("126 emoji: issues %+v, want the text over 500", issues)
	}
}
