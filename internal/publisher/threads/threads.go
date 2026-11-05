// Package threads publishes to a user's own Threads profile.
//
// Access is the Threads API (threads_basic, threads_content_publish) with the
// Threads app id and secret, which are not the main Meta app's. Before App
// Review only the app's Threads testers can connect.
//
// A connection exchanges the one-hour token for a long-lived one (60 days).
// There is no refresh token: the long-lived token refreshes itself once it is
// a day old and before it expires, so this package implements
// publisher.Refresher and renews it a week after each refresh. A token not
// used for 60 days cannot be renewed and the account must be connected again.
//
// Meta downloads media from a URL; nothing is uploaded. Every post is a
// container that Threads processes and then publishes.
//
// References (checked 2026-10-23, rendered pages; plain fetches get a shell):
// https://developers.facebook.com/docs/threads/get-started/get-access-tokens-and-permissions
// https://developers.facebook.com/docs/threads/get-started/long-lived-tokens
// https://developers.facebook.com/docs/threads/posts
// https://developers.facebook.com/docs/threads/reference/publishing
// https://developers.facebook.com/docs/threads/troubleshooting
package threads

import (
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
	authorizeURL = "https://threads.com/oauth/authorize"
	// graphBase is fixed: stored tokens can only ever be sent here. Tests
	// redirect the transport instead.
	graphBase = "https://graph.threads.com"
	apiBase   = graphBase + "/v1.0"

	scopes = "threads_basic,threads_content_publish"

	// refreshAfter is how old a long-lived token gets before it is renewed.
	// Threads allows it after a day; a week keeps the calls rare while an
	// account that posts at all never reaches its 60 days.
	refreshAfter = 7 * 24 * time.Hour
	// mediaURLTTL covers Meta fetching the file while the container
	// processes, which can trail the request by minutes.
	mediaURLTTL = time.Hour
)

// ErrExpired is a long-lived token past its 60 days. Only reconnecting fixes it.
var ErrExpired = errors.New("threads: access expired; reconnect the account")

type Publisher struct {
	client         *http.Client
	appID          string
	appSecret      string
	processingWait time.Duration
	pollEvery      time.Duration
	now            func() time.Time
}

// New returns a Publisher for the Threads app with this id and secret. A nil
// client gets a default with a timeout.
func New(appID, appSecret string, client *http.Client) *Publisher {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Publisher{
		client:    client,
		appID:     appID,
		appSecret: appSecret,
		// Meta suggests about 30 seconds and polling for up to 5 minutes; the
		// job itself has 5, so processing gets 3 and the rest is requests.
		processingWait: 3 * time.Minute,
		pollEvery:      10 * time.Second,
		now:            time.Now,
	}
}

func (p *Publisher) Platform() publisher.Platform { return publisher.Threads }

// Rules for a Threads post, checked 2026-10-23 on the posts page: 500
// characters with emoji counted as their UTF-8 bytes, at most 5 links, a
// carousel of 2 to 20 images and videos in any mix. Images JPEG or PNG up to
// 8 MB; video MP4 or MOV up to 1 GB and 5 minutes. Meta writes MB and GB, so
// the decimal reading, the smaller one, is used. Duration and pixel sizes are
// not checked until Rules can express them (VID-001).
func (p *Publisher) Rules() publisher.Rules {
	return publisher.Rules{
		MaxTextLength: 500,
		MaxLinks:      5,
		MaxMedia:      20,
		MaxMediaBytes: 8_000_000,
		MaxVideoBytes: 1_000_000_000,
		MediaTypes:    []string{"image/jpeg", "image/png", "video/mp4", "video/quicktime"},
		TextCounting:  publisher.CountEmojiBytes,
	}
}

// credentials is what is sealed for an account.
type credentials struct {
	AccessToken string `json:"access_token"`
	// IssuedAt is when the long-lived token was issued or last refreshed;
	// Threads refuses a refresh within a day of it.
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	UserID    string    `json:"user_id"`
}

func (c credentials) valid() bool { return c.AccessToken != "" && c.UserID != "" }

// AuthorizeURL ignores the verifier: the Threads authorization window has no
// PKCE. The server proves itself with the app secret, and the state the
// caller generated guards the callback.
func (p *Publisher) AuthorizeURL(state, _, redirectURL string) string {
	return authorizeURL + "?" + url.Values{
		"client_id":     {p.appID},
		"redirect_uri":  {redirectURL},
		"scope":         {scopes},
		"response_type": {"code"},
		"state":         {state},
	}.Encode()
}

