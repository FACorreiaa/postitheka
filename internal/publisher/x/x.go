// Package x publishes to an X account.
//
// Access is OAuth 2.0 with PKCE as a confidential client: the token endpoint
// takes the client id and secret as HTTP Basic credentials. An access token
// lasts two hours; offline.access adds a refresh token, and X issues a new
// one with every refresh, so a refresh must be stored before it is used
// (publisher.Refresher; the worker holds a lock on the account for it).
//
// The authorization code lasts 30 seconds, so the callback exchanges it at
// once.
//
// Media goes through the v2 chunked upload: initialize, append in segments,
// finalize, then wait while X processes it. Only then is the post created.
// Nothing before the post is visible to anyone, so every upload step is safe
// to retry; the post itself is not, because X offers no idempotency key.
//
// X is pay-per-use: every post created is billed to the developer app.
//
// References (checked 2026-11-15):
// https://docs.x.com/fundamentals/authentication/oauth-2-0/authorization-code
// https://docs.x.com/fundamentals/authentication/oauth-2-0/user-access-token
// https://docs.x.com/x-api/posts/create-post
// https://docs.x.com/x-api/media/quickstart/media-upload-chunked
// https://docs.x.com/x-api/media/quickstart/best-practices
// https://docs.x.com/fundamentals/counting-characters
// https://docs.x.com/x-api/fundamentals/rate-limits
package x

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/oauth2"

	"github.com/FACorreiaa/postitheka/internal/publisher"
)

const (
	authorizeURL = "https://x.com/i/oauth2/authorize"
	tokenURL     = "https://api.x.com/2/oauth2/token"
	// apiBase is fixed: stored tokens can only ever be sent here. Tests
	// redirect the transport instead.
	apiBase = "https://api.x.com"

	// segmentSize is how much of a file one append carries. X's guide says
	// to keep a segment at or below 5 MB (the server takes 8 MB) and its own
	// examples use 4 MB.
	segmentSize = 4 << 20

	requestTimeout = 30 * time.Second
	// appendTimeout is for one 4 MB segment on a slow link.
	appendTimeout = 2 * time.Minute

	// refreshMargin is how long before expiry a token is renewed. A publish
	// may run for 30 minutes (jobs.maxPublishTime), and a token that dies
	// halfway through an upload would fail the post.
	refreshMargin = 40 * time.Minute

	// rateLimitFallback is the wait when a 429 carries no reset time: X's
	// windows are 15 minutes.
	rateLimitFallback = 15 * time.Minute
)

// media.write is for the upload; users.read for the account's name;
// offline.access for the refresh token. tweet.read is listed beside
// tweet.write by the create-post reference.
var scopes = []string{"tweet.read", "tweet.write", "users.read", "media.write", "offline.access"}

type Publisher struct {
	client *http.Client
	oauth  oauth2.Config
	// maxPoll caps the wait X asks for between processing checks.
	maxPoll time.Duration
	now     func() time.Time
}

// New returns a Publisher for the developer app with this client id and
// secret. A nil client gets a default; every request carries its own
// deadline.
func New(clientID, clientSecret string, client *http.Client) *Publisher {
	if client == nil {
		client = &http.Client{}
	}
	return &Publisher{
		client: client,
		oauth: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       scopes,
			// A confidential client authenticates with a Basic header.
			Endpoint: oauth2.Endpoint{AuthURL: authorizeURL, TokenURL: tokenURL, AuthStyle: oauth2.AuthStyleInHeader},
		},
		maxPoll: 30 * time.Second,
		now:     time.Now,
	}
}

func (p *Publisher) Platform() publisher.Platform { return publisher.X }

