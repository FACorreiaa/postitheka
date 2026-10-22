// Package discord publishes to a Discord channel through an incoming webhook.
//
// A webhook needs no bot, no OAuth and no app review: the URL is the whole
// credential, which is also why it is sealed before it is stored. Anyone
// holding it can post to the channel.
//
// Reference: https://discord.com/developers/docs/resources/webhook
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/FACorreiaa/postitheka/internal/publisher"
)

// apiBase is fixed rather than taken from the stored URL, so a stored
// credential can only ever send requests to Discord. Tests redirect the
// transport instead of changing this.
const apiBase = "https://discord.com/api/v10"

var ErrInvalidWebhook = errors.New("discord: not a valid webhook URL")

// webhookHosts are the hostnames Discord hands out webhook URLs on. The check
// is what stops a user from pointing the server at an arbitrary URL and
// using it as a request proxy.
var webhookHosts = map[string]bool{
	"discord.com":        true,
	"discordapp.com":     true,
	"canary.discord.com": true,
	"ptb.discord.com":    true,
}

var webhookPath = regexp.MustCompile(`^/api(?:/v\d+)?/webhooks/(\d+)/([A-Za-z0-9_-]+)/?$`)

type Publisher struct {
	client *http.Client
}

// New returns a Publisher. A nil client gets one with a timeout: twenty
// megabytes per file to Discord is slow on a bad day, but not minutes.
func New(client *http.Client) *Publisher {
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	return &Publisher{client: client}
}

func (p *Publisher) Platform() publisher.Platform { return publisher.Discord }

// Rules for webhook messages, checked against Discord's API reference
// (2026-10-22): 2,000 characters of content, up to ten attachments, and a
// 20 MiB limit that applies to each file rather than to the request. Boosted
// servers allow more; this uses the floor every server has.
//
// Discord accepts any file type. The list is the types it previews inline,
// which is what a cross-post is for.
func (p *Publisher) Rules() publisher.Rules {
	return publisher.Rules{
		MaxTextLength: 2000,
		MaxMedia:      10,
		MaxMediaBytes: 20 << 20,
		MediaTypes: []string{
			"image/jpeg", "image/png", "image/gif", "image/webp",
			"video/mp4", "video/quicktime",
		},
	}
}

// credentials is what a Discord account stores. The guild and channel are
// kept so a published message can be linked to.
type credentials struct {
	WebhookID string `json:"webhook_id"`
	Token     string `json:"token"`
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
}

func (c credentials) endpoint() string {
	return apiBase + "/webhooks/" + c.WebhookID + "/" + c.Token
}

// ParseWebhookURL extracts the id and token from a webhook URL, accepting only
// URLs Discord itself issues.
func ParseWebhookURL(raw string) (id, token string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || !webhookHosts[strings.ToLower(u.Host)] {
		return "", "", ErrInvalidWebhook
	}
	m := webhookPath.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", ErrInvalidWebhook
	}
	return m[1], m[2], nil
}

