// Package tiktok puts a video in a TikTok account's inbox, as a draft.
//
// This is TikTok's "upload" flow (scope video.upload), not direct posting:
// the video arrives in the person's TikTok app as a notification, and they
// write the caption, choose who sees it and post it there. Nothing is public
// until they do, so the outcome is a draft (publisher.Result.Draft), not a
// published post. Direct posting needs TikTok's audit (TT-002).
//
// Access is OAuth 2.0 for a web app: no PKCE, and the client key and secret
// travel in the form body of the token request, under TikTok's own name
// client_key. An access token lasts hours and a refresh token months; the
// answer says how long, and that is what is stored. A refresh may answer with
// a new refresh token, which replaces the old one (publisher.Refresher).
//
// The file is pushed to TikTok in chunks (FILE_UPLOAD). The other way,
// PULL_FROM_URL, needs the bucket's domain verified with TikTok.
//
// References (checked 2026-11-15):
// https://developers.tiktok.com/doc/login-kit-web
// https://developers.tiktok.com/doc/oauth-user-access-token-management
// https://developers.tiktok.com/docs/en/oauth-get-a-user-access-token
// https://developers.tiktok.com/docs/en/oauth-refresh-a-user-access-token
// https://developers.tiktok.com/doc/tiktok-api-v2-get-user-info
// https://developers.tiktok.com/doc/content-posting-api-reference-upload-video
// https://developers.tiktok.com/doc/content-posting-api-media-transfer-guide
// https://developers.tiktok.com/doc/content-posting-api-reference-get-video-status
package tiktok

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FACorreiaa/postitheka/internal/publisher"
)

const (
	authorizeURL = "https://www.tiktok.com/v2/auth/authorize/"
	// apiBase is fixed: stored tokens can only ever be sent here. Tests
	// redirect the transport instead.
	apiBase   = "https://open.tiktokapis.com"
	tokenPath = "/v2/oauth/token/"

	// chunkSize is what one PUT carries. TikTok takes 5 MB to 64 MB a chunk;
	// a file smaller than a chunk goes up whole, and the last chunk takes the
	// bytes left over, so it can reach just under twice this (TikTok allows
	// it 128 MB).
	chunkSize = 10 << 20

	requestTimeout = 30 * time.Second
	// chunkTimeout is for one chunk of up to 20 MB on a slow link.
	chunkTimeout = 5 * time.Minute

	// refreshMargin is how long before expiry a token is renewed. A publish
	// may run for 30 minutes (jobs.maxPublishTime), and a token that dies
	// halfway through an upload would fail it.
	refreshMargin = 40 * time.Minute

	// rateLimitWait is the wait after a 429. TikTok's limits are per minute
	// (6 uploads started a minute for one account) and its answer carries no
	// reset time.
	rateLimitWait = time.Minute

	// settleTime is how long Publish waits for TikTok to say the video
	// reached the inbox, once every byte is with TikTok.
	settleTime = 2 * time.Minute
)

// user.info.basic is for the account's name and id; video.upload for the
// inbox. TikTok separates scopes with commas.
const scopes = "user.info.basic,video.upload"

type Publisher struct {
	client      *http.Client
	clientKey   string
	secret      string
	pollEvery   time.Duration
	settleLimit time.Duration
	now         func() time.Time
}

// New returns a Publisher for the developer app with this client key and
// secret. A nil client gets a default; every request carries its own
// deadline.
func New(clientKey, clientSecret string, client *http.Client) *Publisher {
	if client == nil {
		client = &http.Client{}
	}
	return &Publisher{
		client: client, clientKey: clientKey, secret: clientSecret,
		pollEvery: 5 * time.Second, settleLimit: settleTime, now: time.Now,
	}
}

func (p *Publisher) Platform() publisher.Platform { return publisher.TikTok }

// Rules for a video sent to the inbox, checked 2026-11-15.
//
// From TikTok's pages: one video, MP4, MOV or WebM; at most 10 minutes
// through this endpoint; no side longer than 4096 pixels.
//
// Left out, on purpose:
//   - Photos. TikTok only pulls them from a domain verified with it.
//   - The shorter side: TikTok wants both sides between 360 and 4096 pixels,
//     and Rules holds a limit on the width alone.
//   - Frame rate (23 to 60) and codec: no client reads them.
//
// Postitheka's own, not TikTok's:
//   - A video of 1 GiB. TikTok takes 4 GB, but every byte passes through
//     this server within the publish job's time limit.
//   - The text limit. The post's text is not sent: an inbox upload has no
//     caption, the person writes it in TikTok. 5000 is the longest text any
//     other platform here takes, so a post written for them is never refused
//     on TikTok's account.
func (p *Publisher) Rules() publisher.Rules {
	return publisher.Rules{
		MaxTextLength: 5000,
		MaxMedia:      1,
		MaxMediaBytes: 1 << 30,
		MediaTypes:    []string{"video/mp4", "video/quicktime", "video/webm"},
		RequiresMedia: true,
		VideoAlone:    true,

		MaxVideoLength: 10 * time.Minute,
		MaxVideoWidth:  4096,
	}
}

