// Package instagram publishes to an Instagram professional account: Business
// or Creator, never a personal one.
//
// Access is the Instagram API with Instagram Login (instagram_business_basic,
// instagram_business_content_publish) with the Instagram app id and secret
// from the Meta app's "API setup with Instagram login", which are not the
// main Meta app's. No Facebook Page is involved. Before App Review only
// accounts with a role on the app can connect.
//
// A connection exchanges the one-hour token for a long-lived one (60 days).
// There is no refresh token: the long-lived token refreshes itself once it is
// a day old and before it expires, so this package implements
// publisher.Refresher and renews it a week after each refresh. A token not
// refreshed for 60 days cannot be renewed and the account must be connected
// again.
//
// Instagram has no text-only posts, and it downloads media from a URL;
// nothing is uploaded. The resumable upload exists only for apps on Facebook
// Login. Every post is a container that Instagram processes and then
// publishes: one image, one video (a reel), or a carousel of 2 to 20.
//
// References (checked 2026-11-14, rendered pages; plain fetches get a shell):
// https://developers.facebook.com/docs/instagram-platform/instagram-api-with-instagram-login/business-login
// https://developers.facebook.com/docs/instagram-platform/instagram-api-with-instagram-login/get-started
// https://developers.facebook.com/docs/instagram-platform/content-publishing
// https://developers.facebook.com/docs/instagram-platform/instagram-graph-api/reference/ig-user/media
// https://developers.facebook.com/docs/instagram-platform/instagram-graph-api/reference/error-codes
package instagram

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
	authorizeURL = "https://www.instagram.com/oauth/authorize"
	// The code is exchanged on one host and everything after it happens on
	// another. Both are fixed: stored tokens can only ever be sent here.
	// Tests redirect the transport instead.
	exchangeURL = "https://api.instagram.com/oauth/access_token"
	graphBase   = "https://graph.instagram.com"
	apiBase     = graphBase + "/v27.0"

	scopes = "instagram_business_basic,instagram_business_content_publish"

	// refreshAfter is how old a long-lived token gets before it is renewed.
	// Instagram allows it after a day; a week keeps the calls rare while an
	// account that posts at all never reaches its 60 days.
	refreshAfter = 7 * 24 * time.Hour
	// mediaURLTTL covers Instagram fetching the file while the container
	// processes, which can trail the request by minutes.
	mediaURLTTL = time.Hour
)

// ErrExpired is a long-lived token past its 60 days. Only reconnecting fixes it.
var ErrExpired = errors.New("instagram: access expired; reconnect the account")

type Publisher struct {
	client         *http.Client
	appID          string
	appSecret      string
	processingWait time.Duration
	pollEvery      time.Duration
	now            func() time.Time
}

// New returns a Publisher for the Instagram app with this id and secret. A
// nil client gets a default with a timeout.
func New(appID, appSecret string, client *http.Client) *Publisher {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Publisher{
		client:    client,
		appID:     appID,
		appSecret: appSecret,
		// Meta suggests polling for no more than 5 minutes; the job itself
		// has 5, so processing gets 3 and the rest is requests.
		processingWait: 3 * time.Minute,
		pollEvery:      15 * time.Second,
		now:            time.Now,
	}
}

func (p *Publisher) Platform() publisher.Platform { return publisher.Instagram }

// Rules for an Instagram post, checked 2026-11-14 on the content publishing
// guide and the IG User Media reference: a caption of 2,200 characters, no
// post without media, a carousel of up to 20 images and videos in any mix.
// Images are JPEG only, 8 MB; video is MP4 or MOV, 300 MB. Meta writes MB in
// one place and MiB in another, so the decimal reading, the smaller one, is
// used.
//
// Video, checked 2026-11-15 on the IG User Media reference (Reel
// specifications): 3 seconds to 15 minutes, at most 1920 pixels wide, width
// to height between 0.01:1 and 10:1.
//
// Not checked here, and refused by Instagram when the container is made: more
// than 30 hashtags or 20 @-tags, an image outside 4:5 to 1.91:1.
func (p *Publisher) Rules() publisher.Rules {
	return publisher.Rules{
		MaxTextLength: 2200,
		MaxMedia:      20,
		MaxMediaBytes: 8_000_000,
		MaxVideoBytes: 300_000_000,
		MediaTypes:    []string{"image/jpeg", "video/mp4", "video/quicktime"},
		RequiresMedia: true,

		MinVideoLength: 3 * time.Second,
		MaxVideoLength: 15 * time.Minute,
		MinVideoAspect: 0.01,
		MaxVideoAspect: 10,
		MaxVideoWidth:  1920,
	}
}

