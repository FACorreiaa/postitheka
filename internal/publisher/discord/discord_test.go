package discord

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/FACorreiaa/postitheka/internal/publisher"
)

// stub points every request the publisher makes at a local server, whatever
// host it was addressed to. The publisher keeps its real Discord URLs, so the
// tests also check that it never addresses anything else.
func stub(t *testing.T, h http.HandlerFunc) (*Publisher, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r)
		h(w, r)
	}))
	t.Cleanup(srv.Close)

	target, _ := url.Parse(srv.URL)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "discord.com" {
			t.Errorf("request addressed to %s, want discord.com", r.URL.Host)
		}
		r = r.Clone(r.Context())
		r.Header.Set("X-Original-Host", r.URL.Host)
		r.URL.Scheme, r.URL.Host = target.Scheme, target.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
	return New(client), &seen
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testCreds() []byte {
	b, _ := json.Marshal(credentials{WebhookID: "123", Token: "tok", GuildID: "g1", ChannelID: "c1"})
	return b
}

type memMedia map[string]string

func (m memMedia) Open(_ context.Context, key string) (io.ReadCloser, error) {
	s, ok := m[key]
	if !ok {
		return nil, errors.New("no such key")
	}
	return io.NopCloser(strings.NewReader(s)), nil
}

func (m memMedia) PublicURL(context.Context, string, time.Duration) (string, error) {
	return "", errors.New("not used by discord")
}

func TestParseWebhookURL(t *testing.T) {
	ok := []string{
		"https://discord.com/api/webhooks/123/abc-DEF_9",
		"https://discordapp.com/api/webhooks/123/abc",
		"https://canary.discord.com/api/v10/webhooks/123/abc/",
		"  https://discord.com/api/webhooks/123/abc  ",
	}
	for _, raw := range ok {
		if _, _, err := ParseWebhookURL(raw); err != nil {
			t.Errorf("%q rejected: %v", raw, err)
		}
	}

	bad := []string{
		"http://discord.com/api/webhooks/123/abc",   // not https
		"https://evil.example/api/webhooks/123/abc", // not Discord
		"https://discord.com.evil.example/api/webhooks/1/a",
		"https://discord.com/api/webhooks/abc/abc",      // id not numeric
		"https://discord.com/api/webhooks/123",          // no token
		"https://discord.com/api/channels/123/messages", // not a webhook
		"not a url",
	}
	for _, raw := range bad {
		if _, _, err := ParseWebhookURL(raw); !errors.Is(err, ErrInvalidWebhook) {
			t.Errorf("%q accepted", raw)
		}
	}
}

func TestConnect(t *testing.T) {
	p, seen := stub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"123","name":"announcements","guild_id":"g1","channel_id":"c1"}`)
	})

	conn, err := p.Connect(context.Background(), "https://discordapp.com/api/webhooks/123/tok")
	if err != nil {
		t.Fatal(err)
	}
	if conn.DisplayName != "announcements" {
		t.Errorf("display name %q", conn.DisplayName)
	}
	if got := (*seen)[0].URL.Path; got != "/api/v10/webhooks/123/tok" {
		t.Errorf("checked %s", got)
	}

	var c credentials
	if err := json.Unmarshal(conn.Credentials, &c); err != nil {
		t.Fatal(err)
	}
	if c != (credentials{WebhookID: "123", Token: "tok", GuildID: "g1", ChannelID: "c1"}) {
		t.Errorf("stored %+v", c)
	}
}

func TestConnectRejectsDeletedWebhook(t *testing.T) {
	p, _ := stub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Unknown Webhook","code":10015}`)
	})
	if _, err := p.Connect(context.Background(), "https://discord.com/api/webhooks/123/tok"); !errors.Is(err, ErrInvalidWebhook) {
		t.Fatalf("got %v, want ErrInvalidWebhook", err)
	}
}

func TestPublishText(t *testing.T) {
	var got payload
	p, seen := stub(t, func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content type %q", ct)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"id":"m1","channel_id":"c1"}`)
	})

	res, err := p.Publish(context.Background(), testCreds(), publisher.Post{Text: "hello @everyone"}, memMedia{})
	if err != nil {
		t.Fatal(err)
	}

	req := (*seen)[0]
	if req.URL.Path != "/api/v10/webhooks/123/tok" || req.URL.Query().Get("wait") != "true" {
		t.Errorf("posted to %s", req.URL)
	}
	if got.Content != "hello @everyone" {
		t.Errorf("content %q", got.Content)
	}
	if got.AllowedMentions.Parse == nil || len(got.AllowedMentions.Parse) != 0 {
		t.Errorf("mentions not disabled: %+v", got.AllowedMentions)
	}
	if res.RemoteID != "m1" || res.URL != "https://discord.com/channels/g1/c1/m1" {
		t.Errorf("result %+v", res)
	}
}