// credentials is what is sealed for an account.
type credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	// RefreshExpiresAt is when the refresh token itself dies; zero when
	// TikTok did not say.
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	OpenID           string    `json:"open_id"`
}

func (c credentials) valid() bool { return c.AccessToken != "" && c.OpenID != "" }

// AuthorizeURL ignores the verifier: TikTok's web flow has no PKCE (its
// mobile and desktop flows do).
func (p *Publisher) AuthorizeURL(state, _, redirectURL string) string {
	q := url.Values{
		"client_key":    {p.clientKey},
		"scope":         {scopes},
		"response_type": {"code"},
		"redirect_uri":  {redirectURL},
		"state":         {state},
	}
	return authorizeURL + "?" + q.Encode()
}

func (p *Publisher) Exchange(ctx context.Context, code, _, redirectURL string) (publisher.Connection, error) {
	creds, err := p.token(ctx, "exchange code", url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURL},
	})
	if err != nil {
		return publisher.Connection{}, err
	}

	var me struct {
		Data struct {
			User struct {
				OpenID      string `json:"open_id"`
				DisplayName string `json:"display_name"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := p.call(ctx, creds, http.MethodGet, "/v2/user/info/?fields=open_id,display_name", nil, "read account", &me); err != nil {
		return publisher.Connection{}, err
	}
	if id := me.Data.User.OpenID; id != "" {
		creds.OpenID = id
	}
	if creds.OpenID == "" {
		return publisher.Connection{}, errors.New("tiktok: read account: no account id in the answer")
	}
	name := me.Data.User.DisplayName
	if name == "" {
		name = creds.OpenID
	}
	raw, _ := json.Marshal(creds)
	return publisher.Connection{DisplayName: name, Credentials: raw, RemoteID: creds.OpenID}, nil
}

// tokenErr is a refusal from the token endpoint, as opposed to no answer.
type tokenErr struct {
	status int
	err    error
}

func (e *tokenErr) Error() string { return e.err.Error() }

// token posts to the token endpoint with the app's key and secret added.
func (p *Publisher) token(ctx context.Context, op string, form url.Values) (credentials, error) {
	form.Set("client_key", p.clientKey)
	form.Set("client_secret", p.secret)
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+tokenPath, strings.NewReader(form.Encode()))
	if err != nil {
		return credentials{}, fmt.Errorf("tiktok: %s: %w", op, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client.Do(req)
	if err != nil {
		return credentials{}, fmt.Errorf("tiktok: %s: %w", op, bare(err))
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int64  `json:"expires_in"`
		RefreshExpiresIn int64  `json:"refresh_expires_in"`
		OpenID           string `json:"open_id"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = json.Unmarshal(raw, &out)
	// TikTok can answer 200 with an error in the body.
	if resp.StatusCode/100 != 2 || out.Error != "" || out.AccessToken == "" {
		msg := strings.TrimSpace(out.Error + " " + out.ErrorDescription)
		if msg == "" {
			msg = "no access token in the answer"
		}
		return credentials{}, &tokenErr{status: resp.StatusCode, err: fmt.Errorf("tiktok: %s: %d: %s", op, resp.StatusCode, msg)}
	}

	now := p.now()
	c := credentials{AccessToken: out.AccessToken, RefreshToken: out.RefreshToken, OpenID: out.OpenID}
	// TikTok's page says two hours; the answer is what counts.
	c.ExpiresAt = now.Add(2 * time.Hour)
	if out.ExpiresIn > 0 {
		c.ExpiresAt = now.Add(time.Duration(out.ExpiresIn) * time.Second)
	}
	if out.RefreshExpiresIn > 0 {
		c.RefreshExpiresAt = now.Add(time.Duration(out.RefreshExpiresIn) * time.Second)
	}
	return c, nil
}

func (p *Publisher) NeedsRefresh(raw []byte, now time.Time) bool {
	var c credentials
	if json.Unmarshal(raw, &c) != nil || !c.valid() || c.RefreshToken == "" {
		return false
	}
	return !now.Before(c.ExpiresAt.Add(-refreshMargin))
}

// Refresh trades the refresh token for a new access token. TikTok may send
// a new refresh token with it; the one in the answer is the one to keep.
func (p *Publisher) Refresh(ctx context.Context, raw []byte) ([]byte, error) {
	var c credentials
	if err := json.Unmarshal(raw, &c); err != nil || !c.valid() || c.RefreshToken == "" {
		return nil, publisher.Permanent(errors.New("tiktok: stored credentials are unreadable"))
	}
	fresh, err := p.token(ctx, "refresh token", url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.RefreshToken},
	})
	if err != nil {
		var te *tokenErr
		if errors.As(err, &te) && te.status < 500 {
			// The grant is gone: revoked by the user, or the refresh token
			// expired or was already replaced.
			return nil, publisher.Permanent(err)
		}
		return nil, err
	}
	if fresh.RefreshToken == "" {
		fresh.RefreshToken, fresh.RefreshExpiresAt = c.RefreshToken, c.RefreshExpiresAt
	}
	fresh.OpenID = c.OpenID
	return json.Marshal(fresh)
}

