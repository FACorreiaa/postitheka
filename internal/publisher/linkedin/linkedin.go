// Package linkedin publishes to a member's own LinkedIn profile.
//
// Access is the self-serve "Share on LinkedIn" product (w_member_social) plus
// "Sign In with LinkedIn using OpenID Connect" (openid, profile) to learn who
// connected. Neither needs review. Company Pages need the Community Management
// API, which does, and are not supported.
//
// Tokens last 60 days and LinkedIn issues refresh tokens only to selected
// partners, so an expired account is renewed by connecting it again.
//
// References (checked 2026-10-23):
// https://learn.microsoft.com/en-us/linkedin/shared/authentication/authorization-code-flow
// https://learn.microsoft.com/en-us/linkedin/marketing/community-management/shares/posts-api
// https://learn.microsoft.com/en-us/linkedin/marketing/community-management/shares/images-api
// https://learn.microsoft.com/en-us/linkedin/marketing/community-management/shares/videos-api
// https://learn.microsoft.com/en-us/linkedin/marketing/community-management/shares/little-text-format
package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/oauth2"

	"github.com/FACorreiaa/postitheka/internal/publisher"
)

const (
	authorizeURL = "https://www.linkedin.com/oauth/v2/authorization"
	tokenURL     = "https://www.linkedin.com/oauth/v2/accessToken"
	// apiBase is fixed: stored credentials can only ever be sent here. Tests
	// redirect the transport instead.
	apiBase = "https://api.linkedin.com"

	// version is the Linkedin-Version every /rest call sends. LinkedIn
	// supports a version for at least a year and then answers 426, so this
	// is bumped by hand well before then (202610 sunsets around 2027-10).
	version = "202610"
)

var scopes = []string{"openid", "profile", "w_member_social"}

// ErrExpired is an access token past its 60 days. Only reconnecting fixes it.
var ErrExpired = errors.New("linkedin: access expired; reconnect the account")

type Publisher struct {
	client *http.Client
	oauth  oauth2.Config
	// processingWait bounds how long a video may process before posting.
	processingWait time.Duration
	pollEvery      time.Duration
	now            func() time.Time
}

// New returns a Publisher for the developer app with this client id and
// secret. A nil client gets one with a timeout long enough for a 4 MB video
// part on a slow link.
func New(clientID, clientSecret string, client *http.Client) *Publisher {
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Publisher{
		client: client,
		oauth: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       scopes,
			// LinkedIn wants the client credentials in the form body.
			Endpoint: oauth2.Endpoint{AuthURL: authorizeURL, TokenURL: tokenURL, AuthStyle: oauth2.AuthStyleInParams},
		},
		processingWait: 3 * time.Minute,
		pollEvery:      5 * time.Second,
		now:            time.Now,
	}
}

func (p *Publisher) Platform() publisher.Platform { return publisher.LinkedIn }

// Rules for a member post, checked 2026-10-23. The API reference gives image
// formats and pixel count but no length or byte limits; those come from
// LinkedIn's Help Center: 3,000 characters a post (answer a528176), photos up
// to 5 MB and 20 to a post (a527229). Video is MP4, 3 seconds to 30 minutes,
// 75 KB to 500 MB, per the Videos API (the same page also says 5 GB, under an
// ads heading; 500 MB is the floor). A post is one image, 2 to 20 images, or
// one video. Duration is not checked until Rules can express it (VID-001).
func (p *Publisher) Rules() publisher.Rules {
	return publisher.Rules{
		MaxTextLength: 3000,
		MaxMedia:      20,
		MaxMediaBytes: 5 << 20,
		MaxVideoBytes: 500 << 20,
		MediaTypes:    []string{"image/jpeg", "image/png", "image/gif", "video/mp4"},
		VideoAlone:    true,
	}
}

// credentials is what is sealed for an account.
type credentials struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	// Person is the member's URN, urn:li:person:{id}: the author of every
	// post and the owner of every upload.
	Person string `json:"person"`
}

// AuthorizeURL ignores the verifier: LinkedIn offers PKCE only to native
// apps, on request. A server app proves itself with the client secret.
func (p *Publisher) AuthorizeURL(state, _, redirectURL string) string {
	cfg := p.oauth
	cfg.RedirectURL = redirectURL
	return cfg.AuthCodeURL(state)
}

