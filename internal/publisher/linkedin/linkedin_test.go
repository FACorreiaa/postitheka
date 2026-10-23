package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// stub plays LinkedIn: the token endpoint, userinfo, the Images, Videos and
// Posts APIs, and the upload hosts. Every host is redirected to it, so a
// request to any other host shows up here too and can be caught.
type stub struct {
	mu       sync.Mutex
	requests []recorded
	// partSize is the byte range per video upload instruction.
	partSize      int64
	processing    int    // GET /rest/videos answers PROCESSING this many times first
	videoStatus   string // then this
	postStatus    int
	postHeaders   http.Header
	postBody      string
	uploadURLHost string
	images        int
}

type recorded struct {
	Method, Host, Path, Query string
	Header                    http.Header
	Body                      []byte
}

func (s *stub) find(method, path string) []recorded {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []recorded
	for _, r := range s.requests {
		if r.Method == method && strings.HasPrefix(r.Path, path) {
			out = append(out, r)
		}
	}
	return out
}

func (s *stub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, recorded{r.Method, r.Host, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), body})
	s.mu.Unlock()
	host := s.uploadURLHost
	if host == "" {
		host = "www.linkedin.com"
	}

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/oauth/v2/accessToken":
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"at-1","expires_in":5184000,"scope":"openid,profile,w_member_social"}`)
	case r.Method == http.MethodGet && r.URL.Path == "/v2/userinfo":
		if r.Header.Get("Authorization") != "Bearer at-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"sub":"m-42","name":"Ada Lovelace"}`)
	case r.Method == http.MethodPost && r.URL.Path == "/rest/images":
		s.mu.Lock()
		s.images++
		n := s.images
		s.mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"value":{"uploadUrlExpiresAt":1,"uploadUrl":"https://%s/dms-uploads/img-%d","image":"urn:li:image:I%d"}}`, host, n, n)
	case r.Method == http.MethodPost && r.URL.Path == "/rest/videos" && r.URL.Query().Get("action") == "initializeUpload":
		var req struct {
			Init struct {
				Size int64 `json:"fileSizeBytes"`
			} `json:"initializeUploadRequest"`
		}
		_ = json.Unmarshal(body, &req)
		var parts []string
		for first, i := int64(0), 0; first < req.Init.Size; first, i = first+s.partSize, i+1 {
			last := min(first+s.partSize, req.Init.Size) - 1
			parts = append(parts, fmt.Sprintf(`{"uploadUrl":"https://%s/dms-uploads/vid/part-%d","firstByte":%d,"lastByte":%d}`, host, i, first, last))
		}
		_, _ = fmt.Fprintf(w, `{"value":{"video":"urn:li:video:V1","uploadToken":"","uploadInstructions":[%s]}}`, strings.Join(parts, ","))
	case r.Method == http.MethodPost && r.URL.Path == "/rest/videos":
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/rest/videos/"):
		s.mu.Lock()
		status := s.videoStatus
		if s.processing > 0 {
			s.processing--
			status = "PROCESSING"
		}
		s.mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"id":"urn:li:video:V1","status":%q,"processingFailureReason":"unsupported codec"}`, status)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/dms-uploads/"):
		w.Header().Set("ETag", "etag-"+strings.TrimPrefix(r.URL.Path, "/dms-uploads/"))
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodPost && r.URL.Path == "/rest/posts":
		for k, v := range s.postHeaders {
			w.Header()[k] = v
		}
		if s.postStatus != 0 && s.postStatus != http.StatusCreated {
			w.WriteHeader(s.postStatus)
			_, _ = io.WriteString(w, s.postBody)
			return
		}
		w.Header().Set("x-restli-id", "urn:li:share:7001")
		w.WriteHeader(http.StatusCreated)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newTest(t *testing.T) (*Publisher, *stub) {
	t.Helper()
	s := &stub{partSize: 4, videoStatus: "AVAILABLE"}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.Host = r.URL.Host
		r.URL.Scheme, r.URL.Host = target.Scheme, target.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
	p := New("client-1", "secret-1", client)
	p.pollEvery = time.Millisecond
	return p, s
}

type files map[string][]byte

