package instagram

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

const (
	userID    = "17841405793187218" // past 2^53: mangled if read as a float
	appScoped = "9007199254740993"  // what the code exchange calls user_id

	mediaPath   = "/v27.0/" + userID + "/media"
	publishPath = "/v27.0/" + userID + "/media_publish"
)

// stub plays the Instagram API: the code exchange, the token endpoints, the
// profile, containers, their status and publishing. Every host is redirected
// to it.
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

	wrapped      bool // answer the exchange and the profile inside "data", as the guide shows
	createStatus int  // non-zero: container creation answers this with createBody
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
	return s.findLocked(method, path)
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
		if form.Get("code") != "code-1" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error_type":"OAuthException","code":400,"error_message":"Invalid authorization code"}`)
			return
		}
		if s.wrapped {
			_, _ = fmt.Fprintf(w, `{"data":[{"access_token":"short-1","user_id":"%s","permissions":"instagram_business_basic"}]}`, appScoped)
			return
		}
		_, _ = fmt.Fprintf(w, `{"access_token":"short-1","user_id":%s}`, appScoped)
	case r.Method == http.MethodGet && r.URL.Path == "/access_token":
		if q.Get("access_token") != "short-1" || q.Get("client_secret") != "secret-1" || q.Get("grant_type") != "ig_exchange_token" {
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
		if q.Get("grant_type") != "ig_refresh_token" || q.Get("access_token") != "long-1" {
			s.graphError(w, http.StatusBadRequest, 100, "bad refresh")
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"long-2","token_type":"bearer","expires_in":5183944}`)
	case r.Method == http.MethodGet && r.URL.Path == "/v27.0/me":
		if q.Get("access_token") != "long-1" {
			s.graphError(w, http.StatusBadRequest, 190, "Invalid OAuth access token")
			return
		}
		me := fmt.Sprintf(`{"user_id":%q,"username":"ada","name":"Ada Lovelace","id":%q}`, userID, appScoped)
		if s.wrapped {
			me = `{"data":[` + me + `]}`
		}
		_, _ = io.WriteString(w, me)
	case r.Method == http.MethodPost && r.URL.Path == mediaPath:
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
	case r.Method == http.MethodPost && r.URL.Path == publishPath:
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
	case r.Method == http.MethodGet && r.URL.Path == "/v27.0/m-1":
		_, _ = io.WriteString(w, `{"permalink":"https://www.instagram.com/p/ABC/","id":"m-1"}`)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v27.0/c-"):
		id := strings.TrimPrefix(r.URL.Path, "/v27.0/")
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.statusBroken && len(s.findLocked(http.MethodPost, publishPath)) > 0 {
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
		_, _ = fmt.Fprintf(w, `{"id":%q,"status_code":%q,"status":%q}`, id, status, s.finalError)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

var now = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

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
	return nil, errors.New("instagram never reads media itself")
}

func (files) PublicURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.test/" + key + "?signed", nil
}

func creds(issued, expires time.Time) []byte {
	raw, _ := json.Marshal(credentials{AccessToken: "long-1", IssuedAt: issued, ExpiresAt: expires, UserID: userID})
	return raw
}

var (
	fresh = creds(now.Add(-time.Hour), now.Add(59*24*time.Hour))
	photo = publisher.Post{Text: "look", Media: []publisher.Media{{Key: "a.jpg", ContentType: "image/jpeg"}}}
)

func form(t *testing.T, r recorded) url.Values {
	t.Helper()
	v, err := url.ParseQuery(string(r.Body))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestConnect(t *testing.T) {
	const redirect = "https://postitheka.test/oauth/instagram/callback"

	for name, wrapped := range map[string]bool{"flat answers": false, "answers inside data": true} {
		t.Run(name, func(t *testing.T) {
			p, s := newTest(t)
			s.wrapped = wrapped

			auth, _ := url.Parse(p.AuthorizeURL("st-1", "verifier-1", redirect))
			q := auth.Query()
			if auth.Host != "www.instagram.com" || auth.Path != "/oauth/authorize" {
				t.Errorf("authorize URL %s", auth)
			}
			for k, want := range map[string]string{
				"client_id": "app-1", "redirect_uri": redirect, "state": "st-1", "response_type": "code",
				"scope": "instagram_business_basic,instagram_business_content_publish",
			} {
				if q.Get(k) != want {
					t.Errorf("authorize %s = %q, want %q", k, q.Get(k), want)
				}
			}
			if q.Has("code_challenge") {
				t.Error("sent a PKCE challenge Instagram has no use for")
			}

			conn, err := p.Exchange(context.Background(), "code-1", "verifier-1", redirect)
			if err != nil {
				t.Fatal(err)
			}
			tok := s.find(http.MethodPost, "/oauth/access_token")
			if len(tok) != 1 {
				t.Fatalf("%d token requests", len(tok))
			}
			if tok[0].Host != "api.instagram.com" {
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
			for _, r := range s.requests[1:] {
				if r.Host != "graph.instagram.com" {
					t.Errorf("%s %s went to %s", r.Method, r.Path, r.Host)
				}
			}

			// The account id comes from the profile, never from the exchange.
			if conn.DisplayName != "@ada" || conn.RemoteID != userID {
				t.Errorf("connection %q %q", conn.DisplayName, conn.RemoteID)
			}
			var c credentials
			if err := json.Unmarshal(conn.Credentials, &c); err != nil {
				t.Fatal(err)
			}
			if c.AccessToken != "long-1" || c.UserID != userID || !c.IssuedAt.Equal(now) || !c.ExpiresAt.Equal(now.Add(5183944*time.Second)) {
				t.Errorf("stored %+v; want the long-lived token, the professional account id, issued now", c)
			}
		})
	}

	t.Run("a refused code says why", func(t *testing.T) {
		p, _ := newTest(t)
		_, err := p.Exchange(context.Background(), "code-2", "", redirect)
		if err == nil || !strings.Contains(err.Error(), "Invalid authorization code") {
			t.Errorf("got %v, want Instagram's own message", err)
		}
	})
}

func TestPublishMedia(t *testing.T) {
	t.Run("one image", func(t *testing.T) {
		p, s := newTest(t)
		res, err := p.Publish(context.Background(), fresh, photo, files{})
		if err != nil {
			t.Fatal(err)
		}
		if res.RemoteID != "m-1" || res.URL != "https://www.instagram.com/p/ABC/" {
			t.Errorf("result %+v", res)
		}
		created := s.find(http.MethodPost, mediaPath)
		if len(created) != 1 {
			t.Fatalf("%d containers", len(created))
		}
		f := form(t, created[0])
		if f.Has("media_type") || f.Get("image_url") != "https://storage.test/a.jpg?signed" || f.Get("caption") != "look" || f.Get("access_token") != "long-1" {
			t.Errorf("container form %v", f)
		}
		pub := s.find(http.MethodPost, publishPath)
		if len(pub) != 1 || form(t, pub[0]).Get("creation_id") != "c-1" {
			t.Fatalf("publish requests %+v", pub)
		}
		for _, r := range s.requests {
			if r.Method == http.MethodPost && strings.Contains(r.Query, "access_token") {
				t.Errorf("POST %s carried the token in its URL", r.Path)
			}
			if r.Host != "graph.instagram.com" {
				t.Errorf("request to %s", r.Host)
			}
		}
	})

	t.Run("one video is a reel, processed before publishing", func(t *testing.T) {
		p, s := newTest(t)
		s.inProgress = 3
		post := publisher.Post{Media: []publisher.Media{{Key: "v.mov", ContentType: "video/quicktime"}}}
		if _, err := p.Publish(context.Background(), fresh, post, files{}); err != nil {
			t.Fatal(err)
		}
		f := form(t, s.find(http.MethodPost, mediaPath)[0])
		if f.Get("media_type") != "REELS" || f.Get("video_url") != "https://storage.test/v.mov?signed" || f.Has("caption") || f.Has("is_carousel_item") {
			t.Errorf("container form %v", f)
		}
		if n := len(s.find(http.MethodGet, "/v27.0/c-1")); n != 4 {
			t.Errorf("%d status checks, want 3 in progress and 1 finished", n)
		}
	})

	t.Run("carousel", func(t *testing.T) {
		p, s := newTest(t)
		post := publisher.Post{Text: "three", Media: []publisher.Media{
			{Key: "1.jpg", ContentType: "image/jpeg"},
			{Key: "2.mp4", ContentType: "video/mp4"},
			{Key: "3.jpg", ContentType: "image/jpeg"},
		}}
		if _, err := p.Publish(context.Background(), fresh, post, files{}); err != nil {
			t.Fatal(err)
		}
		created := s.find(http.MethodPost, mediaPath)
		if len(created) != 4 {
			t.Fatalf("%d containers, want 3 items and the carousel", len(created))
		}
		// A video in a carousel is a VIDEO: reels cannot be carousel items.
		for i, want := range []string{"", "VIDEO", ""} {
			f := form(t, created[i])
			if f.Get("media_type") != want || f.Get("is_carousel_item") != "true" || f.Has("caption") {
				t.Errorf("item %d form %v", i, f)
			}
		}
		c := form(t, created[3])
		if c.Get("media_type") != "CAROUSEL" || c.Get("children") != "c-1,c-2,c-3" || c.Get("caption") != "three" {
			t.Errorf("carousel form %v", c)
		}
		if got := strings.Join(s.polledFirst, ","); got != "c-1,c-2,c-3,c-4" {
			t.Errorf("status checked in order %s; every item must finish before the carousel is made", got)
		}
		if form(t, s.find(http.MethodPost, publishPath)[0]).Get("creation_id") != "c-4" {
			t.Error("published something other than the carousel")
		}
	})

	t.Run("no media never reaches Instagram", func(t *testing.T) {
		p, s := newTest(t)
		_, err := p.Publish(context.Background(), fresh, publisher.Post{Text: "words only"}, files{})
		if !publisher.IsPermanent(err) {
			t.Errorf("got %v, want permanent", err)
		}
		if len(s.requests) != 0 {
			t.Errorf("%d requests for a post Instagram cannot take", len(s.requests))
		}
	})
}

func TestContainerLifecycle(t *testing.T) {
	video := publisher.Post{Media: []publisher.Media{{Key: "v.mp4", ContentType: "video/mp4"}}}

	t.Run("ERROR is permanent and says why", func(t *testing.T) {
		p, s := newTest(t)
		s.final, s.finalError = "ERROR", "Error: Media upload has failed with error code 2207026"
		_, err := p.Publish(context.Background(), fresh, video, files{})
		if !publisher.IsPermanent(err) || !strings.Contains(err.Error(), "2207026") {
			t.Errorf("got %v, want a permanent error with Instagram's reason", err)
		}
		if len(s.find(http.MethodPost, publishPath)) != 0 {
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
		if len(s.find(http.MethodPost, publishPath)) != 0 {
			t.Error("published an unfinished container")
		}
	})
}

// A publish that fails without a clear answer may still have gone out. The
// container says which, and only "not published" may be retried.
func TestPublishWithoutAClearAnswer(t *testing.T) {
	t.Run("failed but live counts as published", func(t *testing.T) {
		p, s := newTest(t)
		s.publishMode = "500-live"
		if _, err := p.Publish(context.Background(), fresh, photo, files{}); err != nil {
			t.Errorf("got %v; the post is live, so this is a success", err)
		}
	})

	t.Run("failed and not live is retried", func(t *testing.T) {
		p, s := newTest(t)
		s.publishMode = "500"
		_, err := p.Publish(context.Background(), fresh, photo, files{})
		if err == nil || publisher.IsPermanent(err) {
			t.Errorf("got %v, want a retryable error", err)
		}
	})

	t.Run("failed and unknowable stops and says check Instagram", func(t *testing.T) {
		p, s := newTest(t)
		s.publishMode = "500"
		s.statusBroken = true
		_, err := p.Publish(context.Background(), fresh, photo, files{})
		if !publisher.IsPermanent(err) || !strings.Contains(err.Error(), "check Instagram before posting again") {
			t.Errorf("got %v, want a permanent 'check Instagram'", err)
		}
	})

	t.Run("a refusal is believed without checking", func(t *testing.T) {
		p, s := newTest(t)
		s.publishMode = "400"
		_, err := p.Publish(context.Background(), fresh, photo, files{})
		if !publisher.IsPermanent(err) {
			t.Errorf("got %v, want permanent", err)
		}
		if n := len(s.find(http.MethodGet, "/v27.0/c-1")); n != 1 {
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
			if strings.HasSuffix(r.URL.Path, "/media_publish") {
				cancel() // the job's deadline passes while Instagram answers
			}
			return resp, err
		})}
		if _, err := p.Publish(ctx, fresh, photo, files{}); err != nil {
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
		{"100 posts in 24 hours", 400, `{"error":{"message":"Application request limit reached","code":9,"error_subcode":2207042}}`, false, time.Hour},
		{"spam restriction", 400, `{"error":{"message":"Action blocked","code":4,"error_subcode":2207051}}`, true, 0},
		{"temporary", 500, `{"error":{"message":"An unknown error occurred","code":1}}`, false, 0},
		{"bad gateway", 502, `<html>`, false, 0},
		{"download timed out", 400, `{"error":{"message":"Timeout","code":-2,"error_subcode":2207003}}`, false, 0},
		{"container lost", 400, `{"error":{"message":"Temporary error","code":24,"error_subcode":2207008}}`, false, 0},
		{"not ready", 400, `{"error":{"message":"Media not ready","code":9007,"error_subcode":2207027}}`, false, 0},
		{"media not fetchable", 400, `{"error":{"message":"Media could not be fetched","code":9004,"error_subcode":2207052}}`, true, 0},
		{"wrong aspect ratio", 400, `{"error":{"message":"Invalid aspect ratio","code":36003,"error_subcode":2207009}}`, true, 0},
		{"account restricted", 400, `{"error":{"message":"Account restricted","code":25,"error_subcode":2207050}}`, true, 0},
		{"no permission", 403, `{"error":{"message":"Permissions error","code":10}}`, true, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, s := newTest(t)
			s.createStatus, s.createBody = c.status, c.body
			_, err := p.Publish(context.Background(), fresh, photo, files{})
			if err == nil {
				t.Fatal("no error")
			}
			if publisher.NeedsReconnect(err) != (c.name == "expired token") {
				t.Errorf("reconnect = %v: %v", publisher.NeedsReconnect(err), err)
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

	t.Run("the message written for people wins", func(t *testing.T) {
		p, s := newTest(t)
		s.createStatus = 400
		s.createBody = `{"error":{"message":"Invalid parameter","code":100,"error_user_msg":"The image is too large."}}`
		_, err := p.Publish(context.Background(), fresh, photo, files{})
		if err == nil || !strings.Contains(err.Error(), "The image is too large.") {
			t.Errorf("got %v", err)
		}
	})
}

func TestPublishRefusesBadCredentials(t *testing.T) {
	p, s := newTest(t)
	for name, raw := range map[string][]byte{
		"unreadable": []byte("{"),
		"no user":    []byte(`{"access_token":"long-1"}`),
		"expired":    creds(now.Add(-61*24*time.Hour), now.Add(-time.Hour)),
	} {
		_, err := p.Publish(context.Background(), raw, photo, files{})
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
	_, errPub := p.Publish(context.Background(), fresh, photo, files{})
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
	jpeg := publisher.Media{ContentType: "image/jpeg", Size: 1_000_000}

	for name, c := range map[string]struct {
		post  publisher.Post
		field string // "" means accepted
	}{
		"photo and caption": {publisher.Post{Text: "hi", Media: []publisher.Media{jpeg}}, ""},
		"photo, no caption": {publisher.Post{Media: []publisher.Media{jpeg}}, ""},
		"words only":        {publisher.Post{Text: "hi"}, "media"},
		"caption over 2,200": {
			publisher.Post{Text: strings.Repeat("a", 2201), Media: []publisher.Media{jpeg}}, "text",
		},
		"png": {
			publisher.Post{Media: []publisher.Media{{ContentType: "image/png", Size: 1000}}}, "media",
		},
		"image over 8 MB": {
			publisher.Post{Media: []publisher.Media{{ContentType: "image/jpeg", Size: 8_000_001}}}, "media",
		},
		"video over an image's limit": {
			publisher.Post{Media: []publisher.Media{{ContentType: "video/mp4", Size: 200_000_000}}}, "",
		},
		"video over 300 MB": {
			publisher.Post{Media: []publisher.Media{{ContentType: "video/mp4", Size: 300_000_001}}}, "media",
		},
	} {
		issues := publisher.Validate(r, c.post)
		switch {
		case c.field == "" && len(issues) != 0:
			t.Errorf("%s: refused: %+v", name, issues)
		case c.field != "" && (len(issues) != 1 || !strings.HasPrefix(issues[0].Field, c.field)):
			t.Errorf("%s: issues %+v, want one on %s", name, issues, c.field)
		}
	}
}