func (p *Publisher) Exchange(ctx context.Context, code, _, redirectURL string) (publisher.Connection, error) {
	var short struct {
		AccessToken string      `json:"access_token"`
		UserID      json.Number `json:"user_id"`
	}
	form := url.Values{
		"client_id":     {p.appID},
		"client_secret": {p.appSecret},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURL},
		"code":          {code},
	}
	if err := p.call(ctx, http.MethodPost, graphBase+"/oauth/access_token", form, "exchange code", &short); err != nil {
		return publisher.Connection{}, err
	}
	if short.AccessToken == "" || short.UserID == "" {
		return publisher.Connection{}, errors.New("threads: exchange code: no token or user id in the answer")
	}

	long, err := p.token(ctx, graphBase+"/access_token", url.Values{
		"grant_type":    {"th_exchange_token"},
		"client_secret": {p.appSecret},
		"access_token":  {short.AccessToken},
	}, "long-lived token")
	if err != nil {
		return publisher.Connection{}, err
	}
	long.UserID = short.UserID.String()

	var me struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	q := url.Values{"fields": {"id,username,name"}, "access_token": {long.AccessToken}}
	if err := p.call(ctx, http.MethodGet, apiBase+"/me", q, "profile", &me); err != nil {
		return publisher.Connection{}, err
	}
	name := me.Name
	if me.Username != "" {
		name = "@" + me.Username
	}
	raw, _ := json.Marshal(long)
	return publisher.Connection{DisplayName: name, Credentials: raw, RemoteID: long.UserID}, nil
}

// token asks for a long-lived token, by exchange or refresh; both answer the
// same shape.
func (p *Publisher) token(ctx context.Context, endpoint string, q url.Values, op string) (credentials, error) {
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := p.call(ctx, http.MethodGet, endpoint, q, op, &tok); err != nil {
		return credentials{}, err
	}
	if tok.AccessToken == "" {
		return credentials{}, fmt.Errorf("threads: %s: no token in the answer", op)
	}
	now := p.now()
	expires := now.Add(time.Duration(tok.ExpiresIn) * time.Second)
	if tok.ExpiresIn <= 0 {
		// The documented lifespan, should expires_in ever be missing.
		expires = now.Add(60 * 24 * time.Hour)
	}
	return credentials{AccessToken: tok.AccessToken, IssuedAt: now, ExpiresAt: expires}, nil
}

func (p *Publisher) NeedsRefresh(raw []byte, now time.Time) bool {
	var c credentials
	if json.Unmarshal(raw, &c) != nil || !c.valid() {
		return false
	}
	return now.Sub(c.IssuedAt) >= refreshAfter && now.Before(c.ExpiresAt)
}

// Refresh renews the long-lived token. A failure that may pass, while the
// current token still has more than a day left, keeps the current token: a
// renewal that can wait must not hold up a post.
func (p *Publisher) Refresh(ctx context.Context, raw []byte) ([]byte, error) {
	var c credentials
	if err := json.Unmarshal(raw, &c); err != nil || !c.valid() {
		return nil, publisher.Permanent(errors.New("threads: stored credentials are unreadable"))
	}
	fresh, err := p.token(ctx, graphBase+"/refresh_access_token", url.Values{
		"grant_type":   {"th_refresh_token"},
		"access_token": {c.AccessToken},
	}, "refresh token")
	if err != nil {
		if !publisher.IsPermanent(err) && c.ExpiresAt.Sub(p.now()) > 24*time.Hour {
			return raw, nil
		}
		return nil, err
	}
	fresh.UserID = c.UserID
	return json.Marshal(fresh)
}

