package facebook

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
	pageID    = "104857600123456789" // past 2^53: mangled if read as a float
	feedPath  = "/v27.0/" + pageID + "/feed"
	photoPath = "/v27.0/" + pageID + "/photos"
	videoPath = "/v27.0/" + pageID + "/videos"
)

// stub plays the Graph API: the token endpoint, the list of Pages, and the
// three edges a Page is posted to. Every host is redirected to it.
type stub struct {
	mu       sync.Mutex
	requests []recorded
	nextID   int

	pages [][]string // the Pages answer, one JSON object per Page, in pages of results

	failStatus int // non-zero: failPath answers this with failBody
	failBody   string
	failPath   string
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

func graphError(w http.ResponseWriter, status, code int, msg string) {
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"message":%q,"type":"OAuthException","code":%d,"fbtrace_id":"x"}}`, msg, code)
}

func (s *stub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, recorded{r.Method, r.Host, r.URL.Path, r.URL.RawQuery, body})
	s.nextID++
	n := s.nextID
	s.mu.Unlock()
	form, _ := url.ParseQuery(string(body))
	q := r.URL.Query()
	w.Header().Set("Content-Type", "application/json")

	if s.failStatus != 0 && r.URL.Path == s.failPath {
		w.WriteHeader(s.failStatus)
		_, _ = io.WriteString(w, s.failBody)
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v27.0/oauth/access_token":
		switch {
		case q.Get("client_id") != "app-1" || q.Get("client_secret") != "secret-1":
			graphError(w, http.StatusBadRequest, 101, "Error validating application")
		case q.Get("grant_type") == "fb_exchange_token" && q.Get("fb_exchange_token") == "short-1":
			_, _ = io.WriteString(w, `{"access_token":"long-1","token_type":"bearer","expires_in":5183944}`)
		case q.Get("code") == "code-1" && q.Get("redirect_uri") != "":
			_, _ = io.WriteString(w, `{"access_token":"short-1","token_type":"bearer","expires_in":5000}`)
		default:
			graphError(w, http.StatusBadRequest, 100, "This authorization code has been used.")
		}
	case r.Method == http.MethodGet && r.URL.Path == "/v27.0/me/accounts":
		if q.Get("access_token") != "long-1" {
			graphError(w, http.StatusBadRequest, 190, "Invalid OAuth access token")
			return
		}
		i := 0
		if a := q.Get("after"); a != "" {
			_, _ = fmt.Sscanf(a, "cursor-%d", &i)
		}
		if i >= len(s.pages) {
			_, _ = io.WriteString(w, `{"data":[]}`)
			return
		}
		paging := fmt.Sprintf(`{"cursors":{"after":"cursor-%d"}}`, i+1)
		if i+1 < len(s.pages) {
			// A real "next" is a whole URL with the token in it, on a host
			// of Facebook's choosing. It must never be followed.
			paging = fmt.Sprintf(`{"cursors":{"after":"cursor-%d"},"next":"https://evil.test/v27.0/me/accounts?access_token=long-1"}`, i+1)
		}
		_, _ = fmt.Fprintf(w, `{"data":[%s],"paging":%s}`, strings.Join(s.pages[i], ","), paging)
	case r.Method == http.MethodPost && r.URL.Path == feedPath:
		_, _ = fmt.Fprintf(w, `{"id":"%s_%d"}`, pageID, n)
	case r.Method == http.MethodPost && r.URL.Path == photoPath:
		if form.Get("published") == "false" {
			_, _ = fmt.Fprintf(w, `{"id":"photo-%d"}`, n)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":"photo-%d","post_id":"%s_%d"}`, n, pageID, n)
	case r.Method == http.MethodPost && r.URL.Path == videoPath:
		_, _ = fmt.Fprintf(w, `{"id":"video-%d"}`, n)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/comments"):
		_, _ = fmt.Fprintf(w, `{"id":"%s_%d"}`, strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v27.0/"), "/comments"), n)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newTest(t *testing.T) (*Publisher, *stub) {
	t.Helper()
	s := &stub{}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.Host = r.URL.Host
		r.URL.Scheme, r.URL.Host = target.Scheme, target.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
	return New("app-1", "secret-1", client), s
}