func (f files) Open(_ context.Context, key string) (io.ReadCloser, error) {
	b, ok := f[key]
	if !ok {
		return nil, errors.New("no such object")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (files) PublicURL(context.Context, string, time.Duration) (string, error) {
	return "", errors.New("linkedin never needs a public URL")
}

func creds(t *testing.T, expires time.Time) []byte {
	t.Helper()
	raw, _ := json.Marshal(credentials{AccessToken: "at-1", ExpiresAt: expires, Person: "urn:li:person:m-42"})
	return raw
}

var future = time.Now().Add(30 * 24 * time.Hour)

func TestConnect(t *testing.T) {
	p, s := newTest(t)
	const redirect = "https://postitheka.test/oauth/linkedin/callback"

	auth, _ := url.Parse(p.AuthorizeURL("st-1", "verifier-1", redirect))
	q := auth.Query()
	if auth.Host != "www.linkedin.com" || auth.Path != "/oauth/v2/authorization" {
		t.Errorf("authorize URL %s", auth)
	}
	for k, want := range map[string]string{"client_id": "client-1", "redirect_uri": redirect, "state": "st-1", "scope": "openid profile w_member_social", "response_type": "code"} {
		if q.Get(k) != want {
			t.Errorf("authorize %s = %q, want %q", k, q.Get(k), want)
		}
	}
	if q.Has("code_challenge") {
		t.Error("sent a PKCE challenge LinkedIn does not accept from server apps")
	}

	conn, err := p.Exchange(context.Background(), "code-1", "verifier-1", redirect)
	if err != nil {
		t.Fatal(err)
	}
	tok := s.find(http.MethodPost, "/oauth/v2/accessToken")
	if len(tok) != 1 {
		t.Fatalf("%d token requests", len(tok))
	}
	form, _ := url.ParseQuery(string(tok[0].Body))
	for k, want := range map[string]string{"grant_type": "authorization_code", "code": "code-1", "redirect_uri": redirect, "client_id": "client-1", "client_secret": "secret-1"} {
		if form.Get(k) != want {
			t.Errorf("token %s = %q, want %q", k, form.Get(k), want)
		}
	}
	if conn.DisplayName != "Ada Lovelace" || conn.RemoteID != "m-42" {
		t.Errorf("connection %q / %q", conn.DisplayName, conn.RemoteID)
	}
	var c credentials
	_ = json.Unmarshal(conn.Credentials, &c)
	if c.AccessToken != "at-1" || c.Person != "urn:li:person:m-42" || time.Until(c.ExpiresAt) < 59*24*time.Hour {
		t.Errorf("credentials %+v", c)
	}
}

func TestPublishText(t *testing.T) {
	p, s := newTest(t)
	res, err := p.Publish(context.Background(), creds(t, future), publisher.Post{Text: "Shipping (finally) #golang"}, files{})
	if err != nil {
		t.Fatal(err)
	}
	if res.RemoteID != "urn:li:share:7001" || res.URL != "https://www.linkedin.com/feed/update/urn:li:share:7001/" {
		t.Errorf("result %+v", res)
	}
	posts := s.find(http.MethodPost, "/rest/posts")
	if len(posts) != 1 {
		t.Fatalf("%d posts", len(posts))
	}
	h := posts[0].Header
	if h.Get("Linkedin-Version") != version || h.Get("X-Restli-Protocol-Version") != "2.0.0" || h.Get("Authorization") != "Bearer at-1" {
		t.Errorf("headers %v", h)
	}
	var body map[string]any
	_ = json.Unmarshal(posts[0].Body, &body)
	if body["author"] != "urn:li:person:m-42" || body["commentary"] != `Shipping \(finally\) #golang` || body["lifecycleState"] != "PUBLISHED" {
		t.Errorf("body %s", posts[0].Body)
	}
	if _, ok := body["content"]; ok {
		t.Error("a text post carries content")
	}
}

func TestEscapeText(t *testing.T) {
	for in, want := range map[string]string{
		"plain words":         "plain words",
		"a (b) [c] {d} <e>":   `a \(b\) \[c\] \{d\} \<e\>`,
		"me@example.com":      `me\@example.com`,
		"#tag and # alone":    `#tag and \# alone`,
		"#1 and #éclair":      "#1 and #éclair",
		`back\slash | pipe`:   `back\\slash \| pipe`,
		"*bold* _it_ ~strike": `\*bold\* \_it\_ \~strike`,
		"ending #":            `ending \#`,
	} {
		if got := escapeText(in); got != want {
			t.Errorf("escapeText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPublishImages(t *testing.T) {
	src := files{"a": []byte("png-a"), "b": []byte("png-b"), "c": []byte("png-c")}
	img := func(key string) publisher.Media {
		return publisher.Media{Key: key, ContentType: "image/png", Size: int64(len(src[key]))}
	}

	t.Run("one image", func(t *testing.T) {
		p, s := newTest(t)
		if _, err := p.Publish(context.Background(), creds(t, future), publisher.Post{Text: "x", Media: []publisher.Media{img("a")}}, src); err != nil {
			t.Fatal(err)
		}
		init := s.find(http.MethodPost, "/rest/images")
		if len(init) != 1 || !strings.Contains(string(init[0].Body), `"owner":"urn:li:person:m-42"`) || init[0].Query != "action=initializeUpload" {
			t.Fatalf("initialize %+v", init)
		}
		puts := s.find(http.MethodPut, "/dms-uploads/")
		if len(puts) != 1 || string(puts[0].Body) != "png-a" || puts[0].Header.Get("Authorization") != "Bearer at-1" || puts[0].Host != "www.linkedin.com" {
			t.Fatalf("upload %+v", puts)
		}
		var body struct {
			Content struct {
				Media struct{ ID string } `json:"media"`
			} `json:"content"`
		}
		_ = json.Unmarshal(s.find(http.MethodPost, "/rest/posts")[0].Body, &body)
		if body.Content.Media.ID != "urn:li:image:I1" {
			t.Errorf("content %+v", body.Content)
		}
	})

	t.Run("several images", func(t *testing.T) {
		p, s := newTest(t)
		if _, err := p.Publish(context.Background(), creds(t, future), publisher.Post{Media: []publisher.Media{img("a"), img("b"), img("c")}}, src); err != nil {
			t.Fatal(err)
		}
		var body struct {
			Content struct {
				MultiImage struct {
					Images []struct{ ID string } `json:"images"`
				} `json:"multiImage"`
			} `json:"content"`
		}
		_ = json.Unmarshal(s.find(http.MethodPost, "/rest/posts")[0].Body, &body)
		var ids []string
		for _, i := range body.Content.MultiImage.Images {
			ids = append(ids, i.ID)
		}
		if strings.Join(ids, ",") != "urn:li:image:I1,urn:li:image:I2,urn:li:image:I3" {
			t.Errorf("images in order %v", ids)
		}
	})

	t.Run("an upload URL off LinkedIn", func(t *testing.T) {
		p, s := newTest(t)
		s.uploadURLHost = "uploads.example.com"
		_, err := p.Publish(context.Background(), creds(t, future), publisher.Post{Media: []publisher.Media{img("a")}}, src)
		if !publisher.IsPermanent(err) {
			t.Fatalf("got %v, want a permanent refusal", err)
		}
		if puts := s.find(http.MethodPut, "/"); len(puts) != 0 {
			t.Errorf("sent %d uploads to a foreign host", len(puts))
		}
	})
}

func TestPublishVideo(t *testing.T) {
	data := []byte("0123456789abcdefghij") // 20 bytes: parts of 4, the last full
	data = append(data, 'z')               // 21: the last part is 1 byte
	src := files{"v": data}
	video := publisher.Media{Key: "v", ContentType: "video/mp4", Size: int64(len(data))}

	p, s := newTest(t)
	s.processing = 2
	if _, err := p.Publish(context.Background(), creds(t, future), publisher.Post{Text: "clip", Media: []publisher.Media{video}}, src); err != nil {
		t.Fatal(err)
	}

	init := s.find(http.MethodPost, "/rest/videos")[0]
	if !strings.Contains(string(init.Body), `"fileSizeBytes":21`) {
		t.Errorf("initialize %s", init.Body)
	}
	puts := s.find(http.MethodPut, "/dms-uploads/vid/")
	var got []byte
	for i, put := range puts {
		if put.Path != "/dms-uploads/vid/part-"+strconv.Itoa(i) {
			t.Errorf("part %d went to %s", i, put.Path)
		}
		if put.Header.Get("Authorization") != "" {
			t.Error("the member's token was sent with a video part")
		}
		got = append(got, put.Body...)
	}
	if len(puts) != 6 || !bytes.Equal(got, data) {
		t.Fatalf("%d parts carrying %q, want 6 carrying the file", len(puts), got)
	}

	var fin struct {
		F struct {
			Video string   `json:"video"`
			Parts []string `json:"uploadedPartIds"`
		} `json:"finalizeUploadRequest"`
	}
	finalize := s.find(http.MethodPost, "/rest/videos")[1]
	_ = json.Unmarshal(finalize.Body, &fin)
	if finalize.Query != "action=finalizeUpload" || fin.F.Video != "urn:li:video:V1" || len(fin.F.Parts) != 6 || fin.F.Parts[5] != "etag-vid/part-5" {
		t.Errorf("finalize %s", finalize.Body)
	}
	if n := len(s.find(http.MethodGet, "/rest/videos/")); n != 3 {
		t.Errorf("%d status checks, want 3 (two while processing)", n)
	}
	if !strings.Contains(string(s.find(http.MethodPost, "/rest/posts")[0].Body), `"id":"urn:li:video:V1"`) {
		t.Error("post does not carry the video")
	}

	t.Run("processing fails", func(t *testing.T) {
		p, s := newTest(t)
		s.videoStatus = "PROCESSING_FAILED"
		_, err := p.Publish(context.Background(), creds(t, future), publisher.Post{Media: []publisher.Media{video}}, src)
		if !publisher.IsPermanent(err) || !strings.Contains(err.Error(), "unsupported codec") {
			t.Fatalf("got %v, want permanent with LinkedIn's reason", err)
		}
		if len(s.find(http.MethodPost, "/rest/posts")) != 0 {
			t.Error("posted a video that failed processing")
		}
	})

	t.Run("still processing when time runs out", func(t *testing.T) {
		p, s := newTest(t)
		s.processing = 1 << 30
		p.processingWait = 5 * time.Millisecond
		_, err := p.Publish(context.Background(), creds(t, future), publisher.Post{Media: []publisher.Media{video}}, src)
		if err == nil || publisher.IsPermanent(err) {
			t.Fatalf("got %v, want a retryable error", err)
		}
		if len(s.find(http.MethodPost, "/rest/posts")) != 0 {
			t.Error("posted a video still processing")
		}
	})
}

func TestPublishErrorClassification(t *testing.T) {
	cases := map[string]struct {
		status    int
		header    http.Header
		body      string
		permanent bool
		after     time.Duration
	}{
		"revoked":          {status: 401, body: `{"message":"Invalid access token","status":401}`, permanent: true},
		"rejected":         {status: 422, body: `{"message":"FIELD_LENGTH_TOO_LONG"}`, permanent: true},
		"version sunset":   {status: 426, permanent: true},
		"server error":     {status: 503},
		"media not ready":  {status: 400, body: `{"message":"MEDIA_ASSET_WAITING_UPLOAD"}`},
		"rate limited":     {status: 429, header: http.Header{"Retry-After": {"30"}}, after: 30 * time.Second},
		"daily limit, UTC": {status: 429, after: -1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p, s := newTest(t)
			s.postStatus, s.postHeaders, s.postBody = tc.status, tc.header, tc.body
			_, err := p.Publish(context.Background(), creds(t, future), publisher.Post{Text: "x"}, files{})
			if err == nil {
				t.Fatal("no error")
			}
			if publisher.IsPermanent(err) != tc.permanent {
				t.Errorf("permanent = %v for %v", !tc.permanent, err)
			}
			var ra *publisher.RetryAfterError
			switch {
			case tc.after > 0:
				if !errors.As(err, &ra) || ra.After != tc.after {
					t.Errorf("got %v, want retry after %s", err, tc.after)
				}
			case tc.after < 0:
				// No header: wait for the daily reset at midnight UTC.
				if !errors.As(err, &ra) || ra.After <= 0 || ra.After > 24*time.Hour+time.Minute {
					t.Errorf("got %v, want a wait until midnight UTC", err)
				}
			}
		})
	}
}

func TestPublishRefusesBadCredentials(t *testing.T) {
	p, s := newTest(t)
	_, err := p.Publish(context.Background(), creds(t, time.Now().Add(-time.Hour)), publisher.Post{Text: "x"}, files{})
	if !publisher.IsPermanent(err) || !errors.Is(err, ErrExpired) {
		t.Errorf("expired token: %v", err)
	}
	if _, err := p.Publish(context.Background(), []byte("not json"), publisher.Post{Text: "x"}, files{}); !publisher.IsPermanent(err) {
		t.Errorf("unreadable credentials: %v", err)
	}
	if len(s.requests) != 0 {
		t.Errorf("%d requests sent with unusable credentials", len(s.requests))
	}
}