func (p *Publisher) Publish(ctx context.Context, raw []byte, post publisher.Post, media publisher.MediaSource) (publisher.Result, error) {
	var creds credentials
	if err := json.Unmarshal(raw, &creds); err != nil || !creds.valid() {
		return publisher.Result{}, publisher.Permanent(errors.New("threads: stored credentials are unreadable"))
	}
	if !creds.ExpiresAt.IsZero() && p.now().After(creds.ExpiresAt) {
		return publisher.Result{}, publisher.Permanent(ErrExpired)
	}

	var container string
	var err error
	switch len(post.Media) {
	case 0:
		container, err = p.create(ctx, creds, url.Values{"media_type": {"TEXT"}, "text": {post.Text}})
	case 1:
		form, ferr := mediaForm(ctx, post.Media[0], media)
		if ferr != nil {
			return publisher.Result{}, ferr
		}
		if post.Text != "" {
			form.Set("text", post.Text)
		}
		container, err = p.create(ctx, creds, form)
	default:
		container, err = p.carousel(ctx, creds, post, media)
	}
	if err != nil {
		return publisher.Result{}, err
	}
	if err := p.waitFinished(ctx, creds, container); err != nil {
		return publisher.Result{}, err
	}
	return p.publish(ctx, creds, container)
}

func mediaForm(ctx context.Context, m publisher.Media, media publisher.MediaSource) (url.Values, error) {
	u, err := media.PublicURL(ctx, m.Key, mediaURLTTL)
	if err != nil {
		return nil, fmt.Errorf("threads: media url: %w", err)
	}
	if publisher.IsVideo(m.ContentType) {
		return url.Values{"media_type": {"VIDEO"}, "video_url": {u}}, nil
	}
	return url.Values{"media_type": {"IMAGE"}, "image_url": {u}}, nil
}

// carousel creates a container per file, waits for each, then the carousel.
// Validate has already kept the count within 2 to 20.
func (p *Publisher) carousel(ctx context.Context, creds credentials, post publisher.Post, media publisher.MediaSource) (string, error) {
	children := make([]string, 0, len(post.Media))
	for _, m := range post.Media {
		form, err := mediaForm(ctx, m, media)
		if err != nil {
			return "", err
		}
		form.Set("is_carousel_item", "true")
		id, err := p.create(ctx, creds, form)
		if err != nil {
			return "", err
		}
		children = append(children, id)
	}
	for _, id := range children {
		if err := p.waitFinished(ctx, creds, id); err != nil {
			return "", err
		}
	}
	form := url.Values{"media_type": {"CAROUSEL"}, "children": {strings.Join(children, ",")}}
	if post.Text != "" {
		form.Set("text", post.Text)
	}
	return p.create(ctx, creds, form)
}

func (p *Publisher) create(ctx context.Context, creds credentials, form url.Values) (string, error) {
	form.Set("access_token", creds.AccessToken)
	var out struct {
		ID string `json:"id"`
	}
	if err := p.call(ctx, http.MethodPost, apiBase+"/"+url.PathEscape(creds.UserID)+"/threads", form, "create container", &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("threads: create container: no id in the answer")
	}
	return out.ID, nil
}

type containerStatus struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
}

func (p *Publisher) status(ctx context.Context, creds credentials, container string) (containerStatus, error) {
	var st containerStatus
	q := url.Values{"fields": {"status,error_message"}, "access_token": {creds.AccessToken}}
	err := p.call(ctx, http.MethodGet, apiBase+"/"+url.PathEscape(container), q, "container status", &st)
	return st, err
}

// waitFinished polls a container until Threads has processed it. Nothing is
// on the profile yet, so every failure here is safe to retry or report.
func (p *Publisher) waitFinished(ctx context.Context, creds credentials, container string) error {
	deadline := p.now().Add(p.processingWait)
	for {
		st, err := p.status(ctx, creds, container)
		if err != nil {
			return err
		}
		switch st.Status {
		case "FINISHED":
			return nil
		case "ERROR":
			msg := st.ErrorMessage
			if msg == "" {
				msg = "UNKNOWN"
			}
			// FAILED_DOWNLOADING_VIDEO, INVALID_DURATION and the like: the file
			// or its URL, which another attempt would hand over unchanged.
			return publisher.Permanent(fmt.Errorf("threads: processing failed: %s", msg))
		case "EXPIRED":
			// A container lives 24 hours; a new attempt makes a new one.
			return errors.New("threads: container expired before it was published")
		}
		if !p.now().Before(deadline) {
			return fmt.Errorf("threads: still processing after %s", p.processingWait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.pollEvery):
		}
	}
}