// files hands out URLs on a storage host, as presigned GETs would be.
type files struct{}

func (files) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("facebook never reads media itself")
}

func (files) PublicURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.test/" + key + "?signed", nil
}

var creds, _ = json.Marshal(credentials{AccessToken: "page-token-1", PageID: pageID})

func form(t *testing.T, r recorded) url.Values {
	t.Helper()
	v, err := url.ParseQuery(string(r.Body))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func page(id, name, token string, tasks ...string) string {
	raw, _ := json.Marshal(map[string]any{"id": id, "name": name, "access_token": token, "tasks": tasks})
	return string(raw)
}

func TestConnect(t *testing.T) {
	const redirect = "https://postitheka.test/oauth/facebook/callback"
	p, s := newTest(t)
	s.pages = [][]string{
		{
			page(pageID, "Bakery", "page-token-1", "ANALYZE", "CREATE_CONTENT", "MANAGE"),
			page("222", "Only Moderated", "page-token-2", "MODERATE", "ANALYZE"),
		},
		{page("333", "Bike Shop", "page-token-3", "CREATE_CONTENT")},
	}

	auth, _ := url.Parse(p.AuthorizeURL("st-1", "verifier-1", redirect))
	q := auth.Query()
	if auth.Host != "www.facebook.com" || auth.Path != "/v27.0/dialog/oauth" {
		t.Errorf("authorize URL %s", auth)
	}
	for k, want := range map[string]string{
		"client_id": "app-1", "redirect_uri": redirect, "state": "st-1", "response_type": "code",
		"scope": "pages_show_list,pages_read_engagement,pages_manage_posts,pages_manage_engagement",
	} {
		if q.Get(k) != want {
			t.Errorf("authorize %s = %q, want %q", k, q.Get(k), want)
		}
	}
	if q.Has("code_challenge") {
		t.Error("sent a PKCE challenge Facebook's server flow has no use for")
	}

	conns, err := p.ExchangeAll(context.Background(), "code-1", "verifier-1", redirect)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 2 {
		t.Fatalf("%d connections, want the two Pages this person can post to", len(conns))
	}
	for i, want := range []struct{ name, id, token string }{{"Bakery", pageID, "page-token-1"}, {"Bike Shop", "333", "page-token-3"}} {
		var c credentials
		if err := json.Unmarshal(conns[i].Credentials, &c); err != nil {
			t.Fatal(err)
		}
		if conns[i].DisplayName != want.name || conns[i].RemoteID != want.id || c.PageID != want.id || c.AccessToken != want.token {
			t.Errorf("connection %d: %q %q %+v", i, conns[i].DisplayName, conns[i].RemoteID, c)
		}
	}

	tok := s.find(http.MethodGet, "/v27.0/oauth/access_token")
	if len(tok) != 2 {
		t.Fatalf("%d token requests, want the code exchange and the long-lived exchange", len(tok))
	}
	if cq, _ := url.ParseQuery(tok[0].Query); cq.Get("redirect_uri") != redirect || cq.Get("code") != "code-1" {
		t.Errorf("code exchange %v", cq)
	}
	lists := s.find(http.MethodGet, "/v27.0/me/accounts")
	if len(lists) != 2 {
		t.Fatalf("%d page list requests, want both pages of results", len(lists))
	}
	if lq, _ := url.ParseQuery(lists[1].Query); lq.Get("after") != "cursor-1" {
		t.Errorf("second page asked with %v", lq)
	}
	for _, r := range s.requests {
		if r.Host != "graph.facebook.com" {
			t.Errorf("%s %s went to %s", r.Method, r.Path, r.Host)
		}
	}

	first, err := p.Exchange(context.Background(), "code-1", "", redirect)
	if err != nil || first.RemoteID != pageID {
		t.Errorf("Exchange: %+v, %v; want the first Page", first, err)
	}

	t.Run("no Page to post to is an error", func(t *testing.T) {
		p, s := newTest(t)
		s.pages = [][]string{{page("222", "Only Moderated", "page-token-2", "MODERATE")}}
		if _, err := p.ExchangeAll(context.Background(), "code-1", "", redirect); !errors.Is(err, ErrNoPages) {
			t.Errorf("got %v, want ErrNoPages", err)
		}
	})

	t.Run("a refused code says why", func(t *testing.T) {
		p, _ := newTest(t)
		_, err := p.ExchangeAll(context.Background(), "code-2", "", redirect)
		if err == nil || !strings.Contains(err.Error(), "authorization code has been used") {
			t.Errorf("got %v, want Facebook's own message", err)
		}
	})
}

func TestPublish(t *testing.T) {
	ctx := context.Background()

	t.Run("text", func(t *testing.T) {
		p, s := newTest(t)
		res, err := p.Publish(ctx, creds, publisher.Post{Text: "hello 👋"}, files{})
		if err != nil {
			t.Fatal(err)
		}
		if res.RemoteID != pageID+"_1" || res.URL != "https://www.facebook.com/"+pageID+"_1" {
			t.Errorf("result %+v", res)
		}
		posts := s.find(http.MethodPost, feedPath)
		if len(posts) != 1 {
			t.Fatalf("%d feed posts", len(posts))
		}
		f := form(t, posts[0])
		if f.Get("message") != "hello 👋" || f.Get("access_token") != "page-token-1" {
			t.Errorf("form %v", f)
		}
		if posts[0].Query != "" || posts[0].Host != "graph.facebook.com" {
			t.Errorf("request to %s with %q in its URL", posts[0].Host, posts[0].Query)
		}
	})

	t.Run("one photo", func(t *testing.T) {
		p, s := newTest(t)
		post := publisher.Post{Text: "look", Media: []publisher.Media{{Key: "a.png", ContentType: "image/png"}}}
		res, err := p.Publish(ctx, creds, post, files{})
		if err != nil {
			t.Fatal(err)
		}
		// The post's id, not the photo's.
		if res.RemoteID != pageID+"_1" || res.URL != "https://www.facebook.com/"+pageID+"_1" {
			t.Errorf("result %+v", res)
		}
		f := form(t, s.find(http.MethodPost, photoPath)[0])
		if f.Get("url") != "https://storage.test/a.png?signed" || f.Get("caption") != "look" || f.Has("published") {
			t.Errorf("form %v", f)
		}
		if len(s.find(http.MethodPost, feedPath)) != 0 {
			t.Error("a single photo also made a feed post")
		}
	})

	t.Run("several photos become one post", func(t *testing.T) {
		p, s := newTest(t)
		post := publisher.Post{Text: "three", Media: []publisher.Media{
			{Key: "1.jpg", ContentType: "image/jpeg"},
			{Key: "2.png", ContentType: "image/png"},
			{Key: "3.jpg", ContentType: "image/jpeg"},
		}}
		res, err := p.Publish(ctx, creds, post, files{})
		if err != nil {
			t.Fatal(err)
		}
		uploads := s.find(http.MethodPost, photoPath)
		if len(uploads) != 3 {
			t.Fatalf("%d uploads", len(uploads))
		}
		for i, u := range uploads {
			f := form(t, u)
			if f.Get("published") != "false" || f.Has("caption") || !strings.Contains(f.Get("url"), fmt.Sprintf("/%d.", i+1)) {
				t.Errorf("upload %d form %v", i, f)
			}
		}
		posts := s.find(http.MethodPost, feedPath)
		if len(posts) != 1 {
			t.Fatalf("%d feed posts, want 1", len(posts))
		}
		f := form(t, posts[0])
		if f.Get("message") != "three" {
			t.Errorf("message %q", f.Get("message"))
		}
		for i := range 3 {
			want := fmt.Sprintf(`{"media_fbid":"photo-%d"}`, i+1)
			if got := f.Get(fmt.Sprintf("attached_media[%d]", i)); got != want {
				t.Errorf("attached_media[%d] = %s, want %s", i, got, want)
			}
		}
		if res.RemoteID != pageID+"_4" {
			t.Errorf("result %+v", res)
		}
	})

	t.Run("one video, on the video host", func(t *testing.T) {
		p, s := newTest(t)
		post := publisher.Post{Text: "watch", Media: []publisher.Media{{Key: "v.mp4", ContentType: "video/mp4"}}}
		res, err := p.Publish(ctx, creds, post, files{})
		if err != nil {
			t.Fatal(err)
		}
		if res.RemoteID != "video-1" || res.URL != "" {
			t.Errorf("result %+v; want the video id and no guessed link", res)
		}
		vids := s.find(http.MethodPost, videoPath)
		if len(vids) != 1 || vids[0].Host != "graph-video.facebook.com" {
			t.Fatalf("video requests %+v", vids)
		}
		f := form(t, vids[0])
		if f.Get("file_url") != "https://storage.test/v.mp4?signed" || f.Get("description") != "watch" {
			t.Errorf("form %v", f)
		}
	})

	t.Run("a video with anything else never reaches Facebook", func(t *testing.T) {
		p, s := newTest(t)
		post := publisher.Post{Media: []publisher.Media{
			{Key: "v.mp4", ContentType: "video/mp4"}, {Key: "a.jpg", ContentType: "image/jpeg"},
		}}
		if _, err := p.Publish(ctx, creds, post, files{}); !publisher.IsPermanent(err) {
			t.Errorf("got %v, want permanent", err)
		}
		if len(s.requests) != 0 {
			t.Errorf("%d requests", len(s.requests))
		}
	})

	t.Run("bad credentials", func(t *testing.T) {
		p, s := newTest(t)
		for name, raw := range map[string][]byte{"unreadable": []byte("{"), "no page": []byte(`{"access_token":"t"}`)} {
			if _, err := p.Publish(ctx, raw, publisher.Post{Text: "hi"}, files{}); !publisher.IsPermanent(err) {
				t.Errorf("%s: got %v, want permanent", name, err)
			}
		}
		if len(s.requests) != 0 {
			t.Errorf("%d requests sent with bad credentials", len(s.requests))
		}
	})
}

// There is no way to ask Facebook whether a post landed, so only a clear
// refusal may be retried. Anything else stops and says to look.
func TestPublishWithoutAClearAnswer(t *testing.T) {
	ctx := context.Background()
	album := publisher.Post{Text: "two", Media: []publisher.Media{
		{Key: "1.jpg", ContentType: "image/jpeg"}, {Key: "2.jpg", ContentType: "image/jpeg"},
	}}

	for name, c := range map[string]struct {
		status int
		body   string
	}{
		"server error":            {500, `{"error":{"message":"An unknown error occurred","code":1}}`},
		"unknown error as a 400":  {400, `{"error":{"message":"An unknown error occurred","code":1}}`},
		"bad gateway":             {502, `<html>`},
		"a 400 that is not Graph": {400, `<html>`},
	} {
		t.Run(name, func(t *testing.T) {
			p, s := newTest(t)
			s.failPath, s.failStatus, s.failBody = feedPath, c.status, c.body
			_, err := p.Publish(ctx, creds, publisher.Post{Text: "hi"}, files{})
			if !publisher.IsPermanent(err) || !strings.Contains(err.Error(), "check the Page before posting again") {
				t.Errorf("got %v, want a permanent 'check the Page'", err)
			}
		})
	}

	t.Run("no answer at all", func(t *testing.T) {
		p, _ := newTest(t)
		p.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection reset")
		})}
		_, err := p.Publish(ctx, creds, publisher.Post{Text: "hi"}, files{})
		if !publisher.IsPermanent(err) {
			t.Errorf("got %v, want permanent", err)
		}
	})

	t.Run("a failed upload is retried: nothing is on the Page yet", func(t *testing.T) {
		p, s := newTest(t)
		s.failPath, s.failStatus, s.failBody = photoPath, 500, `{"error":{"message":"An unknown error occurred","code":1}}`
		_, err := p.Publish(ctx, creds, album, files{})
		if err == nil || publisher.IsPermanent(err) {
			t.Errorf("got %v, want a retryable error", err)
		}
		if len(s.find(http.MethodPost, feedPath)) != 0 {
			t.Error("published a post whose photos did not upload")
		}
	})
}