// Rules for a post, checked 2026-11-15.
//
// From X's pages: 280 weighted characters (CountXWeighted); up to 4 photos
// or 1 video in a post; images JPG, PNG or WEBP up to 5 MB; a video is 0.5
// seconds to 20 minutes for an account without Premium, and between 1:3 and
// 3:1. Premium accounts are allowed more; these are the limits every account
// has.
//
// Left out, on purpose:
//   - GIF. X takes one animated GIF, alone, up to 15 MB. Rules cannot say
//     "this type must be alone" or give one image type its own size, so a
//     GIF would be accepted here in posts X refuses.
//   - A limit on a video's width or height. The same page says dimensions
//     must be between 32x32 and 1280x1024 and recommends 720x1280.
//
// Postitheka's own cap, not X's: a video of 1 GiB. X takes 8 GB, but every
// byte passes through this server in 4 MB segments within the publish job's
// time limit.
func (p *Publisher) Rules() publisher.Rules {
	return publisher.Rules{
		MaxTextLength: 280,
		TextCounting:  publisher.CountXWeighted,
		MaxMedia:      4,
		MaxMediaBytes: 5_000_000,
		MaxVideoBytes: 1 << 30,
		MediaTypes:    []string{"image/jpeg", "image/png", "image/webp", "video/mp4", "video/quicktime"},
		VideoAlone:    true,

		MinVideoLength: 500 * time.Millisecond,
		MaxVideoLength: 20 * time.Minute,
		MinVideoAspect: 1.0 / 3,
		MaxVideoAspect: 3,
	}
}

// credentials is what is sealed for an account.
type credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	UserID       string    `json:"user_id"`
	// Username is the handle, kept for the link to a published post.
	Username string `json:"username"`
}

func (c credentials) valid() bool { return c.AccessToken != "" && c.UserID != "" }