// credentials is what is sealed for an account.
type credentials struct {
	AccessToken string `json:"access_token"`
	// IssuedAt is when the long-lived token was issued or last refreshed;
	// Instagram refuses a refresh within a day of it.
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	// UserID is the professional account's id (user_id on /me), the one
	// containers are created under. It is not the app-scoped id.
	UserID string `json:"user_id"`
}

func (c credentials) valid() bool { return c.AccessToken != "" && c.UserID != "" }

// AuthorizeURL ignores the verifier: the Instagram authorization window has
// no PKCE. The server proves itself with the app secret, and the state the
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

// shortToken is the answer to the code exchange. The guide shows it wrapped
// in a one-element "data" array; the endpoint has also answered with the
// object alone. Both are read.
type shortToken struct {
	AccessToken string      `json:"access_token"`
	UserID      json.Number `json:"user_id"`
	Data        []struct {
		AccessToken string      `json:"access_token"`
		UserID      json.Number `json:"user_id"`
	} `json:"data"`
}

func (p *Publisher) Exchange(ctx context.Context, code, _, redirectURL string) (publisher.Connection, error) {
	var short shortToken
	form := url.Values{
		"client_id":     {p.appID},
		"client_secret": {p.appSecret},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURL},
		"code":          {code},
	}
	if err := p.call(ctx, http.MethodPost, exchangeURL, form, "exchange code", &short); err != nil {
		return publisher.Connection{}, err
	}
	if short.AccessToken == "" && len(short.Data) > 0 {
		short.AccessToken = short.Data[0].AccessToken
	}
	if short.AccessToken == "" {
		return publisher.Connection{}, errors.New("instagram: exchange code: no token in the answer")
	}

	long, err := p.token(ctx, graphBase+"/access_token", url.Values{
		"grant_type":    {"ig_exchange_token"},
		"client_secret": {p.appSecret},
		"access_token":  {short.AccessToken},
	}, "long-lived token")
	if err != nil {
		return publisher.Connection{}, err
	}

	// The exchange's user_id is the app-scoped id. Containers are created
	// under the professional account's id, which only the profile gives.
	me, err := p.profile(ctx, long.AccessToken)
	if err != nil {
		return publisher.Connection{}, err
	}
	long.UserID = me.UserID.String()
	name := me.Name
	if me.Username != "" {
		name = "@" + me.Username
	}
	raw, _ := json.Marshal(long)
	return publisher.Connection{DisplayName: name, Credentials: raw, RemoteID: long.UserID}, nil
}

type profile struct {
	UserID   json.Number `json:"user_id"`
	Username string      `json:"username"`
	Name     string      `json:"name"`
}

