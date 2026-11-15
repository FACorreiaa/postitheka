package tiktok

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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FACorreiaa/postitheka/internal/publisher"
)

const uploadPath = "/video/"

// stub plays TikTok: the token endpoint, the account lookup, the start of an
// inbox upload, the upload address and the status check. Every host is
// redirected to it, so a request to any other host shows up here too.
type stub struct {
	mu       sync.Mutex
	requests []recorded
	received []byte // what the upload address was sent, in order

	tokenStatus int    // 0 means 200
	tokenBody   string // "" means a fresh pair of tokens
	failStep    string // a path suffix that answers failStatus
	failStatus  int
	failBody    string
	failPart    int      // a 1-based chunk that answers failStatus; 0 means none
	statuses    []string // answers to the status check, in order; the last repeats
	failReason  string
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
		w.WriteHeader(s.failStatus)
		_, _ = io.WriteString(w, s.failBody)
		return
	}

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v2/oauth/token/":
		if s.tokenStatus != 0 {
			w.WriteHeader(s.tokenStatus)
		}
		if s.tokenBody != "" {
			_, _ = io.WriteString(w, s.tokenBody)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"act.new","expires_in":7200,"open_id":"open-1","refresh_token":"rft.new","refresh_expires_in":15552000,"scope":"user.info.basic,video.upload","token_type":"Bearer"}`)
	case r.Method == http.MethodGet && r.URL.Path == "/v2/user/info/":
		_, _ = io.WriteString(w, `{"data":{"user":{"open_id":"open-1","display_name":"Postitheka"}},"error":{"code":"ok","message":"","log_id":"1"}}`)
	case r.Method == http.MethodPost && r.URL.Path == "/v2/post/publish/inbox/video/init/":
		_, _ = io.WriteString(w, `{"data":{"publish_id":"v_inbox_file~v2.1","upload_url":"https://open-upload.tiktokapis.com`+uploadPath+`?upload_id=67&upload_token=sig-secret"},"error":{"code":"ok","message":"","log_id":"2"}}`)
	case r.Method == http.MethodPut && r.URL.Path == uploadPath:
		part := 0
		for _, q := range s.requests {
			if q.Method == http.MethodPut {
				part++
			}
		}
		if s.failPart == part {
			w.WriteHeader(s.failStatus)
			return
		}
		s.received = append(s.received, body...)
		var first, last, total int64
		_, _ = fmt.Sscanf(r.Header.Get("Content-Range"), "bytes %d-%d/%d", &first, &last, &total)
		if last+1 == total {
			w.WriteHeader(http.StatusCreated)
		} else {
			w.WriteHeader(http.StatusPartialContent)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/v2/post/publish/status/fetch/":
		status := "SEND_TO_USER_INBOX"
		if len(s.statuses) > 0 {
			status = s.statuses[0]
			if len(s.statuses) > 1 {
				s.statuses = s.statuses[1:]
			}
		}
		_, _ = fmt.Fprintf(w, `{"data":{"status":%q,"fail_reason":%q},"error":{"code":"ok","message":"","log_id":"3"}}`, status, s.failReason)
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
	p := New("client-key", "client-secret", client)
	p.pollEvery, p.settleLimit = time.Millisecond, 50*time.Millisecond
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
	return "", errors.New("tiktok uploads bytes; it must not ask for a URL")
}

func testCreds(t *testing.T) []byte {
	t.Helper()
	raw, _ := json.Marshal(credentials{
		AccessToken: "act.1", RefreshToken: "rft.1", ExpiresAt: time.Now().Add(2 * time.Hour), OpenID: "open-1",
	})
	return raw
}

// video returns a post with one video of size bytes, and the bytes.
func video(size int) (publisher.Post, memMedia) {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	post := publisher.Post{
		Text:  "written for the other platforms",
		Media: []publisher.Media{{Key: "v", ContentType: "video/mp4", Size: int64(size), Filename: "v.mp4"}},
	}
	return post, memMedia{"v": data}
}

func TestAuthorizeURL(t *testing.T) {
	p := newPublisher(t, &stub{})
	u, err := url.Parse(p.AuthorizeURL("state-1", "verifier-1", "https://postitheka.test/oauth/tiktok/callback"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "www.tiktok.com" || u.Path != "/v2/auth/authorize/" {
		t.Errorf("authorize URL is %s%s", u.Host, u.Path)
	}
	if q.Get("client_key") != "client-key" || q.Get("scope") != "user.info.basic,video.upload" || q.Get("response_type") != "code" ||
		q.Get("state") != "state-1" || q.Get("redirect_uri") != "https://postitheka.test/oauth/tiktok/callback" {
		t.Errorf("query %v", q)
	}
	if strings.Contains(u.String(), "client-secret") || strings.Contains(u.String(), "verifier-1") {
		t.Error("the client secret or the verifier is in the authorize URL")
	}
}

func TestExchangeConnectsTheAccount(t *testing.T) {
	s := &stub{}
	p := newPublisher(t, s)

	conn, err := p.Exchange(context.Background(), "code-1", "verifier-1", "https://postitheka.test/oauth/tiktok/callback")
	if err != nil {
		t.Fatal(err)
	}
	if conn.DisplayName != "Postitheka" || conn.RemoteID != "open-1" {
		t.Errorf("connection %q / %q", conn.DisplayName, conn.RemoteID)
	}
	var creds credentials
	if err := json.Unmarshal(conn.Credentials, &creds); err != nil {
		t.Fatal(err)
	}
	if creds.AccessToken != "act.new" || creds.RefreshToken != "rft.new" || creds.OpenID != "open-1" {
		t.Errorf("credentials %+v", creds)
	}
	// What the answer said, not what the page says.
	if left := time.Until(creds.ExpiresAt); left < 119*time.Minute || left > 121*time.Minute {
		t.Errorf("expires in %s, want two hours", left)
	}
	if left := time.Until(creds.RefreshExpiresAt); left < 179*24*time.Hour || left > 181*24*time.Hour {
		t.Errorf("refresh token expires in %s, want 180 days", left)
	}

	tok := s.find(http.MethodPost, "/v2/oauth/token/")
	if len(tok) != 1 {
		t.Fatalf("%d token requests", len(tok))
	}
	form, _ := url.ParseQuery(string(tok[0].Body))
	if form.Get("client_key") != "client-key" || form.Get("client_secret") != "client-secret" || form.Get("code") != "code-1" ||
		form.Get("grant_type") != "authorization_code" || form.Get("redirect_uri") != "https://postitheka.test/oauth/tiktok/callback" {
		t.Errorf("token form %v", form)
	}
	if tok[0].Host != "open.tiktokapis.com" || tok[0].Query != "" {
		t.Errorf("token request went to %q?%q", tok[0].Host, tok[0].Query)
	}
	me := s.find(http.MethodGet, "/v2/user/info/")
	if len(me) != 1 || me[0].Header.Get("Authorization") != "Bearer act.new" || me[0].Query != "fields=open_id,display_name" {
		t.Errorf("account lookup %+v", me)
	}
}

func TestExchangeFailureCarriesNoSecrets(t *testing.T) {
	// TikTok can answer 200 with the error in the body.
	s := &stub{tokenBody: `{"error":"invalid_grant","error_description":"Authorization code is expired.","log_id":"9"}`}
	p := newPublisher(t, s)
	_, err := p.Exchange(context.Background(), "code-1", "", "https://postitheka.test/oauth/tiktok/callback")
	if err == nil {
		t.Fatal("an error in a 200 answer was taken as success")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error %q does not say what TikTok said", err)
	}
	for _, secret := range []string{"client-secret", "code-1"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error %q carries %q", err, secret)
		}
	}
}

func TestNeedsRefreshLeavesRoomForALongPublish(t *testing.T) {
	p := newPublisher(t, &stub{})
	now := time.Now()
	creds := func(left time.Duration, refresh string) []byte {
		raw, _ := json.Marshal(credentials{AccessToken: "a", RefreshToken: refresh, ExpiresAt: now.Add(left), OpenID: "o"})
		return raw
	}
	if p.NeedsRefresh(creds(time.Hour, "r"), now) {
		t.Error("a token with an hour left was refreshed")
	}
	if !p.NeedsRefresh(creds(35*time.Minute, "r"), now) {
		t.Error("a token that dies during a 30-minute publish was not refreshed")
	}
	if p.NeedsRefresh(creds(-time.Minute, ""), now) {
		t.Error("credentials with no refresh token were sent for a refresh")
	}
	if p.NeedsRefresh([]byte("{"), now) {
		t.Error("unreadable credentials were sent for a refresh")
	}
}

func TestRefreshKeepsTheNewRefreshToken(t *testing.T) {
	s := &stub{}
	p := newPublisher(t, s)
	raw, err := p.Refresh(context.Background(), testCreds(t))
	if err != nil {
		t.Fatal(err)
	}
	var creds credentials
	_ = json.Unmarshal(raw, &creds)
	if creds.AccessToken != "act.new" || creds.RefreshToken != "rft.new" || creds.OpenID != "open-1" {
		t.Errorf("credentials %+v", creds)
	}
	form, _ := url.ParseQuery(string(s.find(http.MethodPost, "/v2/oauth/token/")[0].Body))
	if form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != "rft.1" || form.Get("client_key") != "client-key" {
		t.Errorf("token form %v", form)
	}
}

func TestRefreshWithoutANewRefreshTokenKeepsTheOldOne(t *testing.T) {
	s := &stub{tokenBody: `{"access_token":"act.new","expires_in":7200,"open_id":"open-1"}`}
	p := newPublisher(t, s)
	raw, err := p.Refresh(context.Background(), testCreds(t))
	if err != nil {
		t.Fatal(err)
	}
	var creds credentials
	_ = json.Unmarshal(raw, &creds)
	if creds.RefreshToken != "rft.1" {
		t.Errorf("refresh token %q, want the one already held", creds.RefreshToken)
	}
}

func TestRefreshOfARevokedGrantIsPermanent(t *testing.T) {
	for _, s := range []*stub{
		{tokenStatus: http.StatusBadRequest, tokenBody: `{"error":"invalid_grant","error_description":"Refresh token is invalid or expired."}`},
		{tokenBody: `{"error":"invalid_grant","error_description":"Refresh token is invalid or expired."}`},
	} {
		p := newPublisher(t, s)
		_, err := p.Refresh(context.Background(), testCreds(t))
		if !publisher.IsPermanent(err) {
			t.Errorf("status %d: %v is not permanent", s.tokenStatus, err)
		}
		if err != nil && strings.Contains(err.Error(), "rft.1") {
			t.Errorf("error %q carries the refresh token", err)
		}
	}
}

func TestRefreshThatMayPassIsRetried(t *testing.T) {
	s := &stub{tokenStatus: http.StatusBadGateway, tokenBody: `oops`}
	p := newPublisher(t, s)
	_, err := p.Refresh(context.Background(), testCreds(t))
	if err == nil || publisher.IsPermanent(err) {
		t.Errorf("a 502 from the token endpoint gave %v, want an error that is retried", err)
	}
}

func TestASmallVideoGoesUpWholeAndIsADraft(t *testing.T) {
	s := &stub{}
	p := newPublisher(t, s)
	post, media := video(3 << 20)

	res, err := p.Publish(context.Background(), testCreds(t), post, media)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Draft || res.RemoteID != "v_inbox_file~v2.1" || res.URL != "" {
		t.Errorf("result %+v, want a draft with TikTok's id and no link", res)
	}

	start := s.find(http.MethodPost, "/inbox/video/init/")
	if len(start) != 1 || start[0].Host != "open.tiktokapis.com" || start[0].Header.Get("Authorization") != "Bearer act.1" {
		t.Fatalf("start requests %+v", start)
	}
	var body struct {
		Source struct {
			Source string `json:"source"`
			Size   int64  `json:"video_size"`
			Chunk  int64  `json:"chunk_size"`
			Count  int64  `json:"total_chunk_count"`
		} `json:"source_info"`
	}
	_ = json.Unmarshal(start[0].Body, &body)
	if got := body.Source; got.Source != "FILE_UPLOAD" || got.Size != 3<<20 || got.Chunk != 3<<20 || got.Count != 1 {
		t.Errorf("source_info %+v", got)
	}
	// The post's text is for the other platforms: an inbox upload has no caption.
	if strings.Contains(string(start[0].Body), "written for") {
		t.Error("the post's text was sent to TikTok")
	}

	puts := s.find(http.MethodPut, uploadPath)
	if len(puts) != 1 {
		t.Fatalf("%d uploads, want 1", len(puts))
	}
	if got := puts[0]; got.Host != "open-upload.tiktokapis.com" || got.Query != "upload_id=67&upload_token=sig-secret" ||
		got.Header.Get("Content-Range") != fmt.Sprintf("bytes 0-%d/%d", 3<<20-1, 3<<20) || got.Header.Get("Content-Type") != "video/mp4" {
		t.Errorf("upload to %s?%s, range %q, type %q", got.Host, got.Query, got.Header.Get("Content-Range"), got.Header.Get("Content-Type"))
	}
	// The address is the credential. The account's token stays with TikTok's API host.
	if puts[0].Header.Get("Authorization") != "" {
		t.Error("the access token was sent to the upload address")
	}
	if !bytes.Equal(s.received, media["v"]) {
		t.Error("TikTok did not receive the file as it was")
	}
	if len(s.find(http.MethodPost, "/status/fetch/")) != 1 {
		t.Error("the upload was not checked once it was sent")
	}
}

func TestALargeVideoGoesUpInChunksWithTheRestInTheLast(t *testing.T) {
	s := &stub{statuses: []string{"PROCESSING_UPLOAD", "PROCESSING_UPLOAD", "SEND_TO_USER_INBOX"}}
	p := newPublisher(t, s)
	size := 2*chunkSize + 3<<20 // two chunks, and 3 MiB too few for a third
	post, media := video(size)

	res, err := p.Publish(context.Background(), testCreds(t), post, media)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Draft {
		t.Errorf("result %+v, want a draft", res)
	}
	start := s.find(http.MethodPost, "/inbox/video/init/")[0]
	if !strings.Contains(string(start.Body), fmt.Sprintf(`"chunk_size":%d`, chunkSize)) || !strings.Contains(string(start.Body), `"total_chunk_count":2`) {
		t.Errorf("start body %s", start.Body)
	}
	puts := s.find(http.MethodPut, uploadPath)
	want := []string{
		fmt.Sprintf("bytes 0-%d/%d", chunkSize-1, size),
		fmt.Sprintf("bytes %d-%d/%d", chunkSize, size-1, size),
	}
	if len(puts) != len(want) {
		t.Fatalf("%d uploads, want %d", len(puts), len(want))
	}
	for i, r := range puts {
		if r.Header.Get("Content-Range") != want[i] {
			t.Errorf("part %d: range %q, want %q", i+1, r.Header.Get("Content-Range"), want[i])
		}
	}
	if !bytes.Equal(s.received, media["v"]) {
		t.Error("TikTok did not receive the file as it was")
	}
	if n := len(s.find(http.MethodPost, "/status/fetch/")); n != 3 {
		t.Errorf("%d status checks, want 3: it waits while TikTok processes", n)
	}
}

func TestAVideoTikTokRefusesFailsForGood(t *testing.T) {
	s := &stub{statuses: []string{"FAILED"}, failReason: "frame_rate_check_failed"}
	p := newPublisher(t, s)
	post, media := video(1 << 20)
	_, err := p.Publish(context.Background(), testCreds(t), post, media)
	if !publisher.IsPermanent(err) || !strings.Contains(err.Error(), "frame_rate_check_failed") {
		t.Errorf("got %v, want a permanent error naming TikTok's reason", err)
	}
}

// Once every byte is with TikTok, a retry would put a second video in the
// inbox. So a status that cannot be read, or is still processing when the
// wait ends, is a draft.
func TestAnUploadThatCannotBeCheckedIsStillADraft(t *testing.T) {
	for name, s := range map[string]*stub{
		"status check fails":   {failStep: "/status/fetch/", failStatus: http.StatusInternalServerError},
		"still processing":     {statuses: []string{"PROCESSING_UPLOAD"}},
		"status check refused": {failStep: "/status/fetch/", failStatus: http.StatusTooManyRequests, failBody: `{"error":{"code":"rate_limit_exceeded"}}`},
	} {
		p := newPublisher(t, s)
		post, media := video(1 << 20)
		res, err := p.Publish(context.Background(), testCreds(t), post, media)
		if err != nil || !res.Draft {
			t.Errorf("%s: got %+v, %v; want a draft", name, res, err)
		}
	}
}

func TestALostAnswerToTheLastChunkIsNotRetried(t *testing.T) {
	s := &stub{failPart: 2, failStatus: http.StatusBadGateway}
	p := newPublisher(t, s)
	post, media := video(2*chunkSize + 1)
	_, err := p.Publish(context.Background(), testCreds(t), post, media)
	if !publisher.IsPermanent(err) || !strings.Contains(err.Error(), "check the TikTok inbox") {
		t.Errorf("got %v, want a permanent error that says to check the inbox", err)
	}
}

func TestAnEarlierChunkThatFailsIsRetried(t *testing.T) {
	for _, status := range []int{http.StatusBadGateway, http.StatusForbidden, http.StatusRequestedRangeNotSatisfiable} {
		s := &stub{failPart: 1, failStatus: status}
		p := newPublisher(t, s)
		post, media := video(2*chunkSize + 1)
		_, err := p.Publish(context.Background(), testCreds(t), post, media)
		if err == nil || publisher.IsPermanent(err) {
			t.Errorf("%d on the first chunk gave %v, want an error that is retried", status, err)
		}
		if n := len(s.find(http.MethodPut, uploadPath)); n != 1 {
			t.Errorf("%d: %d uploads, want it to stop at the failed chunk", status, n)
		}
	}
}

func TestErrorClasses(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"token revoked", 401, `{"error":{"code":"access_token_invalid","message":"The access token is invalid or not found in the request."}}`, publisher.NeedsReconnect},
		{"scope removed", 401, `{"error":{"code":"scope_not_authorized","message":"no"}}`, publisher.NeedsReconnect},
		{"rate limit", 429, `{"error":{"code":"rate_limit_exceeded","message":"slow down"}}`, func(err error) bool {
			var ra *publisher.RetryAfterError
			return errors.As(err, &ra) && ra.After == time.Minute
		}},
		{"too many drafts", 403, `{"error":{"code":"spam_risk_too_many_pending_share","message":"x"}}`, func(err error) bool {
			return publisher.IsPermanent(err) && !publisher.NeedsReconnect(err) && strings.Contains(err.Error(), "5 unfinished drafts")
		}},
		{"banned", 403, `{"error":{"code":"spam_risk_user_banned_from_posting","message":"x"}}`, func(err error) bool {
			return publisher.IsPermanent(err) && !publisher.NeedsReconnect(err)
		}},
		{"bad request", 400, `{"error":{"code":"invalid_param","message":"x"}}`, publisher.IsPermanent},
		{"error in a 200", 200, `{"data":{},"error":{"code":"invalid_param","message":"x"}}`, publisher.IsPermanent},
		{"server error", 503, `oops`, func(err error) bool {
			var ra *publisher.RetryAfterError
			return err != nil && !publisher.IsPermanent(err) && !errors.As(err, &ra)
		}},
	}
	for _, c := range cases {
		s := &stub{failStep: "/inbox/video/init/", failStatus: c.status, failBody: c.body}
		p := newPublisher(t, s)
		post, media := video(1 << 20)
		_, err := p.Publish(context.Background(), testCreds(t), post, media)
		if err == nil || !c.check(err) {
			t.Errorf("%s: %v is not classified as expected", c.name, err)
		}
		if len(s.find(http.MethodPut, uploadPath)) != 0 {
			t.Errorf("%s: bytes were sent after TikTok refused the upload", c.name)
		}
	}
}

func TestErrorsCarryNoSecrets(t *testing.T) {
	// A transport failure on the upload: net/http puts the whole address,
	// signature included, in its error.
	p := New("client-key", "client-secret", &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPut {
			return nil, errors.New("connection reset")
		}
		rec := httptest.NewRecorder()
		_, _ = io.WriteString(rec, `{"data":{"publish_id":"p","upload_url":"https://open-upload.tiktokapis.com/video/?upload_token=sig-secret"},"error":{"code":"ok"}}`)
		return rec.Result(), nil
	})})
	post, media := video(2*chunkSize + 1)
	_, err := p.Publish(context.Background(), testCreds(t), post, media)
	if err == nil {
		t.Fatal("a failed upload was taken as success")
	}
	for _, secret := range []string{"sig-secret", "act.1", "rft.1", "client-secret"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error %q carries %q", err, secret)
		}
	}
}

func TestPublishRefusesWhatItCannotSend(t *testing.T) {
	p := newPublisher(t, &stub{})
	post, media := video(1 << 20)

	if _, err := p.Publish(context.Background(), []byte("{"), post, media); !publisher.IsPermanent(err) {
		t.Errorf("unreadable credentials: %v", err)
	}
	expired, _ := json.Marshal(credentials{AccessToken: "a", OpenID: "o", ExpiresAt: time.Now().Add(-time.Minute)})
	if _, err := p.Publish(context.Background(), expired, post, media); !publisher.NeedsReconnect(err) {
		t.Errorf("expired credentials: %v", err)
	}
	if _, err := p.Publish(context.Background(), testCreds(t), publisher.Post{Text: "words only"}, media); !publisher.IsPermanent(err) {
		t.Errorf("a post with no video: %v", err)
	}
}

func TestAnUploadAddressThatIsNotHTTPSIsRefused(t *testing.T) {
	s := &stub{failStep: "/inbox/video/init/", failStatus: 200, failBody: `{"data":{"publish_id":"p","upload_url":"http://elsewhere.example/video/"},"error":{"code":"ok"}}`}
	p := newPublisher(t, s)
	post, media := video(1 << 20)
	if _, err := p.Publish(context.Background(), testCreds(t), post, media); !publisher.IsPermanent(err) {
		t.Errorf("got %v, want a refusal", err)
	}
	if len(s.find(http.MethodPut, "/video/")) != 0 {
		t.Error("the video was sent over plain http")
	}
}

func TestRules(t *testing.T) {
	rules := New("k", "s", nil).Rules()
	mp4 := publisher.Media{ContentType: "video/mp4", Size: 10 << 20, Duration: time.Minute, Width: 1080, Height: 1920}
	ok := func(post publisher.Post) bool { return len(publisher.Validate(rules, post)) == 0 }

	if !ok(publisher.Post{Text: "a caption for elsewhere", Media: []publisher.Media{mp4}}) {
		t.Error("a vertical video with text was refused")
	}
	if ok(publisher.Post{Text: "words only"}) {
		t.Error("a post with no video was accepted")
	}
	if ok(publisher.Post{Media: []publisher.Media{{ContentType: "image/jpeg", Size: 1 << 20}}}) {
		t.Error("a photo was accepted: TikTok only pulls photos from a verified domain")
	}
	if ok(publisher.Post{Media: []publisher.Media{mp4, mp4}}) {
		t.Error("two videos were accepted")
	}
	long := mp4
	long.Duration = 11 * time.Minute
	if ok(publisher.Post{Media: []publisher.Media{long}}) {
		t.Error("an 11-minute video was accepted")
	}
	wide := mp4
	wide.Width = 5000
	if ok(publisher.Post{Media: []publisher.Media{wide}}) {
		t.Error("a video 5000 pixels wide was accepted")
	}
	for _, typ := range []string{"video/quicktime", "video/webm"} {
		m := mp4
		m.ContentType = typ
		if !ok(publisher.Post{Media: []publisher.Media{m}}) {
			t.Errorf("%s was refused", typ)
		}
	}
}