// publish makes the container a post. This is the only request that puts
// something on the profile, so a failure without a clear answer is checked
// against the container before anyone retries: Threads marks a container
// PUBLISHED once it is live.
func (p *Publisher) publish(ctx context.Context, creds credentials, container string) (publisher.Result, error) {
	form := url.Values{"creation_id": {container}, "access_token": {creds.AccessToken}}
	var out struct {
		ID string `json:"id"`
	}
	err := p.call(ctx, http.MethodPost, apiBase+"/"+url.PathEscape(creds.UserID)+"/threads_publish", form, "publish", &out)
	var api *apiErr
	switch {
	case err == nil && out.ID != "":
		return publisher.Result{RemoteID: out.ID, URL: p.permalink(ctx, creds, out.ID)}, nil
	case errors.As(err, &api) && api.status < 500:
		// Threads answered and said no: nothing was posted.
		return publisher.Result{}, err
	}

	// No answer, a server error, or no id: ask the container. The request's
	// own context may be what ended, so the check gets one of its own.
	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	st, serr := p.status(checkCtx, creds, container)
	switch {
	case serr == nil && st.Status == "PUBLISHED":
		return publisher.Result{}, nil
	case serr == nil && st.Status == "FINISHED":
		// Still waiting to be published after the request ended: it was not.
		if err == nil {
			err = errors.New("threads: publish answered without an id")
		}
		return publisher.Result{}, err
	default:
		return publisher.Result{}, publisher.Permanent(errors.New("threads: the publish request failed and Threads did not confirm whether the post went out; check Threads before posting again"))
	}
}

// permalink is a courtesy: the post is live whether or not this answers.
func (p *Publisher) permalink(ctx context.Context, creds credentials, mediaID string) string {
	var out struct {
		Permalink string `json:"permalink"`
	}
	q := url.Values{"fields": {"permalink"}, "access_token": {creds.AccessToken}}
	if p.call(ctx, http.MethodGet, apiBase+"/"+url.PathEscape(mediaID), q, "permalink", &out) != nil {
		return ""
	}
	return out.Permalink
}

// call sends params as the query of a GET or the form body of a POST, where
// Meta's examples put them, and decodes a 200 into out.
//
// The access token and app secret travel in those params, so no error from
// here may carry the request URL: net/http puts the whole URL, query and
// all, into a transport error, and errors end up in logs and in last_error.
func (p *Publisher) call(ctx context.Context, method, endpoint string, params url.Values, op string, out any) error {
	var req *http.Request
	if method == http.MethodGet {
		req, _ = http.NewRequestWithContext(ctx, method, endpoint+"?"+params.Encode(), nil)
	} else {
		req, _ = http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(params.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("threads: %s: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return apiError(op, resp)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("threads: %s: unreadable answer: %w", op, err)
	}
	return nil
}

// apiErr is an answer from Threads, as opposed to no answer at all.
type apiErr struct {
	status int
	err    error
}

func (e *apiErr) Error() string { return e.err.Error() }
func (e *apiErr) Unwrap() error { return e.err }

// apiError turns a non-success response into a classified error. The Threads
// docs list no error codes; these are the Graph API's, which Threads shares.
func apiError(op string, resp *http.Response) error {
	var body struct {
		Error struct {
			Message     string `json:"message"`
			Code        int    `json:"code"`
			UserMessage string `json:"error_user_msg"`
		} `json:"error"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = json.Unmarshal(raw, &body)
	msg := body.Error.UserMessage
	if msg == "" {
		msg = body.Error.Message
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	err := fmt.Errorf("threads: %s: %d: %s", op, resp.StatusCode, msg)

	var classified error
	switch code := body.Error.Code; {
	case resp.StatusCode == http.StatusTooManyRequests, code == 4, code == 17, code == 32, code == 613:
		// Rate limits, including 250 posts a day. Meta sends no Retry-After;
		// the limits are rolling, so an hour is a fair wait.
		classified = publisher.RetryAfter(time.Hour, err)
	case resp.StatusCode == http.StatusUnauthorized, code == 190:
		// Expired or revoked token: only the user reconnecting fixes it.
		classified = publisher.Permanent(fmt.Errorf("%w (reconnect the account)", err))
	case resp.StatusCode >= 500, code == 1, code == 2:
		classified = err
	default:
		classified = publisher.Permanent(err)
	}
	return &apiErr{status: resp.StatusCode, err: classified}
}