// profile reads the account behind a token. Like the code exchange, the
// guide shows the answer inside a "data" array and the endpoint has answered
// with the object alone.
func (p *Publisher) profile(ctx context.Context, token string) (profile, error) {
	var out struct {
		profile
		Data []profile `json:"data"`
	}
	q := url.Values{"fields": {"user_id,username,name"}, "access_token": {token}}
	if err := p.call(ctx, http.MethodGet, apiBase+"/me", q, "profile", &out); err != nil {
		return profile{}, err
	}
	me := out.profile
	if me.UserID == "" && len(out.Data) > 0 {
		me = out.Data[0]
	}
	if me.UserID == "" {
		return profile{}, errors.New("instagram: profile: no account id in the answer")
	}
	return me, nil
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
		return credentials{}, fmt.Errorf("instagram: %s: no token in the answer", op)
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
		return nil, publisher.Permanent(errors.New("instagram: stored credentials are unreadable"))
	}
	fresh, err := p.token(ctx, graphBase+"/refresh_access_token", url.Values{
		"grant_type":   {"ig_refresh_token"},
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
		return publisher.Result{}, publisher.Permanent(errors.New("instagram: stored credentials are unreadable"))
	}
	if !creds.ExpiresAt.IsZero() && p.now().After(creds.ExpiresAt) {
		return publisher.Result{}, publisher.Reconnect(ErrExpired)
	}

	var container string
	var err error
	switch len(post.Media) {
	case 0:
		// Validate refuses this at the door; a post that got here anyway
		// must not become a request Instagram answers with a riddle.
		return publisher.Result{}, publisher.Permanent(errors.New("instagram: a post needs a photo or a video"))
	case 1:
		form, ferr := mediaForm(ctx, post.Media[0], media, false)
		if ferr != nil {
			return publisher.Result{}, ferr
		}
		if post.Text != "" {
			form.Set("caption", post.Text)
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

// mediaForm is the container for one file. An image needs no media_type. A
// video on its own is a reel, the only kind of single-video post there is; in
// a carousel it is a VIDEO, because reels cannot appear in carousels.
func mediaForm(ctx context.Context, m publisher.Media, media publisher.MediaSource, inCarousel bool) (url.Values, error) {
	u, err := media.PublicURL(ctx, m.Key, mediaURLTTL)
	if err != nil {
		return nil, fmt.Errorf("instagram: media url: %w", err)
	}
	form := url.Values{}
	switch {
	case !publisher.IsVideo(m.ContentType):
		form.Set("image_url", u)
	case inCarousel:
		form.Set("media_type", "VIDEO")
		form.Set("video_url", u)
	default:
		form.Set("media_type", "REELS")
		form.Set("video_url", u)
	}
	if inCarousel {
		form.Set("is_carousel_item", "true")
	}
	return form, nil
}

// carousel creates a container per file, waits for each, then the carousel.
// Validate has already kept the count within 2 to 20. The caption goes on
// the carousel only: Instagram does not take one on its items.
func (p *Publisher) carousel(ctx context.Context, creds credentials, post publisher.Post, media publisher.MediaSource) (string, error) {
	children := make([]string, 0, len(post.Media))
	for _, m := range post.Media {
		form, err := mediaForm(ctx, m, media, true)
		if err != nil {
			return "", err
		}
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
		form.Set("caption", post.Text)
	}
	return p.create(ctx, creds, form)
}

func (p *Publisher) create(ctx context.Context, creds credentials, form url.Values) (string, error) {
	form.Set("access_token", creds.AccessToken)
	var out struct {
		ID string `json:"id"`
	}
	if err := p.call(ctx, http.MethodPost, apiBase+"/"+url.PathEscape(creds.UserID)+"/media", form, "create container", &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("instagram: create container: no id in the answer")
	}
	return out.ID, nil
}

type containerStatus struct {
	StatusCode string `json:"status_code"`
	// Status is the sentence that goes with ERROR, when Instagram gives one.
	Status string `json:"status"`
}

func (p *Publisher) status(ctx context.Context, creds credentials, container string) (containerStatus, error) {
	var st containerStatus
	q := url.Values{"fields": {"status_code,status"}, "access_token": {creds.AccessToken}}
	err := p.call(ctx, http.MethodGet, apiBase+"/"+url.PathEscape(container), q, "container status", &st)
	return st, err
}

// waitFinished polls a container until Instagram has processed it. Nothing is
// on the profile yet, so every failure here is safe to retry or report.
func (p *Publisher) waitFinished(ctx context.Context, creds credentials, container string) error {
	deadline := p.now().Add(p.processingWait)
	for {
		st, err := p.status(ctx, creds, container)
		if err != nil {
			return err
		}
		switch st.StatusCode {
		case "FINISHED":
			return nil
		case "ERROR":
			msg := st.Status
			if msg == "" {
				msg = "no reason given"
			}
			// The file or its URL, which another attempt would hand over
			// unchanged.
			return publisher.Permanent(fmt.Errorf("instagram: processing failed: %s", msg))
		case "EXPIRED":
			// A container lives 24 hours; a new attempt makes a new one.
			return errors.New("instagram: container expired before it was published")
		}
		if !p.now().Before(deadline) {
			return fmt.Errorf("instagram: still processing after %s", p.processingWait)
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
// against the container before anyone retries: Instagram marks a container
// PUBLISHED once it is live.
func (p *Publisher) publish(ctx context.Context, creds credentials, container string) (publisher.Result, error) {
	form := url.Values{"creation_id": {container}, "access_token": {creds.AccessToken}}
	var out struct {
		ID string `json:"id"`
	}
	err := p.call(ctx, http.MethodPost, apiBase+"/"+url.PathEscape(creds.UserID)+"/media_publish", form, "publish", &out)
	var api *apiErr
	switch {
	case err == nil && out.ID != "":
		return publisher.Result{RemoteID: out.ID, URL: p.permalink(ctx, creds, out.ID)}, nil
	case errors.As(err, &api) && api.status < 500:
		// Instagram answered and said no: nothing was posted.
		return publisher.Result{}, err
	}

	// No answer, a server error, or no id: ask the container. The request's
	// own context may be what ended, so the check gets one of its own.
	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	st, serr := p.status(checkCtx, creds, container)
	switch {
	case serr == nil && st.StatusCode == "PUBLISHED":
		return publisher.Result{}, nil
	case serr == nil && st.StatusCode == "FINISHED":
		// Still waiting to be published after the request ended: it was not.
		if err == nil {
			err = errors.New("instagram: publish answered without an id")
		}
		return publisher.Result{}, err
	default:
		return publisher.Result{}, publisher.Permanent(errors.New("instagram: the publish request failed and Instagram did not confirm whether the post went out; check Instagram before posting again"))
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

// call sends params as the query of a GET or the form body of a POST, and
// decodes a 200 into out.
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
		return fmt.Errorf("instagram: %s: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return apiError(op, resp)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("instagram: %s: unreadable answer: %w", op, err)
	}
	return nil
}

// apiErr is an answer from Instagram, as opposed to no answer at all.
type apiErr struct {
	status int
	err    error
}

func (e *apiErr) Error() string { return e.err.Error() }
func (e *apiErr) Unwrap() error { return e.err }

// Subcodes from the error codes reference that decide what a failure means
// where its code alone does not.
const (
	subDailyLimit    = 2207042 // the account's 100 posts in 24 hours
	subContainerGone = 2207008 // "Temporary error publishing a container"
	subNotReady      = 2207027 // "The media is not ready for publishing"
)

// apiError turns a non-success response into a classified error. Graph
// errors come as {"error": {...}}; the code exchange has its own flat shape.
func apiError(op string, resp *http.Response) error {
	var body struct {
		Error struct {
			Message     string `json:"message"`
			Code        int    `json:"code"`
			Subcode     int    `json:"error_subcode"`
			UserMessage string `json:"error_user_msg"`
		} `json:"error"`
		ErrorMessage string `json:"error_message"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = json.Unmarshal(raw, &body)
	msg := body.Error.UserMessage
	if msg == "" {
		msg = body.Error.Message
	}
	if msg == "" {
		msg = body.ErrorMessage
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	err := fmt.Errorf("instagram: %s: %d: %s", op, resp.StatusCode, msg)

	var classified error
	switch code, sub := body.Error.Code, body.Error.Subcode; {
	case sub == subDailyLimit:
		// The limit is a moving 24 hours; asking again every hour finds the
		// first slot that opens.
		classified = publisher.RetryAfter(time.Hour, err)
	case resp.StatusCode == http.StatusTooManyRequests, code == 4 && sub == 0, code == 17, code == 32, code == 613:
		// Request rate limits. Meta sends no Retry-After; the limits are
		// rolling, so an hour is a fair wait. Code 4 with a subcode is
		// Instagram's spam restriction, which waiting does not lift.
		classified = publisher.RetryAfter(time.Hour, err)
	case resp.StatusCode == http.StatusUnauthorized, code == 190:
		// Expired or revoked token: only the user reconnecting fixes it.
		classified = publisher.Reconnect(fmt.Errorf("%w (reconnect the account)", err))
	case resp.StatusCode >= 500, code == 1, code == 2, code == -1, code == -2, sub == subContainerGone, sub == subNotReady:
		// Instagram's own trouble, a slow download, or a container that is
		// not ready or was lost: a new attempt makes a new container.
		classified = err
	default:
		classified = publisher.Permanent(err)
	}
	return &apiErr{status: resp.StatusCode, err: classified}
}
