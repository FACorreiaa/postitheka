// Package facebook publishes to Facebook Pages. Personal profiles cannot be
// posted to through any API.
//
// Access is Facebook Login on the Meta app itself (pages_show_list,
// pages_read_engagement, pages_manage_posts), so the app id and secret are
// the Meta app's own, unlike Threads and Instagram. Before App Review only
// people with a role on the app can connect.
//
// One login can hold several Pages: Facebook's consent dialog asks which
// Pages to share, and every Page shared becomes an account of its own
// (publisher.MultiConnector). There is no Page picker here because that
// dialog is one.
//
// A connection exchanges the short-lived user token for a long-lived one and
// asks for the Pages with it. A Page token got that way has no expiry date,
// so there is nothing to refresh; it dies when the person changes their
// password, loses their role on the Page, or removes the app.
//
// Three kinds of post: text to /{page}/feed, photos to /{page}/photos by URL
// (several are uploaded unpublished and attached to one feed post), and one
// video to /{page}/videos on the video host, also by URL. Nothing is
// uploaded from here.
//
// References (checked 2026-11-15, rendered pages; plain fetches get a shell):
// https://developers.facebook.com/docs/facebook-login/guides/advanced/manual-flow
// https://developers.facebook.com/docs/facebook-login/guides/access-tokens/get-long-lived
// https://developers.facebook.com/docs/pages-api/posts
// https://developers.facebook.com/docs/graph-api/reference/page/photos/
// https://developers.facebook.com/docs/graph-api/reference/page/videos/
// https://developers.facebook.com/docs/video-api/guides/publishing
package facebook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FACorreiaa/postitheka/internal/publisher"
)

const (
	version      = "v27.0"
	authorizeURL = "https://www.facebook.com/" + version + "/dialog/oauth"
	// Both hosts are fixed: stored tokens can only ever be sent here. Tests
	// redirect the transport instead.
	apiBase   = "https://graph.facebook.com/" + version
	videoBase = "https://graph-video.facebook.com/" + version

	// The three every reference page asks for. The Pages guide lists more
	// (pages_manage_engagement, pages_read_user_engagement, publish_video);
	// the photo and video references, which describe the calls made here, do
	// not, and each extra permission is one more thing App Review must grant.
	scopes = "pages_show_list,pages_read_engagement,pages_manage_posts"

	// mediaURLTTL covers Facebook fetching the file, which for a video can
	// trail the request by minutes.
	mediaURLTTL = time.Hour

	requestTimeout = 30 * time.Second
	// videoTimeout is longer because Facebook may fetch the whole file
	// before it answers. The job has 5 minutes in all.
	videoTimeout = 4 * time.Minute

	// maxPageRequests bounds how many Pages one login can connect: 4
	// requests of 100. Someone with more than 400 Pages is not who this is for.
	maxPageRequests = 4
)

type Publisher struct {
	client    *http.Client
	appID     string
	appSecret string
}

// New returns a Publisher for the Meta app with this id and secret. A nil
// client gets a default; every request carries its own deadline.
func New(appID, appSecret string, client *http.Client) *Publisher {
	if client == nil {
		client = &http.Client{}
	}
	return &Publisher{client: client, appID: appID, appSecret: appSecret}
}

func (p *Publisher) Platform() publisher.Platform { return publisher.Facebook }

// Rules for a Page post, checked 2026-11-15.
//
// From Meta's reference: photos are JPEG, PNG, GIF, BMP or TIFF and at most
// 10 MB ("we recommend not exceeding 1MB" for PNG is advice, not a limit).
// BMP and TIFF are left out: nothing else here takes them.
//
// Not stated anywhere in the reference, so these are Postitheka's own caps,
// set low enough to be safe, and not Facebook's limits: the text length, the
// number of photos in one post, and a video's size and formats. The only
// video type Meta names is MP4, in its upload guide. Raise them only against
// a Meta page that says so, or after a real post proves it.
//
// A video is posted alone: a Page post holds photos or one video.
func (p *Publisher) Rules() publisher.Rules {
	return publisher.Rules{
		MaxTextLength: 5000,
		MaxMedia:      10,
		MaxMediaBytes: 10_000_000,
		MaxVideoBytes: 300_000_000,
		MediaTypes:    []string{"image/jpeg", "image/png", "image/gif", "video/mp4"},
		VideoAlone:    true,
	}
}

