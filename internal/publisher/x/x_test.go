package x

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FACorreiaa/postitheka/internal/publisher"
)

// stub plays X: the token endpoint, the account lookup, the chunked upload
// and post creation. Every host is redirected to it, so a request to any
// other host shows up here too and can be caught.
type stub struct {
	mu       sync.Mutex
	requests []recorded
	segments map[string][][]byte // media id -> segments, by index

	tokenStatus  int    // 0 means 200
	tokenBody    string // "" means a fresh pair of tokens
	processing   int    // status checks that answer in_progress first
	finalState   string // "" means no processing_info at all
	failStep     string // a path suffix that answers failStatus
	failStatus   int
	failHeaders  http.Header
	failBody     string
	mediaCounter int
}

type recorded struct {
	Method, Host, Path, Query string
	Header                    http.Header
	Body                      []byte
}

func (s *stub) find(method, suffix string) []recorded {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []recorded
	for _, r := range s.requests {
		if r.Method == method && strings.HasSuffix(r.Path, suffix) {
			out = append(out, r)
		}
	}
	return out
}

func (s *stub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, recorded{r.Method, r.Host, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), body})
	w.Header().Set("Content-Type", "application/json")

	if s.failStep != "" && strings.HasSuffix(r.URL.Path, s.failStep) {
		for k, v := range s.failHeaders {
			w.Header()[k] = v
		}
		w.WriteHeader(s.failStatus)
		_, _ = io.WriteString(w, s.failBody)
		return
	}

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/2/oauth2/token":
		if s.tokenStatus != 0 {
			w.WriteHeader(s.tokenStatus)
		}
		if s.tokenBody != "" {
			_, _ = io.WriteString(w, s.tokenBody)
			return
		}
		_, _ = io.WriteString(w, `{"token_type":"bearer","expires_in":7200,"access_token":"at-1","refresh_token":"rt-1","scope":"tweet.write"}`)
	case r.Method == http.MethodGet && r.URL.Path == "/2/users/me":
		_, _ = io.WriteString(w, `{"data":{"id":"2244994945","name":"Postitheka","username":"postitheka"}}`)
	case r.Method == http.MethodPost && r.URL.Path == "/2/media/upload/initialize":
		s.mediaCounter++
		_, _ = fmt.Fprintf(w, `{"data":{"id":"90%d","media_key":"13_90%d","expires_after_secs":86400}}`, s.mediaCounter, s.mediaCounter)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/append"):
		id := strings.Split(r.URL.Path, "/")[4]
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		index, data := -1, []byte(nil)
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			raw, _ := io.ReadAll(part)
			switch part.FormName() {
			case "segment_index":
				index, _ = strconv.Atoi(string(raw))
			case "media":
				data = raw
			}
		}
		if s.segments == nil {
			s.segments = map[string][][]byte{}
		}
		for len(s.segments[id]) <= index {
			s.segments[id] = append(s.segments[id], nil)
		}
		s.segments[id][index] = data
		_, _ = io.WriteString(w, `{"data":{"expires_at":1}}`)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/finalize"):
		id := strings.Split(r.URL.Path, "/")[4]
		if s.finalState == "" {
			_, _ = fmt.Fprintf(w, `{"data":{"id":%q,"size":1}}`, id)
			return
		}
		_, _ = fmt.Fprintf(w, `{"data":{"id":%q,"processing_info":{"state":"pending","check_after_secs":1}}}`, id)
	case r.Method == http.MethodGet && r.URL.Path == "/2/media/upload":
		state := s.finalState
		if s.processing > 0 {
			s.processing--
			state = "in_progress"
		}
		extra := ""
		if state == "failed" {
			extra = `,"error":{"code":1,"name":"InvalidMedia","message":"file is not a video"}`
		}
		_, _ = fmt.Fprintf(w, `{"data":{"id":%q,"processing_info":{"state":%q,"check_after_secs":1%s}}}`, r.URL.Query().Get("media_id"), state, extra)
	case r.Method == http.MethodPost && r.URL.Path == "/2/tweets":
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"id":"1880028106020515840","text":"hi"}}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newPublisher(t *testing.T, s *stub) *Publisher {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.Host = r.URL.Host // what the request was meant for, as the stub records it
		r.URL.Scheme, r.URL.Host = target.Scheme, target.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
	p := New("client-id", "client-secret", client)
	p.maxPoll = time.Millisecond
	return p
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type memMedia map[string][]byte

func (m memMedia) Open(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := m[key]
	if !ok {
		return nil, errors.New("no such file")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (memMedia) PublicURL(context.Context, string, time.Duration) (string, error) {
	return "", errors.New("x uploads bytes; it must not ask for a URL")
}

func testCreds(t *testing.T) []byte {
	t.Helper()
	raw, _ := json.Marshal(credentials{
		AccessToken: "at-1", RefreshToken: "rt-1", ExpiresAt: time.Now().Add(2 * time.Hour),
		UserID: "2244994945", Username: "postitheka",
	})
	return raw
}

func TestAuthorizeURLCarriesThePKCEChallenge(t *testing.T) {
	p := newPublisher(t, &stub{})
	u, err := url.Parse(p.AuthorizeURL("state-1", "verifier-1", "https://postitheka.test/oauth/x/callback"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "x.com" || u.Path != "/i/oauth2/authorize" {
		t.Errorf("authorize URL is %s%s", u.Host, u.Path)
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("code_challenge") == "verifier-1" {
		t.Errorf("challenge %q by %q, want an S256 digest of the verifier", q.Get("code_challenge"), q.Get("code_challenge_method"))
	}
	if q.Get("scope") != "tweet.read tweet.write users.read media.write offline.access" {
		t.Errorf("scope %q", q.Get("scope"))
	}
	if q.Get("state") != "state-1" || q.Get("redirect_uri") != "https://postitheka.test/oauth/x/callback" || q.Get("client_id") != "client-id" {
		t.Errorf("query %v", q)
	}
	if strings.Contains(u.String(), "client-secret") {
		t.Error("the client secret is in the authorize URL")
	}
}

func TestExchangeConnectsTheAccount(t *testing.T) {
	s := &stub{}
	p := newPublisher(t, s)

	conn, err := p.Exchange(context.Background(), "code-1", "verifier-1", "https://postitheka.test/oauth/x/callback")
	if err != nil {
		t.Fatal(err)
	}
	if conn.DisplayName != "@postitheka" || conn.RemoteID != "2244994945" {
		t.Errorf("connection %q / %q", conn.DisplayName, conn.RemoteID)
	}
	var creds credentials
	if err := json.Unmarshal(conn.Credentials, &creds); err != nil {
		t.Fatal(err)
	}
	if creds.AccessToken != "at-1" || creds.RefreshToken != "rt-1" || creds.UserID != "2244994945" || creds.Username != "postitheka" {
		t.Errorf("credentials %+v", creds)
	}
	if left := time.Until(creds.ExpiresAt); left < 119*time.Minute || left > 121*time.Minute {
		t.Errorf("expires in %s, want two hours", left)
	}

	tok := s.find(http.MethodPost, "/2/oauth2/token")
	if len(tok) != 1 {
		t.Fatalf("%d token requests", len(tok))
	}
	// A confidential client proves itself with a Basic header, and PKCE
	// with the verifier the authorize URL was built from.
	if user, pass, ok := (&http.Request{Header: tok[0].Header}).BasicAuth(); !ok || user != "client-id" || pass != "client-secret" {
		t.Errorf("token request authorised as %q, want the client id and secret as Basic", user)
	}
	form, _ := url.ParseQuery(string(tok[0].Body))
	if form.Get("code") != "code-1" || form.Get("code_verifier") != "verifier-1" || form.Get("grant_type") != "authorization_code" ||
		form.Get("redirect_uri") != "https://postitheka.test/oauth/x/callback" {
		t.Errorf("token form %v", form)
	}
	if tok[0].Host != "api.x.com" {
		t.Errorf("token request went to %q", tok[0].Host)
	}
	me := s.find(http.MethodGet, "/2/users/me")
	if len(me) != 1 || me[0].Header.Get("Authorization") != "Bearer at-1" {
		t.Errorf("account lookup %+v", me)
	}
}

func TestExchangeFailureCarriesNoSecrets(t *testing.T) {
	s := &stub{tokenStatus: http.StatusBadRequest, tokenBody: `{"error":"invalid_request","error_description":"Value passed for the authorization code was invalid."}`}
	p := newPublisher(t, s)
	_, err := p.Exchange(context.Background(), "code-1", "verifier-1", "https://postitheka.test/oauth/x/callback")
	if err == nil {
		t.Fatal("a refused code connected an account")
	}
	for _, secret := range []string{"client-secret", "code-1", "verifier-1"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error %q carries %q", err, secret)
		}
	}
	if !strings.Contains(err.Error(), "invalid_request") {
		t.Errorf("error %q lost what X said", err)
	}
}

func TestNeedsRefreshLeavesRoomForALongPublish(t *testing.T) {
	p := newPublisher(t, &stub{})
	now := time.Now()
	creds := func(left time.Duration, refresh string) []byte {
		raw, _ := json.Marshal(credentials{AccessToken: "at", RefreshToken: refresh, ExpiresAt: now.Add(left), UserID: "1"})
		return raw
	}
	if p.NeedsRefresh(creds(90*time.Minute, "rt"), now) {
		t.Error("a token with 90 minutes left was refreshed")
	}
	if !p.NeedsRefresh(creds(30*time.Minute, "rt"), now) {
		t.Error("a token with 30 minutes left was kept: a long upload would outlive it")
	}
	if !p.NeedsRefresh(creds(-time.Hour, "rt"), now) {
		t.Error("an expired token was kept")
	}
	if p.NeedsRefresh(creds(-time.Hour, ""), now) {
		t.Error("asked to refresh with no refresh token")
	}
	if p.NeedsRefresh([]byte("not json"), now) {
		t.Error("asked to refresh unreadable credentials")
	}
}

func TestRefreshKeepsTheNewRefreshToken(t *testing.T) {
	s := &stub{tokenBody: `{"token_type":"bearer","expires_in":7200,"access_token":"at-2","refresh_token":"rt-2"}`}
	p := newPublisher(t, s)

	raw, err := p.Refresh(context.Background(), testCreds(t))
	if err != nil {
		t.Fatal(err)
	}
	var creds credentials
	_ = json.Unmarshal(raw, &creds)
	// X spends the old refresh token: losing the new one locks the account out.
	if creds.AccessToken != "at-2" || creds.RefreshToken != "rt-2" {
		t.Errorf("tokens %q / %q, want the new pair", creds.AccessToken, creds.RefreshToken)
	}
	if creds.UserID != "2244994945" || creds.Username != "postitheka" {
		t.Errorf("the account was lost in the refresh: %+v", creds)
	}
	tok := s.find(http.MethodPost, "/2/oauth2/token")
	form, _ := url.ParseQuery(string(tok[0].Body))
	if form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != "rt-1" {
		t.Errorf("refresh form %v", form)
	}
	if user, _, ok := (&http.Request{Header: tok[0].Header}).BasicAuth(); !ok || user != "client-id" {
		t.Error("the refresh was not authorised as the client")
	}
}

func TestRefreshOfARevokedGrantIsPermanent(t *testing.T) {
	s := &stub{tokenStatus: http.StatusBadRequest, tokenBody: `{"error":"invalid_request","error_description":"Value passed for the token was invalid."}`}
	p := newPublisher(t, s)
	_, err := p.Refresh(context.Background(), testCreds(t))
	if !publisher.IsPermanent(err) {
		t.Fatalf("got %v, want a permanent error: the grant is gone", err)
	}
	if strings.Contains(err.Error(), "rt-1") || strings.Contains(err.Error(), "client-secret") {
		t.Errorf("error %q carries a secret", err)
	}
}

func TestRefreshThatMayPassIsRetried(t *testing.T) {
	s := &stub{tokenStatus: http.StatusServiceUnavailable, tokenBody: `{}`}
	p := newPublisher(t, s)
	_, err := p.Refresh(context.Background(), testCreds(t))
	if err == nil || publisher.IsPermanent(err) {
		t.Fatalf("got %v, want an error that is retried", err)
	}
}

func TestPublishText(t *testing.T) {
	s := &stub{}
	p := newPublisher(t, s)

	res, err := p.Publish(context.Background(), testCreds(t), publisher.Post{Text: "launch day"}, memMedia{})
	if err != nil {
		t.Fatal(err)
	}
	if res.RemoteID != "1880028106020515840" || res.URL != "https://x.com/postitheka/status/1880028106020515840" {
		t.Errorf("result %+v", res)
	}
	posts := s.find(http.MethodPost, "/2/tweets")
	if len(posts) != 1 {
		t.Fatalf("%d posts created", len(posts))
	}
	if posts[0].Host != "api.x.com" || posts[0].Header.Get("Authorization") != "Bearer at-1" {
		t.Errorf("post sent to %q as %q", posts[0].Host, posts[0].Header.Get("Authorization"))
	}
	if got := strings.TrimSpace(string(posts[0].Body)); got != `{"text":"launch day"}` {
		t.Errorf("body %s", got)
	}
	if len(s.find(http.MethodPost, "/initialize")) != 0 {
		t.Error("a text post started an upload")
	}
}

func TestPublishImagesUploadsEachBeforePosting(t *testing.T) {
	s := &stub{}
	p := newPublisher(t, s)
	files := memMedia{"k1": []byte("PNG-ONE"), "k2": []byte("JPEG-TWO")}
	post := publisher.Post{Text: "two photos", Media: []publisher.Media{
		{Key: "k1", ContentType: "image/png", Size: 7, Filename: "a.png"},
		{Key: "k2", ContentType: "image/jpeg", Size: 8, Filename: "b.jpg"},
	}}

	if _, err := p.Publish(context.Background(), testCreds(t), post, files); err != nil {
		t.Fatal(err)
	}
	inits := s.find(http.MethodPost, "/initialize")
	if len(inits) != 2 {
		t.Fatalf("%d uploads started, want 2", len(inits))
	}
	var first struct {
		MediaType     string `json:"media_type"`
		TotalBytes    int64  `json:"total_bytes"`
		MediaCategory string `json:"media_category"`
	}
	_ = json.Unmarshal(inits[0].Body, &first)
	if first.MediaType != "image/png" || first.TotalBytes != 7 || first.MediaCategory != "tweet_image" {
		t.Errorf("first upload started as %+v", first)
	}
	if string(s.segments["901"][0]) != "PNG-ONE" || string(s.segments["902"][0]) != "JPEG-TWO" {
		t.Errorf("segments %q", s.segments)
	}
	if len(s.find(http.MethodPost, "/finalize")) != 2 {
		t.Error("an upload was not finished")
	}
	// No processing_info on finalize: the media is ready, nothing to wait for.
	if len(s.find(http.MethodGet, "/2/media/upload")) != 0 {
		t.Error("asked for the status of media that was already ready")
	}
	posts := s.find(http.MethodPost, "/2/tweets")
	if got := strings.TrimSpace(string(posts[0].Body)); got != `{"media":{"media_ids":["901","902"]},"text":"two photos"}` {
		t.Errorf("body %s", got)
	}
}

func TestPublishVideoGoesUpInSegmentsAndWaitsForProcessing(t *testing.T) {
	s := &stub{finalState: "succeeded", processing: 2}
	p := newPublisher(t, s)
	video := bytes.Repeat([]byte("v"), segmentSize+segmentSize/2)
	post := publisher.Post{Media: []publisher.Media{{Key: "k", ContentType: "video/mp4", Size: int64(len(video)), Filename: "clip.mp4"}}}

	if _, err := p.Publish(context.Background(), testCreds(t), post, memMedia{"k": video}); err != nil {
		t.Fatal(err)
	}
	var init struct {
		MediaCategory string `json:"media_category"`
	}
	_ = json.Unmarshal(s.find(http.MethodPost, "/initialize")[0].Body, &init)
	if init.MediaCategory != "tweet_video" {
		t.Errorf("category %q", init.MediaCategory)
	}
	got := s.segments["901"]
	if len(got) != 2 || len(got[0]) != segmentSize || len(got[1]) != segmentSize/2 {
		t.Fatalf("segments of %d and %d bytes, want %d and %d", len(got[0]), len(got[len(got)-1]), segmentSize, segmentSize/2)
	}
	checks := s.find(http.MethodGet, "/2/media/upload")
	if len(checks) != 3 {
		t.Errorf("%d status checks, want 3: two in progress, then succeeded", len(checks))
	}
	if q, _ := url.ParseQuery(checks[0].Query); q.Get("command") != "STATUS" || q.Get("media_id") != "901" {
		t.Errorf("status query %q", checks[0].Query)
	}
	posts := s.find(http.MethodPost, "/2/tweets")
	// A video alone: no text key at all rather than an empty one.
	if got := strings.TrimSpace(string(posts[0].Body)); got != `{"media":{"media_ids":["901"]}}` {
		t.Errorf("body %s", got)
	}
}

func TestMediaXCannotProcessFailsForGood(t *testing.T) {
	s := &stub{finalState: "failed"}
	p := newPublisher(t, s)
	post := publisher.Post{Media: []publisher.Media{{Key: "k", ContentType: "video/mp4", Size: 3, Filename: "clip.mp4"}}}

	_, err := p.Publish(context.Background(), testCreds(t), post, memMedia{"k": []byte("abc")})
	if !publisher.IsPermanent(err) || !strings.Contains(err.Error(), "file is not a video") {
		t.Fatalf("got %v, want a permanent error with X's reason", err)
	}
	if len(s.find(http.MethodPost, "/2/tweets")) != 0 {
		t.Error("a post was created with media X refused")
	}
}

func TestErrorClasses(t *testing.T) {
	reset := strconv.FormatInt(time.Now().Add(10*time.Minute).Unix(), 10)
	post := publisher.Post{Text: "hi", Media: []publisher.Media{{Key: "k", ContentType: "image/png", Size: 3, Filename: "a.png"}}}
	cases := []struct {
		name    string
		step    string
		status  int
		headers http.Header
		body    string
		check   func(t *testing.T, err error)
	}{
		{
			"a revoked token asks for a reconnect", "/2/tweets", http.StatusUnauthorized, nil,
			`{"title":"Unauthorized","type":"about:blank","status":401,"detail":"Unauthorized"}`,
			func(t *testing.T, err error) {
				if !publisher.NeedsReconnect(err) {
					t.Errorf("got %v", err)
				}
			},
		},
		{
			"a rate limit waits for the reset", "/2/tweets", http.StatusTooManyRequests,
			http.Header{"X-Rate-Limit-Reset": {reset}},
			`{"title":"Too Many Requests","detail":"Too Many Requests","status":429}`,
			func(t *testing.T, err error) {
				var ra *publisher.RetryAfterError
				if !errors.As(err, &ra) || ra.After < 9*time.Minute || ra.After > 11*time.Minute {
					t.Errorf("got %v, want a wait of about ten minutes", err)
				}
			},
		},
		{
			"a rate limit with no reset waits a window", "/initialize", http.StatusTooManyRequests, nil, `{}`,
			func(t *testing.T, err error) {
				var ra *publisher.RetryAfterError
				if !errors.As(err, &ra) || ra.After != rateLimitFallback {
					t.Errorf("got %v", err)
				}
			},
		},
		{
			"a refused post is permanent, with X's reason", "/2/tweets", http.StatusForbidden, nil,
			`{"title":"Forbidden","detail":"This user is not allowed to post a video longer than 20 minutes.","type":"about:blank","status":403}`,
			func(t *testing.T, err error) {
				if !publisher.IsPermanent(err) || publisher.NeedsReconnect(err) || !strings.Contains(err.Error(), "longer than 20 minutes") {
					t.Errorf("got %v", err)
				}
			},
		},
		{
			"an errors list is read too", "/initialize", http.StatusBadRequest, nil,
			`{"errors":[{"message":"media_type is invalid"}]}`,
			func(t *testing.T, err error) {
				if !publisher.IsPermanent(err) || !strings.Contains(err.Error(), "media_type is invalid") {
					t.Errorf("got %v", err)
				}
			},
		},
		{
			"X's own trouble during an upload is retried", "/append", http.StatusBadGateway, nil, ``,
			func(t *testing.T, err error) {
				if err == nil || publisher.IsPermanent(err) {
					t.Errorf("got %v, want an error that is retried: nothing was posted", err)
				}
			},
		},
		{
			"X's own trouble creating the post is not retried", "/2/tweets", http.StatusBadGateway, nil, ``,
			func(t *testing.T, err error) {
				if !publisher.IsPermanent(err) || !strings.Contains(err.Error(), "check the account before posting again") {
					t.Errorf("got %v, want a permanent error: the post may be live", err)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &stub{failStep: c.step, failStatus: c.status, failHeaders: c.headers, failBody: c.body}
			p := newPublisher(t, s)
			_, err := p.Publish(context.Background(), testCreds(t), post, memMedia{"k": []byte("abc")})
			c.check(t, err)
			if err != nil && strings.Contains(err.Error(), "at-1") {
				t.Errorf("error %q carries the token", err)
			}
		})
	}
}

// A connection that drops while the post is being created leaves no answer
// at all: the post may be live, so it must not be sent again.
func TestALostAnswerToCreatePostIsNotRetried(t *testing.T) {
	p := New("client-id", "client-secret", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection reset")
	})})
	_, err := p.Publish(context.Background(), testCreds(t), publisher.Post{Text: "hi"}, memMedia{})
	if !publisher.IsPermanent(err) {
		t.Fatalf("got %v, want a permanent error", err)
	}
}

func TestPublishRefusesCredentialsItCannotUse(t *testing.T) {
	p := newPublisher(t, &stub{})
	if _, err := p.Publish(context.Background(), []byte("{}"), publisher.Post{Text: "hi"}, memMedia{}); !publisher.IsPermanent(err) {
		t.Errorf("unreadable credentials: %v", err)
	}
	expired, _ := json.Marshal(credentials{AccessToken: "at", ExpiresAt: time.Now().Add(-time.Minute), UserID: "1"})
	if _, err := p.Publish(context.Background(), expired, publisher.Post{Text: "hi"}, memMedia{}); !publisher.NeedsReconnect(err) {
		t.Errorf("an expired token with nothing to refresh it: %v, want a reconnect", err)
	}
}

func TestRules(t *testing.T) {
	rules := New("id", "secret", nil).Rules()
	ok := publisher.Post{Text: strings.Repeat("a", 280)}
	if issues := publisher.Validate(rules, ok); len(issues) != 0 {
		t.Errorf("280 characters refused: %+v", issues)
	}
	// 257 letters, a space and a link of any length come to 281.
	long := publisher.Post{Text: strings.Repeat("a", 257) + " https://example.com/a/very/long/path/that/would/not/fit/otherwise"}
	if issues := publisher.Validate(rules, long); len(issues) != 1 || issues[0].Field != "text" {
		t.Errorf("issues %+v, want the text one over", issues)
	}
	cjk := publisher.Post{Text: strings.Repeat("日", 141)}
	if issues := publisher.Validate(rules, cjk); len(issues) != 1 {
		t.Errorf("141 CJK characters count 282: %+v", issues)
	}
	gif := publisher.Post{Media: []publisher.Media{{ContentType: "image/gif", Size: 1}}}
	if issues := publisher.Validate(rules, gif); len(issues) != 1 {
		t.Errorf("a GIF was accepted: %+v", issues)
	}
	mixed := publisher.Post{Media: []publisher.Media{{ContentType: "video/mp4", Size: 1}, {ContentType: "image/png", Size: 1}}}
	if issues := publisher.Validate(rules, mixed); len(issues) != 1 {
		t.Errorf("a video with a photo: %+v", issues)
	}
	tall := publisher.Post{Media: []publisher.Media{{ContentType: "video/mp4", Size: 1, Duration: time.Minute, Width: 300, Height: 1000}}}
	if issues := publisher.Validate(rules, tall); len(issues) != 1 {
		t.Errorf("a video narrower than 1:3: %+v", issues)
	}
}