func (p *Publisher) Exchange(ctx context.Context, code, _, redirectURL string) (publisher.Connection, error) {
	cfg := p.oauth
	cfg.RedirectURL = redirectURL
	tok, err := cfg.Exchange(context.WithValue(ctx, oauth2.HTTPClient, p.client), code)
	if err != nil {
		return publisher.Connection{}, fmt.Errorf("linkedin: exchange code: %w", err)
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/v2/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	resp, err := p.client.Do(req)
	if err != nil {
		return publisher.Connection{}, fmt.Errorf("linkedin: userinfo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return publisher.Connection{}, apiError("userinfo", resp)
	}
	var me struct {
		Sub  string `json:"sub"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil || me.Sub == "" {
		return publisher.Connection{}, fmt.Errorf("linkedin: userinfo has no member id: %v", err)
	}

	expires := tok.Expiry
	if expires.IsZero() {
		// LinkedIn always sends expires_in; this is the documented lifespan
		// should it ever not.
		expires = p.now().Add(60 * 24 * time.Hour)
	}
	raw, _ := json.Marshal(credentials{AccessToken: tok.AccessToken, ExpiresAt: expires, Person: "urn:li:person:" + me.Sub})
	return publisher.Connection{DisplayName: me.Name, Credentials: raw, RemoteID: me.Sub}, nil
}

func (p *Publisher) Publish(ctx context.Context, raw []byte, post publisher.Post, media publisher.MediaSource) (publisher.Result, error) {
	var creds credentials
	if err := json.Unmarshal(raw, &creds); err != nil || creds.AccessToken == "" || creds.Person == "" {
		return publisher.Result{}, publisher.Permanent(errors.New("linkedin: stored credentials are unreadable"))
	}
	if !creds.ExpiresAt.IsZero() && p.now().After(creds.ExpiresAt) {
		return publisher.Result{}, publisher.Permanent(ErrExpired)
	}

	body := map[string]any{
		"author":     creds.Person,
		"commentary": escapeText(post.Text),
		"visibility": "PUBLIC",
		"distribution": map[string]any{
			"feedDistribution":               "MAIN_FEED",
			"targetEntities":                 []any{},
			"thirdPartyDistributionChannels": []any{},
		},
		"lifecycleState":            "PUBLISHED",
		"isReshareDisabledByAuthor": false,
	}

	switch {
	case len(post.Media) == 1 && publisher.IsVideo(post.Media[0].ContentType):
		urn, err := p.uploadVideo(ctx, creds, post.Media[0], media)
		if err != nil {
			return publisher.Result{}, err
		}
		body["content"] = map[string]any{"media": map[string]any{"id": urn}}
	case len(post.Media) == 1:
		urn, err := p.uploadImage(ctx, creds, post.Media[0], media)
		if err != nil {
			return publisher.Result{}, err
		}
		body["content"] = map[string]any{"media": map[string]any{"id": urn}}
	case len(post.Media) > 1:
		images := make([]map[string]any, 0, len(post.Media))
		for _, m := range post.Media {
			urn, err := p.uploadImage(ctx, creds, m, media)
			if err != nil {
				return publisher.Result{}, err
			}
			images = append(images, map[string]any{"id": urn})
		}
		body["content"] = map[string]any{"multiImage": map[string]any{"images": images}}
	}

	resp, err := p.rest(ctx, creds, http.MethodPost, "/rest/posts", body)
	if err != nil {
		// Unclassified, so it is retried. A timeout after LinkedIn created
		// the post would post it twice; the Posts API takes no idempotency
		// key.
		return publisher.Result{}, fmt.Errorf("linkedin: create post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		return publisher.Result{}, apiError("create post", resp)
	}
	// LinkedIn said 201, so the post exists even if the id is missing.
	urn := resp.Header.Get("x-restli-id")
	res := publisher.Result{RemoteID: urn}
	if urn != "" {
		res.URL = "https://www.linkedin.com/feed/update/" + urn + "/"
	}
	return res, nil
}

func (p *Publisher) uploadImage(ctx context.Context, creds credentials, m publisher.Media, media publisher.MediaSource) (string, error) {
	resp, err := p.rest(ctx, creds, http.MethodPost, "/rest/images?action=initializeUpload",
		map[string]any{"initializeUploadRequest": map[string]any{"owner": creds.Person}})
	if err != nil {
		return "", fmt.Errorf("linkedin: initialize image upload: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", apiError("initialize image upload", resp)
	}
	var init struct {
		Value struct {
			UploadURL string `json:"uploadUrl"`
			Image     string `json:"image"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&init); err != nil || init.Value.Image == "" {
		return "", fmt.Errorf("linkedin: unreadable image upload instructions: %v", err)
	}

	f, err := media.Open(ctx, m.Key)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	// The images API reference shows the PUT with the member's token; the
	// host check in put keeps that token on LinkedIn's own hosts.
	if _, err := p.put(ctx, init.Value.UploadURL, creds.AccessToken, f, m.Size, m.ContentType); err != nil {
		return "", err
	}
	return init.Value.Image, nil
}

func (p *Publisher) uploadVideo(ctx context.Context, creds credentials, m publisher.Media, media publisher.MediaSource) (string, error) {
	resp, err := p.rest(ctx, creds, http.MethodPost, "/rest/videos?action=initializeUpload", map[string]any{
		"initializeUploadRequest": map[string]any{
			"owner":           creds.Person,
			"fileSizeBytes":   m.Size,
			"uploadCaptions":  false,
			"uploadThumbnail": false,
		},
	})
	if err != nil {
		return "", fmt.Errorf("linkedin: initialize video upload: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", apiError("initialize video upload", resp)
	}
	var init struct {
		Value struct {
			Video        string `json:"video"`
			UploadToken  string `json:"uploadToken"`
			Instructions []struct {
				UploadURL string `json:"uploadUrl"`
				FirstByte int64  `json:"firstByte"`
				LastByte  int64  `json:"lastByte"`
			} `json:"uploadInstructions"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&init); err != nil || init.Value.Video == "" || len(init.Value.Instructions) == 0 {
		return "", fmt.Errorf("linkedin: unreadable video upload instructions: %v", err)
	}

	f, err := media.Open(ctx, m.Key)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	// Parts arrive in file order, each a byte range of LinkedIn's choosing
	// (4 MB). Reading them in order streams the file once, never whole.
	var offset int64
	etags := make([]string, 0, len(init.Value.Instructions))
	for _, in := range init.Value.Instructions {
		if in.FirstByte != offset || in.LastByte < in.FirstByte {
			return "", fmt.Errorf("linkedin: upload part %d-%d does not follow byte %d", in.FirstByte, in.LastByte, offset)
		}
		n := in.LastByte - in.FirstByte + 1
		etag, err := p.put(ctx, in.UploadURL, "", io.LimitReader(f, n), n, "application/octet-stream")
		if err != nil {
			return "", err
		}
		if etag == "" {
			return "", errors.New("linkedin: upload part answered without an ETag")
		}
		etags = append(etags, etag)
		offset += n
	}
	if offset != m.Size {
		return "", publisher.Permanent(fmt.Errorf("linkedin: upload instructions cover %d of %d bytes", offset, m.Size))
	}

	fin, err := p.rest(ctx, creds, http.MethodPost, "/rest/videos?action=finalizeUpload", map[string]any{
		"finalizeUploadRequest": map[string]any{
			"video":           init.Value.Video,
			"uploadToken":     init.Value.UploadToken,
			"uploadedPartIds": etags,
		},
	})
	if err != nil {
		return "", fmt.Errorf("linkedin: finalize video upload: %w", err)
	}
	defer func() { _ = fin.Body.Close() }()
	if fin.StatusCode != http.StatusOK {
		return "", apiError("finalize video upload", fin)
	}

	return init.Value.Video, p.waitForVideo(ctx, creds, init.Value.Video)
}

// waitForVideo polls until LinkedIn has processed the video: posting one that
// is still processing fails with MEDIA_ASSET_WAITING_UPLOAD. Running out of
// time is retried, which uploads the video again; nothing was posted yet.
func (p *Publisher) waitForVideo(ctx context.Context, creds credentials, urn string) error {
	deadline := p.now().Add(p.processingWait)
	for {
		resp, err := p.rest(ctx, creds, http.MethodGet, "/rest/videos/"+url.PathEscape(urn), nil)
		if err != nil {
			return fmt.Errorf("linkedin: video status: %w", err)
		}
		var v struct {
			Status        string `json:"status"`
			FailureReason string `json:"processingFailureReason"`
		}
		ok := resp.StatusCode == http.StatusOK
		var apiErr error
		if ok {
			_ = json.NewDecoder(resp.Body).Decode(&v)
		} else {
			apiErr = apiError("video status", resp)
		}
		_ = resp.Body.Close()
		if apiErr != nil {
			return apiErr
		}

		switch v.Status {
		case "AVAILABLE":
			return nil
		case "PROCESSING_FAILED":
			return publisher.Permanent(fmt.Errorf("linkedin: video processing failed: %s", v.FailureReason))
		}
		if p.now().After(deadline) {
			return fmt.Errorf("linkedin: video still %s after %s", v.Status, p.processingWait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.pollEvery):
		}
	}
}

// rest calls a versioned endpoint. body, when not nil, is sent as JSON.
func (p *Publisher) rest(ctx context.Context, creds credentials, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Linkedin-Version", version)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return p.client.Do(req)
}

// put uploads bytes to a URL LinkedIn handed out and returns its ETag. The URL
// came from LinkedIn's response, so it is checked before anything, the token
// above all, is sent to it.
func (p *Publisher) put(ctx context.Context, rawURL, token string, body io.Reader, size int64, contentType string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || (u.Hostname() != "linkedin.com" && !strings.HasSuffix(u.Hostname(), ".linkedin.com")) {
		return "", publisher.Permanent(fmt.Errorf("linkedin: refusing upload URL on %q", u.Hostname()))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, body)
	if err != nil {
		return "", err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", contentType)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("linkedin: upload: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return "", apiError("upload", resp)
	}
	return resp.Header.Get("ETag"), nil
}

// apiError turns a non-success response into a classified error.
func apiError(op string, resp *http.Response) error {
	var body struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = json.Unmarshal(raw, &body)
	msg := body.Message
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	err := fmt.Errorf("linkedin: %s: %d: %s", op, resp.StatusCode, msg)

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return publisher.RetryAfter(retryAfter(resp, time.Now()), err)
	case resp.StatusCode >= 500:
		return err
	case resp.StatusCode == http.StatusUnauthorized:
		// Expired or revoked: only the member reconnecting fixes it.
		return publisher.Permanent(fmt.Errorf("%w (reconnect the account)", err))
	case resp.StatusCode == 426:
		// The pinned Linkedin-Version was sunset: a code change, not a retry.
		return publisher.Permanent(fmt.Errorf("%w (Linkedin-Version %s is no longer served)", err, version))
	case strings.Contains(msg, "MEDIA_ASSET_WAITING_UPLOAD") || body.Code == "MEDIA_ASSET_WAITING_UPLOAD":
		// An upload LinkedIn has not finished taking in; worth another go.
		return err
	default:
		return publisher.Permanent(err)
	}
}

// retryAfter honours Retry-After when LinkedIn sends one. Its documented
// limits are daily and reset at midnight UTC, so without a header that is
// when trying again can work.
func retryAfter(resp *http.Response, now time.Time) time.Duration {
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
		return time.Duration(s) * time.Second
	}
	midnight := now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	return midnight.Sub(now) + time.Minute
}

// escapeText makes post text literal in LinkedIn's "little" format, where
// | { } @ [ ] ( ) < > # \ * _ ~ are markup and must be backslash-escaped, or
// the post fails or renders as something else. A # directly followed by a
// letter or digit is left alone, so hashtags written as text stay hashtags.
func escapeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	runes := []rune(s)
	for i, r := range runes {
		switch r {
		case '#':
			if i+1 < len(runes) && (unicode.IsLetter(runes[i+1]) || unicode.IsDigit(runes[i+1])) {
				b.WriteRune(r)
				continue
			}
			b.WriteString(`\#`)
		case '|', '{', '}', '@', '[', ']', '(', ')', '<', '>', '\\', '*', '_', '~':
			b.WriteRune('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