func (p *Publisher) Publish(ctx context.Context, raw []byte, post publisher.Post, media publisher.MediaSource) (publisher.Result, error) {
	var creds credentials
	if err := json.Unmarshal(raw, &creds); err != nil || !creds.valid() {
		return publisher.Result{}, publisher.Permanent(errors.New("tiktok: stored credentials are unreadable"))
	}
	if !creds.ExpiresAt.IsZero() && !p.now().Before(creds.ExpiresAt) {
		// The worker refreshes before publishing; a token still expired
		// here has no refresh token behind it.
		return publisher.Result{}, publisher.Reconnect(errors.New("tiktok: access expired; reconnect the account"))
	}
	if len(post.Media) != 1 || !publisher.IsVideo(post.Media[0].ContentType) {
		return publisher.Result{}, publisher.Permanent(errors.New("tiktok: a draft is one video"))
	}
	m := post.Media[0]

	size := m.Size
	chunk := min(size, int64(chunkSize))
	if chunk <= 0 {
		return publisher.Result{}, publisher.Permanent(errors.New("tiktok: the video is empty"))
	}
	// The last chunk takes what is left over, so the count rounds down.
	count := max(size/chunk, 1)

	start, _ := json.Marshal(map[string]any{"source_info": map[string]any{
		"source": "FILE_UPLOAD", "video_size": size, "chunk_size": chunk, "total_chunk_count": count,
	}})
	var started struct {
		Data struct {
			PublishID string `json:"publish_id"`
			UploadURL string `json:"upload_url"`
		} `json:"data"`
	}
	if err := p.call(ctx, creds, http.MethodPost, "/v2/post/publish/inbox/video/init/", start, "start upload", &started); err != nil {
		return publisher.Result{}, err
	}
	id, target := started.Data.PublishID, started.Data.UploadURL
	if id == "" || target == "" {
		return publisher.Result{}, errors.New("tiktok: start upload: no upload address in the answer")
	}
	if u, err := url.Parse(target); err != nil || u.Scheme != "https" || u.Host == "" {
		return publisher.Result{}, publisher.Permanent(errors.New("tiktok: start upload: the upload address is not an https URL"))
	}

	file, err := media.Open(ctx, m.Key)
	if err != nil {
		return publisher.Result{}, fmt.Errorf("tiktok: open media: %w", err)
	}
	defer func() { _ = file.Close() }()

	for i := int64(0); i < count; i++ {
		first := i * chunk
		last := first + chunk - 1
		if i == count-1 {
			last = size - 1
		}
		buf := make([]byte, last-first+1)
		if _, err := io.ReadFull(file, buf); err != nil {
			return publisher.Result{}, fmt.Errorf("tiktok: read media: %w", err)
		}
		status, err := p.put(ctx, target, m.ContentType, buf, first, last, size)
		final := i == count-1
		switch {
		case err == nil && final && status == http.StatusCreated:
		case err == nil && !final && status == http.StatusPartialContent:
		case final && (err != nil || status >= 500):
			// The answer to the last chunk is lost, so the video may be in
			// the inbox. Sending again would put a second one beside it.
			return publisher.Result{}, publisher.Permanent(errors.New("tiktok: upload: the last part was sent and TikTok did not confirm it; check the TikTok inbox before sending again"))
		case err != nil:
			return publisher.Result{}, err
		case status >= 500:
			return publisher.Result{}, fmt.Errorf("tiktok: upload: %d", status)
		default:
			// 400, 403 (the address expired), 404, 416: this upload is dead;
			// a new attempt starts a new one.
			return publisher.Result{}, fmt.Errorf("tiktok: upload: TikTok answered %d to part %d of %d", status, i+1, count)
		}
	}

	// Every byte is with TikTok. From here on nothing is an error that a
	// retry should answer, because a retry uploads the video again.
	if reason, failed := p.settle(ctx, creds, id); failed {
		return publisher.Result{}, publisher.Permanent(fmt.Errorf("tiktok: TikTok refused the video: %s", reason))
	}
	return publisher.Result{RemoteID: id, Draft: true}, nil
}