// credentials is what is sealed for an account: one Page and its token.
type credentials struct {
	AccessToken string `json:"access_token"`
	PageID      string `json:"page_id"`
}

func (c credentials) valid() bool { return c.AccessToken != "" && c.PageID != "" }

// AuthorizeURL ignores the verifier: Facebook's server-side flow proves the
// app with its secret, and the state the caller generated guards the
// callback.
func (p *Publisher) AuthorizeURL(state, _, redirectURL string) string {
	return authorizeURL + "?" + url.Values{
		"client_id":     {p.appID},
		"redirect_uri":  {redirectURL},
		"scope":         {scopes},
		"response_type": {"code"},
		"state":         {state},
	}.Encode()
}

// ErrNoPages is a login that shared no Page the person can post to.
var ErrNoPages = errors.New("facebook: the login shared no Page this person can post to")

// Exchange connects the first Page the login shared. The service uses
// ExchangeAll; this is here so the type is a whole OAuthConnector.
func (p *Publisher) Exchange(ctx context.Context, code, verifier, redirectURL string) (publisher.Connection, error) {
	conns, err := p.ExchangeAll(ctx, code, verifier, redirectURL)
	if err != nil {
		return publisher.Connection{}, err
	}
	return conns[0], nil
}

// ExchangeAll trades the code for one connection per Page the person shared
// in Facebook's dialog and can create content on.
func (p *Publisher) ExchangeAll(ctx context.Context, code, _, redirectURL string) ([]publisher.Connection, error) {
	short, err := p.token(ctx, url.Values{
		"client_id":     {p.appID},
		"client_secret": {p.appSecret},
		"redirect_uri":  {redirectURL},
		"code":          {code},
	}, "exchange code")
	if err != nil {
		return nil, err
	}
	// Page tokens asked for with a short-lived user token are short-lived
	// too. Only a long-lived user token yields Page tokens that last.
	long, err := p.token(ctx, url.Values{
		"grant_type":        {"fb_exchange_token"},
		"client_id":         {p.appID},
		"client_secret":     {p.appSecret},
		"fb_exchange_token": {short},
	}, "long-lived token")
	if err != nil {
		return nil, err
	}

	var conns []publisher.Connection
	after := ""
	for range maxPageRequests {
		var out struct {
			Data []struct {
				ID          string   `json:"id"`
				Name        string   `json:"name"`
				AccessToken string   `json:"access_token"`
				Tasks       []string `json:"tasks"`
			} `json:"data"`
			Paging struct {
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
				Next string `json:"next"`
			} `json:"paging"`
		}
		q := url.Values{"fields": {"id,name,access_token,tasks"}, "limit": {"100"}, "access_token": {long}}
		if after != "" {
			q.Set("after", after)
		}
		if err := p.call(ctx, http.MethodGet, apiBase+"/me/accounts", q, "list pages", requestTimeout, &out); err != nil {
			return nil, err
		}
		for _, pg := range out.Data {
			// A Page the person only moderates or analyses cannot be posted
			// to. An answer without tasks is taken at its word.
			if pg.ID == "" || pg.AccessToken == "" || (len(pg.Tasks) > 0 && !slices.Contains(pg.Tasks, "CREATE_CONTENT")) {
				continue
			}
			raw, _ := json.Marshal(credentials{AccessToken: pg.AccessToken, PageID: pg.ID})
			conns = append(conns, publisher.Connection{DisplayName: pg.Name, Credentials: raw, RemoteID: pg.ID})
		}
		// The cursor is followed, never paging.next: that is a whole URL
		// with the token in it, and it names its own host.
		after = out.Paging.Cursors.After
		if out.Paging.Next == "" || after == "" {
			break
		}
	}
	if len(conns) == 0 {
		return nil, ErrNoPages
	}
	return conns, nil
}