// Connect checks a webhook URL against Discord and returns what to store. A
// URL for a deleted webhook is refused here rather than at the first post.
func (p *Publisher) Connect(ctx context.Context, rawURL string) (publisher.Connection, error) {
	id, token, err := ParseWebhookURL(rawURL)
	if err != nil {
		return publisher.Connection{}, err
	}
	creds := credentials{WebhookID: id, Token: token}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, creds.endpoint(), nil)
	if err != nil {
		return publisher.Connection{}, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return publisher.Connection{}, fmt.Errorf("discord: get webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound {
		return publisher.Connection{}, ErrInvalidWebhook
	}
	if resp.StatusCode != http.StatusOK {
		return publisher.Connection{}, apiError(resp)
	}

	var hook struct {
		Name      string `json:"name"`
		GuildID   string `json:"guild_id"`
		ChannelID string `json:"channel_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&hook); err != nil {
		return publisher.Connection{}, fmt.Errorf("discord: decode webhook: %w", err)
	}
	creds.GuildID, creds.ChannelID = hook.GuildID, hook.ChannelID

	blob, err := json.Marshal(creds)
	if err != nil {
		return publisher.Connection{}, err
	}
	return publisher.Connection{DisplayName: hook.Name, Credentials: blob}, nil
}

type attachment struct {
	ID       int    `json:"id"`
	Filename string `json:"filename"`
}

type payload struct {
	Content string `json:"content,omitempty"`
	// Mentions are switched off. A cross-post written for five platforms
	// should not ping a whole server because its text happened to contain
	// "@everyone"; Discord's own docs recommend this for user-written text.
	AllowedMentions struct {
		Parse []string `json:"parse"`
	} `json:"allowed_mentions"`
	Attachments []attachment `json:"attachments,omitempty"`
}

// Publish executes the webhook with ?wait=true, which makes Discord return the
// created message, or an error if it was not saved. Without it a rejected
// message still answers 204 and the failure is invisible.
//
// Forum and media channels are not supported yet: they need a thread_name,
// and Discord rejects the request with a 400, which surfaces as a permanent
// failure on the target.
func (p *Publisher) Publish(ctx context.Context, raw []byte, post publisher.Post, media publisher.MediaSource) (publisher.Result, error) {
	var creds credentials
	if err := json.Unmarshal(raw, &creds); err != nil || creds.WebhookID == "" || creds.Token == "" {
		return publisher.Result{}, publisher.Permanent(errors.New("discord: stored credentials are unreadable"))
	}

	pl := payload{Content: post.Text}
	pl.AllowedMentions.Parse = []string{}
	for i, m := range post.Media {
		pl.Attachments = append(pl.Attachments, attachment{ID: i, Filename: m.Filename})
	}
	plJSON, err := json.Marshal(pl)
	if err != nil {
		return publisher.Result{}, err
	}

	var (
		body        io.Reader
		length      int64
		contentType string
	)
	if len(post.Media) == 0 {
		body, length, contentType = bytes.NewReader(plJSON), int64(len(plJSON)), "application/json"
	} else {
		body, length, contentType = multipartBody(ctx, plJSON, post.Media, media)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, creds.endpoint()+"?wait=true", body)
	if err != nil {
		return publisher.Result{}, err
	}
	req.ContentLength = length
	req.Header.Set("Content-Type", contentType)

	resp, err := p.client.Do(req)
	if err != nil {
		// Unclassified, so it is retried. A timeout after Discord accepted
		// the message would post it twice; webhooks take no idempotency key.
		return publisher.Result{}, fmt.Errorf("discord: execute webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return publisher.Result{}, apiError(resp)
	}

	var msg struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		// Discord said 200, so the message exists. Retrying would duplicate
		// it; report success without a link instead.
		return publisher.Result{}, nil
	}

	res := publisher.Result{RemoteID: msg.ID}
	if creds.GuildID != "" && msg.ChannelID != "" {
		res.URL = fmt.Sprintf("https://discord.com/channels/%s/%s/%s", creds.GuildID, msg.ChannelID, msg.ID)
	}
	return res, nil
}

// multipartBody streams a multipart request with an exact Content-Length.
//
// Up to ten 20 MiB files is too much to buffer per job, so the files are read
// from storage only as the request body is sent. The length is still known
// up front — the part headers are small and built here, and the file sizes
// were verified against storage when the post was created — so the request
// goes out with Content-Length rather than chunked encoding.
func multipartBody(ctx context.Context, plJSON []byte, files []publisher.Media, src publisher.MediaSource) (io.Reader, int64, string) {
	var head bytes.Buffer
	mw := multipart.NewWriter(&head)

	// The writer only touches head when a part is created or closed, so
	// taking head's bytes between those calls splits the body into the
	// framing (held here) and the file contents (read later).
	var pieces []io.Reader
	var length int64
	flush := func() {
		b := bytes.Clone(head.Bytes())
		head.Reset()
		pieces = append(pieces, bytes.NewReader(b))
		length += int64(len(b))
	}

	pw, _ := mw.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="payload_json"`},
		"Content-Type":        {"application/json"},
	})
	_, _ = pw.Write(plJSON)

	for i, m := range files {
		_, _ = mw.CreatePart(textproto.MIMEHeader{
			"Content-Disposition": {fmt.Sprintf(`form-data; name="files[%d]"; filename=%q`, i, m.Filename)},
			"Content-Type":        {m.ContentType},
		})
		flush()
		pieces = append(pieces, &lazyFile{ctx: ctx, src: src, key: m.Key, size: m.Size})
		length += m.Size
	}
	_ = mw.Close()
	flush()

	return io.MultiReader(pieces...), length, mw.FormDataContentType()
}

// lazyFile opens a stored file on first read and yields exactly size bytes,
// so a file that changed in storage fails the request instead of sending a
// body that disagrees with its Content-Length.
type lazyFile struct {
	ctx  context.Context
	src  publisher.MediaSource
	key  string
	size int64

	rc   io.ReadCloser
	read int64
}

func (f *lazyFile) Read(p []byte) (int, error) {
	if f.rc == nil {
		rc, err := f.src.Open(f.ctx, f.key)
		if err != nil {
			return 0, fmt.Errorf("discord: open %s: %w", f.key, err)
		}
		f.rc = rc
	}
	if remaining := f.size - f.read; int64(len(p)) > remaining {
		p = p[:remaining]
	}
	if len(p) == 0 {
		_ = f.rc.Close()
		return 0, io.EOF
	}
	n, err := f.rc.Read(p)
	f.read += int64(n)
	if errors.Is(err, io.EOF) {
		_ = f.rc.Close()
		if f.read < f.size {
			return n, fmt.Errorf("discord: %s is shorter than its recorded %d bytes", f.key, f.size)
		}
		return n, io.EOF
	}
	return n, err
}

// apiError turns a non-success response into a classified error.
func apiError(resp *http.Response) error {
	var body struct {
		Message    string  `json:"message"`
		Code       int     `json:"code"`
		RetryAfter float64 `json:"retry_after"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = json.Unmarshal(raw, &body)

	msg := body.Message
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	err := fmt.Errorf("discord: %d: %s", resp.StatusCode, msg)

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		after := time.Duration(body.RetryAfter * float64(time.Second))
		if after <= 0 {
			if s, convErr := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64); convErr == nil {
				after = time.Duration(s * float64(time.Second))
			}
		}
		if after <= 0 {
			after = 5 * time.Second
		}
		return publisher.RetryAfter(after, err)
	case resp.StatusCode >= 500:
		return err
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNotFound:
		return publisher.Permanent(fmt.Errorf("%w (the webhook was deleted; reconnect the channel)", err))
	default:
		return publisher.Permanent(err)
	}
}