func TestPublishMedia(t *testing.T) {
	files := memMedia{"k/a": "PNGDATA", "k/b": "MP4DATA-longer"}
	post := publisher.Post{
		Text: "two files",
		Media: []publisher.Media{
			{Key: "k/a", ContentType: "image/png", Size: 7, Filename: "a.png"},
			{Key: "k/b", ContentType: "video/mp4", Size: 14, Filename: "b.mp4"},
		},
	}

	p, _ := stub(t, func(w http.ResponseWriter, r *http.Request) {
		// Content-Length, not chunked: the length is computed up front.
		if r.ContentLength <= 0 || len(r.TransferEncoding) > 0 {
			t.Errorf("content length %d, transfer encoding %v", r.ContentLength, r.TransferEncoding)
		}
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatal(err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])

		var parts []string
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(part)
			switch part.FormName() {
			case "payload_json":
				var pl payload
				_ = json.Unmarshal(data, &pl)
				if len(pl.Attachments) != 2 || pl.Attachments[1].Filename != "b.mp4" || pl.Attachments[1].ID != 1 {
					t.Errorf("attachments %+v", pl.Attachments)
				}
			default:
				parts = append(parts, part.FormName()+"="+part.FileName()+":"+part.Header.Get("Content-Type")+":"+string(data))
			}
		}
		want := "files[0]=a.png:image/png:PNGDATA,files[1]=b.mp4:video/mp4:MP4DATA-longer"
		if strings.Join(parts, ",") != want {
			t.Errorf("parts\n got %s\nwant %s", strings.Join(parts, ","), want)
		}
		_, _ = io.WriteString(w, `{"id":"m2","channel_id":"c1"}`)
	})

	if _, err := p.Publish(context.Background(), testCreds(), post, files); err != nil {
		t.Fatal(err)
	}
}

func TestPublishFailsWhenStoredFileIsShort(t *testing.T) {
	p, _ := stub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"id":"m"}`)
	})
	post := publisher.Post{Media: []publisher.Media{{Key: "k", ContentType: "image/png", Size: 100, Filename: "a.png"}}}
	if _, err := p.Publish(context.Background(), testCreds(), post, memMedia{"k": "short"}); err == nil {
		t.Fatal("published a file shorter than its recorded size")
	}
}

func TestPublishErrorClassification(t *testing.T) {
	cases := map[string]struct {
		status    int
		header    string
		body      string
		permanent bool
		retry     time.Duration
	}{
		"rate limited, body":   {429, "", `{"message":"slow down","retry_after":1.5}`, false, 1500 * time.Millisecond},
		"rate limited, header": {429, "3", `{}`, false, 3 * time.Second},
		"server error":         {502, "", `bad gateway`, false, 0},
		"deleted webhook":      {404, "", `{"message":"Unknown Webhook","code":10015}`, true, 0},
		"rejected message":     {400, "", `{"message":"Cannot send an empty message","code":50006}`, true, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p, _ := stub(t, func(w http.ResponseWriter, r *http.Request) {
				if tc.header != "" {
					w.Header().Set("Retry-After", tc.header)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})

			_, err := p.Publish(context.Background(), testCreds(), publisher.Post{Text: "x"}, memMedia{})
			if err == nil {
				t.Fatal("no error")
			}
			if publisher.IsPermanent(err) != tc.permanent {
				t.Errorf("permanent = %v, want %v (%v)", publisher.IsPermanent(err), tc.permanent, err)
			}
			var ra *publisher.RetryAfterError
			if errors.As(err, &ra) != (tc.retry > 0) || (ra != nil && ra.After != tc.retry) {
				t.Errorf("retry after %v, want %v", ra, tc.retry)
			}
		})
	}
}

func TestPublishRejectsUnreadableCredentials(t *testing.T) {
	_, err := New(nil).Publish(context.Background(), []byte("{}"), publisher.Post{Text: "x"}, memMedia{})
	if !publisher.IsPermanent(err) {
		t.Fatalf("got %v, want a permanent error", err)
	}
}