func (p *Publisher) token(ctx context.Context, q url.Values, op string) (string, error) {
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := p.call(ctx, http.MethodGet, apiBase+"/oauth/access_token", q, op, requestTimeout, &tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("facebook: %s: no token in the answer", op)
	}
	return tok.AccessToken, nil
}

func (p *Publisher) Publish(ctx context.Context, raw []byte, post publisher.Post, media publisher.MediaSource) (publisher.Result, error) {
	var creds credentials
	if err := json.Unmarshal(raw, &creds); err != nil || !creds.valid() {
		return publisher.Result{}, publisher.Permanent(errors.New("facebook: stored credentials are unreadable"))
	}
	page := "/" + url.PathEscape(creds.PageID)

	switch {
	case len(post.Media) == 0:
		form := url.Values{"message": {post.Text}}
		return p.post(ctx, creds, apiBase+page+"/feed", form, "publish post", requestTimeout)

	case publisher.IsVideo(post.Media[0].ContentType):
		// Validate keeps a video alone; a post that got here with more would
		// silently lose the rest.
		if len(post.Media) > 1 {
			return publisher.Result{}, publisher.Permanent(errors.New("facebook: a video must be the only file in the post"))
		}
		u, err := media.PublicURL(ctx, post.Media[0].Key, mediaURLTTL)
		if err != nil {
			return publisher.Result{}, fmt.Errorf("facebook: media url: %w", err)
		}
		form := url.Values{"file_url": {u}}
		if post.Text != "" {
			form.Set("description", post.Text)
		}
		res, err := p.post(ctx, creds, videoBase+page+"/videos", form, "publish video", videoTimeout)
		// The answer is a video id, and Meta documents no address for one:
		// better no link than one that might not open.
		res.URL = ""
		return res, err

	case len(post.Media) == 1:
		u, err := media.PublicURL(ctx, post.Media[0].Key, mediaURLTTL)
		if err != nil {
			return publisher.Result{}, fmt.Errorf("facebook: media url: %w", err)
		}
		form := url.Values{"url": {u}}
		if post.Text != "" {
			form.Set("caption", post.Text)
		}
		return p.post(ctx, creds, apiBase+page+"/photos", form, "publish photo", requestTimeout)

	default:
		return p.album(ctx, creds, page, post, media)
	}
}

// album uploads each photo unpublished, then publishes one feed post that
// attaches them. An unpublished photo is on nobody's timeline and Facebook
// deletes it after about a day, so every upload is safe to retry.
func (p *Publisher) album(ctx context.Context, creds credentials, page string, post publisher.Post, media publisher.MediaSource) (publisher.Result, error) {
	form := url.Values{}
	if post.Text != "" {
		form.Set("message", post.Text)
	}
	for i, m := range post.Media {
		if publisher.IsVideo(m.ContentType) {
			return publisher.Result{}, publisher.Permanent(errors.New("facebook: a video must be the only file in the post"))
		}
		u, err := media.PublicURL(ctx, m.Key, mediaURLTTL)
		if err != nil {
			return publisher.Result{}, fmt.Errorf("facebook: media url: %w", err)
		}
		var out struct {
			ID string `json:"id"`
		}
		upload := url.Values{"url": {u}, "published": {"false"}, "access_token": {creds.AccessToken}}
		if err := p.call(ctx, http.MethodPost, apiBase+page+"/photos", upload, "upload photo", requestTimeout, &out); err != nil {
			return publisher.Result{}, err
		}
		if out.ID == "" {
			return publisher.Result{}, errors.New("facebook: upload photo: no id in the answer")
		}
		ref, _ := json.Marshal(map[string]string{"media_fbid": out.ID})
		form.Set("attached_media["+strconv.Itoa(i)+"]", string(ref))
	}
	return p.post(ctx, creds, apiBase+page+"/feed", form, "publish post", requestTimeout)
}