func (p *Publisher) AuthorizeURL(state, verifier, redirectURL string) string {
	cfg := p.oauth
	cfg.RedirectURL = redirectURL
	return cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

func (p *Publisher) Exchange(ctx context.Context, code, verifier, redirectURL string) (publisher.Connection, error) {
	cfg := p.oauth
	cfg.RedirectURL = redirectURL
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	tok, err := cfg.Exchange(context.WithValue(ctx, oauth2.HTTPClient, p.client), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return publisher.Connection{}, fmt.Errorf("x: exchange code: %w", tokenError(err))
	}
	creds := p.fromToken(tok)

	var me struct {
		Data struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"data"`
	}
	if err := p.call(ctx, creds, http.MethodGet, "/2/users/me", nil, "", "read account", requestTimeout, &me); err != nil {
		return publisher.Connection{}, err
	}
	if me.Data.ID == "" {
		return publisher.Connection{}, errors.New("x: read account: no user id in the answer")
	}
	creds.UserID, creds.Username = me.Data.ID, me.Data.Username
	raw, _ := json.Marshal(creds)
	name := "@" + me.Data.Username
	if me.Data.Username == "" {
		name = me.Data.ID
	}
	return publisher.Connection{DisplayName: name, Credentials: raw, RemoteID: me.Data.ID}, nil
}

func (p *Publisher) fromToken(tok *oauth2.Token) credentials {
	expires := tok.Expiry
	if expires.IsZero() {
		// X always sends expires_in; this is the documented lifespan should
		// it ever not.
		expires = p.now().Add(2 * time.Hour)
	}
	return credentials{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, ExpiresAt: expires}
}

// tokenError keeps what the token endpoint said and drops the rest: the
// oauth2 package's error carries the whole response body.
func tokenError(err error) error {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		status := 0
		if re.Response != nil {
			status = re.Response.StatusCode
		}
		return fmt.Errorf("%d %s %s", status, re.ErrorCode, re.ErrorDescription)
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

func (p *Publisher) NeedsRefresh(raw []byte, now time.Time) bool {
	var c credentials
	if json.Unmarshal(raw, &c) != nil || !c.valid() || c.RefreshToken == "" {
		return false
	}
	return !now.Before(c.ExpiresAt.Add(-refreshMargin))
}

// Refresh trades the refresh token for a new access token and a new refresh
// token. The old refresh token is spent by this call, whatever happens to
// the answer afterwards.
func (p *Publisher) Refresh(ctx context.Context, raw []byte) ([]byte, error) {
	var c credentials
	if err := json.Unmarshal(raw, &c); err != nil || !c.valid() || c.RefreshToken == "" {
		return nil, publisher.Permanent(errors.New("x: stored credentials are unreadable"))
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	// A token with no expiry and no access token is always refreshed.
	src := p.oauth.TokenSource(context.WithValue(ctx, oauth2.HTTPClient, p.client), &oauth2.Token{RefreshToken: c.RefreshToken})
	tok, err := src.Token()
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && re.Response != nil && (re.Response.StatusCode == http.StatusBadRequest || re.Response.StatusCode == http.StatusUnauthorized) {
			// The grant is gone: revoked by the user, or the refresh token
			// was already spent.
			return nil, publisher.Permanent(fmt.Errorf("x: refresh token: %w", tokenError(err)))
		}
		return nil, fmt.Errorf("x: refresh token: %w", tokenError(err))
	}
	fresh := p.fromToken(tok)
	if fresh.RefreshToken == "" {
		fresh.RefreshToken = c.RefreshToken
	}
	fresh.UserID, fresh.Username = c.UserID, c.Username
	return json.Marshal(fresh)
}

func (p *Publisher) Publish(ctx context.Context, raw []byte, post publisher.Post, media publisher.MediaSource) (publisher.Result, error) {
	var creds credentials
	if err := json.Unmarshal(raw, &creds); err != nil || !creds.valid() {
		return publisher.Result{}, publisher.Permanent(errors.New("x: stored credentials are unreadable"))
	}
	if !creds.ExpiresAt.IsZero() && !p.now().Before(creds.ExpiresAt) {
		// The worker refreshes before publishing; a token still expired
		// here has no refresh token behind it.
		return publisher.Result{}, publisher.Reconnect(errors.New("x: access expired; reconnect the account"))
	}

	body := map[string]any{}
	if post.Text != "" {
		body["text"] = post.Text
	}
	if len(post.Media) > 0 {
		ids := make([]string, 0, len(post.Media))
		for _, m := range post.Media {
			id, err := p.upload(ctx, creds, m, media)
			if err != nil {
				return publisher.Result{}, err
			}
			ids = append(ids, id)
		}
		body["media"] = map[string]any{"media_ids": ids}
	}
	payload, _ := json.Marshal(body)

	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	err := p.call(ctx, creds, http.MethodPost, "/2/tweets", bytes.NewReader(payload), "application/json", "create post", requestTimeout, &out)
	var api *apiErr
	switch {
	case err == nil && out.Data.ID != "":
		res := publisher.Result{RemoteID: out.Data.ID}
		if creds.Username != "" {
			// The address every X post has. X's API reference does not
			// state it, so it is to be confirmed with the first real post.
			res.URL = "https://x.com/" + url.PathEscape(creds.Username) + "/status/" + url.PathEscape(out.Data.ID)
		}
		return res, nil
	case errors.As(err, &api) && api.refused:
		// X answered and said no: nothing was posted.
		return publisher.Result{}, err
	default:
		// No idempotency key and no way to ask whether the request landed:
		// a second try could put the same post on the timeline twice, and
		// be billed twice.
		return publisher.Result{}, publisher.Permanent(errors.New("x: create post: the request failed and X did not confirm whether the post went out; check the account before posting again"))
	}
}

// upload sends one file through the chunked upload and returns its media id
// once X has finished processing it.
func (p *Publisher) upload(ctx context.Context, creds credentials, m publisher.Media, src publisher.MediaSource) (string, error) {
	category := "tweet_image"
	if publisher.IsVideo(m.ContentType) {
		category = "tweet_video"
	}
	init, _ := json.Marshal(map[string]any{"media_type": m.ContentType, "total_bytes": m.Size, "media_category": category})
	var started struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := p.call(ctx, creds, http.MethodPost, "/2/media/upload/initialize", bytes.NewReader(init), "application/json", "start upload", requestTimeout, &started); err != nil {
		return "", err
	}
	id := started.Data.ID
	if id == "" {
		return "", errors.New("x: start upload: no media id in the answer")
	}
	path := "/2/media/upload/" + url.PathEscape(id)

	file, err := src.Open(ctx, m.Key)
	if err != nil {
		return "", fmt.Errorf("x: open media: %w", err)
	}
	defer func() { _ = file.Close() }()
	segment := make([]byte, segmentSize)
	for index := 0; ; index++ {
		n, readErr := io.ReadFull(file, segment)
		if n > 0 {
			var buf bytes.Buffer
			form := multipart.NewWriter(&buf)
			_ = form.WriteField("segment_index", strconv.Itoa(index))
			part, _ := form.CreateFormFile("media", m.Filename)
			_, _ = part.Write(segment[:n])
			_ = form.Close()
			if err := p.call(ctx, creds, http.MethodPost, path+"/append", &buf, form.FormDataContentType(), "upload", appendTimeout, nil); err != nil {
				return "", err
			}
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("x: read media: %w", readErr)
		}
	}

	var state mediaState
	if err := p.call(ctx, creds, http.MethodPost, path+"/finalize", nil, "", "finish upload", requestTimeout, &state); err != nil {
		return "", err
	}
	// No processing_info means the media is ready as it is.
	for state.Data.Processing != nil && state.Data.Processing.State != "succeeded" {
		info := state.Data.Processing
		if info.State == "failed" {
			msg := "X could not process the file"
			if info.Error != nil && info.Error.Message != "" {
				msg = info.Error.Message
			}
			return "", publisher.Permanent(fmt.Errorf("x: process media: %s", msg))
		}
		wait := min(max(time.Duration(info.CheckAfterSecs)*time.Second, time.Second), p.maxPoll)
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("x: process media: %w", ctx.Err())
		case <-time.After(wait):
		}
		state = mediaState{}
		query := "?" + url.Values{"command": {"STATUS"}, "media_id": {id}}.Encode()
		if err := p.call(ctx, creds, http.MethodGet, "/2/media/upload"+query, nil, "", "check upload", requestTimeout, &state); err != nil {
			return "", err
		}
	}
	return id, nil
}

type mediaState struct {
	Data struct {
		Processing *struct {
			State          string `json:"state"`
			CheckAfterSecs int    `json:"check_after_secs"`
			Error          *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"processing_info"`
	} `json:"data"`
}

// call sends one request with the account's token and decodes a 2xx answer
// into out, when out is not nil.
func (p *Publisher) call(ctx context.Context, creds credentials, method, path string, body io.Reader, contentType, op string, timeout time.Duration, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, body)
	if err != nil {
		return fmt.Errorf("x: %s: %w", op, err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("x: %s: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return apiError(op, resp, p.now())
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("x: %s: unreadable answer: %w", op, err)
	}
	return nil
}

// apiErr is an answer from X, as opposed to no answer at all.
type apiErr struct {
	// refused: X understood the request and declined it, so nothing was
	// created. A 5xx is not that.
	refused bool
	err     error
}

func (e *apiErr) Error() string { return e.err.Error() }
func (e *apiErr) Unwrap() error { return e.err }

// apiError turns a non-success response into a classified error.
func apiError(op string, resp *http.Response, now time.Time) error {
	// X answers with a problem document (title, detail), or with a list of
	// errors, depending on the endpoint.
	var body struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
		Errors []struct {
			Message string `json:"message"`
			Detail  string `json:"detail"`
		} `json:"errors"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = json.Unmarshal(raw, &body)
	msg := http.StatusText(resp.StatusCode)
	switch {
	case body.Detail != "":
		msg = body.Detail
	case len(body.Errors) > 0 && body.Errors[0].Message != "":
		msg = body.Errors[0].Message
	case len(body.Errors) > 0 && body.Errors[0].Detail != "":
		msg = body.Errors[0].Detail
	case body.Title != "":
		msg = body.Title
	}
	err := fmt.Errorf("x: %s: %d: %s", op, resp.StatusCode, msg)

	var classified error
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		classified = publisher.RetryAfter(untilReset(resp, now), err)
	case resp.StatusCode == http.StatusUnauthorized:
		// The token was revoked, or the app's access removed.
		classified = publisher.Reconnect(fmt.Errorf("%w (reconnect the account)", err))
	case resp.StatusCode >= 500:
		classified = err
	default:
		classified = publisher.Permanent(err)
	}
	return &apiErr{refused: resp.StatusCode < 500, err: classified}
}

// untilReset reads x-rate-limit-reset, the Unix time the window reopens.
func untilReset(resp *http.Response, now time.Time) time.Duration {
	if epoch, err := strconv.ParseInt(resp.Header.Get("x-rate-limit-reset"), 10, 64); err == nil && epoch > 0 {
		if wait := time.Unix(epoch, 0).Sub(now); wait > 0 {
			// A minute at least, as X's own example does: a reset a second
			// away is often not yet in effect.
			return max(wait, time.Minute)
		}
		return time.Minute
	}
	return rateLimitFallback
}