// put sends one chunk to the upload address. The address is the credential;
// no token goes with it.
func (p *Publisher) put(ctx context.Context, target, contentType string, chunk []byte, first, last, total int64) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, chunkTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, bytes.NewReader(chunk))
	if err != nil {
		return 0, errors.New("tiktok: upload: the upload address is unusable")
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", first, last, total))
	resp, err := p.client.Do(req)
	if err != nil {
		// The address carries the upload's signature in its query.
		return 0, fmt.Errorf("tiktok: upload: %w", bare(err))
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, nil
}

// settle waits for TikTok to say what became of the upload. It reports a
// failure only when TikTok names one; an unanswered question, or a video
// still being processed when the wait ends, counts as delivered.
func (p *Publisher) settle(ctx context.Context, creds credentials, id string) (reason string, failed bool) {
	deadline := p.now().Add(p.settleLimit)
	body, _ := json.Marshal(map[string]string{"publish_id": id})
	for {
		var out struct {
			Data struct {
				Status     string `json:"status"`
				FailReason string `json:"fail_reason"`
			} `json:"data"`
		}
		if err := p.call(ctx, creds, http.MethodPost, "/v2/post/publish/status/fetch/", body, "check upload", &out); err != nil {
			return "", false
		}
		switch out.Data.Status {
		case "FAILED":
			if out.Data.FailReason == "" {
				return "no reason given", true
			}
			return out.Data.FailReason, true
		case "SEND_TO_USER_INBOX", "PUBLISH_COMPLETE":
			return "", false
		}
		if !p.now().Add(p.pollEvery).Before(deadline) {
			return "", false
		}
		select {
		case <-ctx.Done():
			return "", false
		case <-time.After(p.pollEvery):
		}
	}
}

// call sends one request with the account's token and decodes the answer
// into out. TikTok wraps every answer: {"data": ..., "error": {"code": "ok"}}.
func (p *Publisher) call(ctx context.Context, creds credentials, method, path string, body []byte, op string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, reader)
	if err != nil {
		return fmt.Errorf("tiktok: %s: %w", op, err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("tiktok: %s: %w", op, bare(err))
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	code := envelope.Error.Code
	if resp.StatusCode/100 != 2 || (code != "" && code != "ok") {
		return apiError(op, resp.StatusCode, code, envelope.Error.Message)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("tiktok: %s: unreadable answer: %w", op, err)
	}
	return nil
}

// apiError classifies a refusal by TikTok's error code first and the HTTP
// status second.
func apiError(op string, status int, code, message string) error {
	msg := strings.TrimSpace(code + " " + message)
	if msg == "" {
		msg = http.StatusText(status)
	}
	err := fmt.Errorf("tiktok: %s: %d: %s", op, status, msg)
	switch {
	case code == "rate_limit_exceeded" || status == http.StatusTooManyRequests:
		return publisher.RetryAfter(rateLimitWait, err)
	case code == "access_token_invalid" || code == "scope_not_authorized" || status == http.StatusUnauthorized:
		// The token was revoked, or the upload permission removed.
		return publisher.Reconnect(fmt.Errorf("%w (reconnect the account)", err))
	case code == "spam_risk_too_many_pending_share":
		return publisher.Permanent(fmt.Errorf("%w: TikTok holds at most 5 unfinished drafts from the last 24 hours; post or delete some in TikTok, then send this again", err))
	case status >= 500:
		return err
	default:
		return publisher.Permanent(err)
	}
}

// bare drops the request URL from a transport error: an upload address
// carries its signature in the query.
func bare(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