func TestErrorClassification(t *testing.T) {
	for _, c := range []struct {
		name      string
		status    int
		body      string
		permanent bool
		reconnect bool
		wait      time.Duration
	}{
		{"dead token", 400, `{"error":{"message":"Error validating access token","code":190}}`, true, true, 0},
		{"unauthorized", 401, `{"error":{"message":"Unauthorized","code":0}}`, true, true, 0},
		{"rate limited", 400, `{"error":{"message":"Application request limit reached","code":4}}`, false, false, time.Hour},
		{"page rate limited", 400, `{"error":{"message":"Page request limit reached","code":32}}`, false, false, time.Hour},
		{"too many requests", 429, ``, false, false, time.Hour},
		{"abusive", 400, `{"error":{"message":"The action attempted has been deemed abusive","code":368}}`, true, false, 0},
		{"no permission", 403, `{"error":{"message":"Permissions error","code":200}}`, true, false, 0},
		{"invalid parameter", 400, `{"error":{"message":"Invalid parameter","code":100}}`, true, false, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, s := newTest(t)
			s.failPath, s.failStatus, s.failBody = feedPath, c.status, c.body
			_, err := p.Publish(context.Background(), creds, publisher.Post{Text: "hi"}, files{})
			if err == nil {
				t.Fatal("no error")
			}
			if publisher.IsPermanent(err) != c.permanent || publisher.NeedsReconnect(err) != c.reconnect {
				t.Errorf("permanent %v reconnect %v, want %v %v: %v", publisher.IsPermanent(err), publisher.NeedsReconnect(err), c.permanent, c.reconnect, err)
			}
			var ra *publisher.RetryAfterError
			if got := errors.As(err, &ra); got != (c.wait > 0) || (got && ra.After != c.wait) {
				t.Errorf("retry after: %v, want %s", err, c.wait)
			}
		})
	}

	// Facebook refused the video outright, so trying again is safe.
	for name, body := range map[string]string{
		"video not fetched": `{"error":{"message":"Unable to fetch video file from URL.","code":389}}`,
		"upload problem":    `{"error":{"message":"There was a problem uploading your video. Please try again.","code":6001}}`,
	} {
		t.Run(name, func(t *testing.T) {
			p, s := newTest(t)
			s.failPath, s.failStatus, s.failBody = videoPath, 400, body
			post := publisher.Post{Media: []publisher.Media{{Key: "v.mp4", ContentType: "video/mp4"}}}
			_, err := p.Publish(context.Background(), creds, post, files{})
			if err == nil || publisher.IsPermanent(err) {
				t.Errorf("got %v, want a retryable error", err)
			}
		})
	}

	t.Run("a bad video file is permanent", func(t *testing.T) {
		p, s := newTest(t)
		s.failPath, s.failStatus = videoPath, 400
		s.failBody = `{"error":{"message":"There was a problem uploading your video file. Please try again with another file.","code":6000}}`
		post := publisher.Post{Media: []publisher.Media{{Key: "v.mp4", ContentType: "video/mp4"}}}
		if _, err := p.Publish(context.Background(), creds, post, files{}); !publisher.IsPermanent(err) {
			t.Errorf("got %v, want permanent", err)
		}
	})
}