// post sends a request that puts something on the Page. Facebook offers no
// idempotency key and no way to ask afterwards whether a request landed, so
// a failure without a clear refusal is not retried: the post may be live,
// and a second one would be a duplicate on someone's Page.
func (p *Publisher) post(ctx context.Context, creds credentials, endpoint string, form url.Values, op string, timeout time.Duration) (publisher.Result, error) {
	form.Set("access_token", creds.AccessToken)
	var out struct {
		ID     string `json:"id"`
		PostID string `json:"post_id"`
	}
	err := p.call(ctx, http.MethodPost, endpoint, form, op, timeout, &out)
	var api *apiErr
	switch {
	case err == nil && out.ID != "":
		id := out.ID
		if out.PostID != "" {
			id = out.PostID // a photo answers with its own id and the post's
		}
		// "The URL, or permalink, for a Page post is
		// https://www.facebook.com/page_post_id" (Pages API, Posts).
		return publisher.Result{RemoteID: id, URL: "https://www.facebook.com/" + url.PathEscape(id)}, nil
	case errors.As(err, &api) && api.refused:
		// Facebook answered and said no: nothing was posted.
		return publisher.Result{}, err
	default:
		return publisher.Result{}, publisher.Permanent(fmt.Errorf("facebook: %s: the request failed and Facebook did not confirm whether the post went out; check the Page before posting again", op))
	}
}

// call sends params as the query of a GET or the form body of a POST, and
// decodes a 200 into out.
//
// The access token and app secret travel in those params, so no error from
// here may carry the request URL: net/http puts the whole URL, query and
// all, into a transport error, and errors end up in logs and in last_error.
func (p *Publisher) call(ctx context.Context, method, endpoint string, params url.Values, op string, timeout time.Duration, out any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
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
		return fmt.Errorf("facebook: %s: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return apiError(op, resp)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("facebook: %s: unreadable answer: %w", op, err)
	}
	return nil
}

// apiErr is an answer from Facebook, as opposed to no answer at all.
type apiErr struct {
	// refused: Facebook understood the request and declined it, so nothing
	// was created. A 5xx or an answer that is not a Graph error is not that.
	refused bool
	err     error
}

func (e *apiErr) Error() string { return e.err.Error() }
func (e *apiErr) Unwrap() error { return e.err }

// Error codes from the Page Photos and Page Videos references.
const (
	codeBadToken     = 190
	codeFetchFailed  = 389  // "Unable to fetch video file from URL."
	codeUploadFailed = 6001 // "There was a problem uploading your video. Please try again."
)

// apiError turns a non-success response into a classified error.
func apiError(op string, resp *http.Response) error {
	var body struct {
		Error *struct {
			Message     string `json:"message"`
			Code        int    `json:"code"`
			UserMessage string `json:"error_user_msg"`
		} `json:"error"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = json.Unmarshal(raw, &body)
	msg, code := http.StatusText(resp.StatusCode), 0
	if body.Error != nil {
		code = body.Error.Code
		if body.Error.UserMessage != "" {
			msg = body.Error.UserMessage
		} else if body.Error.Message != "" {
			msg = body.Error.Message
		}
	}
	err := fmt.Errorf("facebook: %s: %d: %s", op, resp.StatusCode, msg)
	refused := body.Error != nil && resp.StatusCode < 500

	var classified error
	switch {
	case resp.StatusCode == http.StatusTooManyRequests, code == 4, code == 17, code == 32, code == 613:
		// Rate limits. Meta sends no Retry-After; the limits are rolling,
		// so an hour is a fair wait.
		refused = true
		classified = publisher.RetryAfter(time.Hour, err)
	case resp.StatusCode == http.StatusUnauthorized, code == codeBadToken:
		// The token is dead: a changed password, a lost role on the Page,
		// the app removed. Only connecting again fixes it.
		classified = publisher.Reconnect(fmt.Errorf("%w (reconnect the Page)", err))
	case resp.StatusCode >= 500, code == 1, code == 2, code == codeFetchFailed, code == codeUploadFailed:
		// Facebook's own trouble, or a file it could not fetch this time.
		// "An unknown error occurred" (1) and "service unavailable" (2) do
		// not say whether the request was carried out.
		if code == 1 || code == 2 {
			refused = false
		}
		classified = err
	default:
		classified = publisher.Permanent(err)
	}
	return &apiErr{refused: refused, err: classified}
}