// net/http writes the whole request URL into a transport error, and the
// tokens and app secret travel in URLs here. None may reach an error.
func TestErrorsCarryNoSecrets(t *testing.T) {
	p, _ := newTest(t)
	p.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}
	_, errPub := p.Publish(context.Background(), creds, publisher.Post{Text: "hi"}, files{})
	_, errEx := p.ExchangeAll(context.Background(), "code-1", "", "https://postitheka.test/cb")
	for _, err := range []error{errPub, errEx} {
		if err == nil {
			t.Fatal("no error")
		}
		for _, secret := range []string{"page-token-1", "secret-1", "code-1"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error %q contains %s", err, secret)
			}
		}
	}
}

func TestRules(t *testing.T) {
	r := New("a", "b", nil).Rules()
	jpeg := publisher.Media{ContentType: "image/jpeg", Size: 1_000_000}
	mp4 := publisher.Media{ContentType: "video/mp4", Size: 50_000_000}

	for name, c := range map[string]struct {
		post  publisher.Post
		field string // "" means accepted
	}{
		"text only":         {publisher.Post{Text: "hi"}, ""},
		"photo, no text":    {publisher.Post{Media: []publisher.Media{jpeg}}, ""},
		"ten photos":        {publisher.Post{Media: slicesOf(jpeg, 10)}, ""},
		"eleven photos":     {publisher.Post{Media: slicesOf(jpeg, 11)}, "media"},
		"one video":         {publisher.Post{Text: "hi", Media: []publisher.Media{mp4}}, ""},
		"video and a photo": {publisher.Post{Media: []publisher.Media{mp4, jpeg}}, "media"},
		"photo over 10 MB": {
			publisher.Post{Media: []publisher.Media{{ContentType: "image/jpeg", Size: 10_000_001}}}, "media",
		},
		"quicktime": {
			publisher.Post{Media: []publisher.Media{{ContentType: "video/quicktime", Size: 1000}}}, "media",
		},
		"text over the cap":  {publisher.Post{Text: strings.Repeat("a", 5001)}, "text"},
		"comment at the cap": {publisher.Post{Text: "hi", Comment: strings.Repeat("a", 5000)}, ""},
		"comment over the cap": {
			publisher.Post{Text: "hi", Comment: strings.Repeat("a", 5001)}, "first_comment",
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

func slicesOf(m publisher.Media, n int) []publisher.Media {
	out := make([]publisher.Media, n)
	for i := range out {
		out[i] = m
	}
	return out
}

const postPath = "/v27.0/" + pageID + "_7/comments"

func TestCommentGoesUnderThePost(t *testing.T) {
	p, s := newTest(t)
	res, err := p.Comment(context.Background(), creds, pageID+"_7", "#launch")
	if err != nil {
		t.Fatal(err)
	}
	sent := s.find(http.MethodPost, postPath)
	if len(sent) != 1 {
		t.Fatalf("%d comment requests", len(sent))
	}
	if f := form(t, sent[0]); f.Get("message") != "#launch" || f.Get("access_token") != "page-token-1" || strings.Contains(sent[0].Query, "access_token") {
		t.Errorf("comment form %v, query %q; want the Page token in the body only", f, sent[0].Query)
	}
	if res.RemoteID != pageID+"_7_1" || res.URL != "" || sent[0].Host != "graph.facebook.com" {
		t.Errorf("result %+v to %s", res, sent[0].Host)
	}
}

func TestCommentOutcomes(t *testing.T) {
	ctx := context.Background()
	for name, c := range map[string]struct {
		status int
		body   string
		check  func(error) bool
		want   string
	}{
		// Commenting as a person rather than the Page (Pages comments guide).
		"refused": {
			400, `{"error":{"message":"(#1705) There was an error posting to this wall","code":1705}}`,
			publisher.IsPermanent, "1705",
		},
		"dead token": {
			400, `{"error":{"message":"Error validating access token","code":190}}`,
			publisher.NeedsReconnect, "reconnect",
		},
		// No clear answer: the comment may be live, so it is not tried again.
		"server error": {
			500, `{"error":{"message":"An unknown error occurred","code":1}}`,
			publisher.IsPermanent, "check the post before commenting again",
		},
		"not Graph": {502, `<html>`, publisher.IsPermanent, "check the post before commenting again"},
	} {
		t.Run(name, func(t *testing.T) {
			p, s := newTest(t)
			s.failPath, s.failStatus, s.failBody = postPath, c.status, c.body
			_, err := p.Comment(ctx, creds, pageID+"_7", "#tags")
			if err == nil || !c.check(err) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want %q", err, c.want)
			}
		})
	}

	t.Run("rate limited is retried later", func(t *testing.T) {
		p, s := newTest(t)
		s.failPath, s.failStatus, s.failBody = postPath, 400, `{"error":{"message":"Application request limit reached","code":4}}`
		_, err := p.Comment(ctx, creds, pageID+"_7", "#tags")
		var ra *publisher.RetryAfterError
		if !errors.As(err, &ra) || publisher.IsPermanent(err) {
			t.Errorf("got %v, want retry after", err)
		}
	})

	t.Run("unreadable credentials", func(t *testing.T) {
		p, _ := newTest(t)
		if _, err := p.Comment(ctx, []byte("{}"), pageID+"_7", "#tags"); !publisher.IsPermanent(err) {
			t.Errorf("got %v, want permanent", err)
		}
	})
}
