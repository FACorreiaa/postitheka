// Package publisher defines what every platform integration implements, so the
// rest of the application never names a platform. Adding X or Instagram means
// one new package that satisfies Publisher, registered in cmd/server.
package publisher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
)

type Platform string

const (
	Discord   Platform = "discord"
	LinkedIn  Platform = "linkedin"
	X         Platform = "x"
	Instagram Platform = "instagram"
	Facebook  Platform = "facebook"
	TikTok    Platform = "tiktok"
	WhatsApp  Platform = "whatsapp"
)

// IsAssisted reports whether a platform is published by handing the post to
// its own app on the device (the share sheet) rather than by the server.
// Such a platform has no Publisher and no publish job; WhatsApp has no
// posting API at all.
func IsAssisted(p Platform) bool { return p == WhatsApp }

// Media is one attached file, as the publisher sees it: metadata plus a key to
// fetch it by. The bytes are not loaded until a publisher asks for them,
// because some platforms never want them — Instagram downloads from a URL.
type Media struct {
	Key         string
	ContentType string
	Size        int64
	// Filename is what the platform shows, with an extension matching
	// ContentType; Discord decides whether to render a preview from it.
	Filename string
}

type Post struct {
	Text  string
	Media []Media
}

// MediaSource is how a publisher reaches the files of a post.
type MediaSource interface {
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	// PublicURL is for platforms that fetch media themselves. The URL must
	// outlive the platform's download, which for Instagram can be minutes.
	PublicURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}

// Result says where a published post ended up.
type Result struct {
	RemoteID string
	URL      string
}

// Connection is what connecting an account yields: a name to show the user and
// the credentials to store. Credentials are opaque outside the publisher that
// produced them; they are sealed before they are written anywhere.
type Connection struct {
	DisplayName string
	Credentials []byte
	// RemoteID is the platform's id for what was connected, when it has one.
	// Connecting the same RemoteID again replaces the stored credentials
	// rather than adding a second account.
	RemoteID string
}

// OAuthConnector is implemented by a publisher whose accounts connect through
// the platform's consent screen. The service mints and checks the state and
// the PKCE verifier; the publisher knows the platform's endpoints, scopes and
// whatever it deviates from the standard on.
type OAuthConnector interface {
	// AuthorizeURL is where the user is sent to grant access.
	AuthorizeURL(state, verifier, redirectURL string) string
	// Exchange trades the code from the callback for credentials, and looks
	// up who was connected for the display name and RemoteID.
	Exchange(ctx context.Context, code, verifier, redirectURL string) (Connection, error)
}

type Publisher interface {
	Platform() Platform
	Rules() Rules

	// Publish sends one post to one account. Errors should be classified with
	// Permanent or RetryAfter where the platform's answer allows it; anything
	// unclassified is retried with backoff.
	//
	// Delivery is at least once. A request that times out after the platform
	// accepted it is indistinguishable from one that never arrived, and is
	// retried. Where a platform offers an idempotency key, use it.
	Publish(ctx context.Context, credentials []byte, post Post, media MediaSource) (Result, error)
}

// Registry holds the publishers this process can use.
type Registry struct {
	byPlatform map[Platform]Publisher
}

func NewRegistry(pubs ...Publisher) *Registry {
	r := &Registry{byPlatform: make(map[Platform]Publisher, len(pubs))}
	for _, p := range pubs {
		r.byPlatform[p.Platform()] = p
	}
	return r
}

func (r *Registry) Get(p Platform) (Publisher, bool) {
	pub, ok := r.byPlatform[p]
	return pub, ok
}

// All returns the publishers in a stable order, for listing to clients.
func (r *Registry) All() []Publisher {
	out := make([]Publisher, 0, len(r.byPlatform))
	for _, p := range r.byPlatform {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Publisher) int {
		switch {
		case a.Platform() < b.Platform():
			return -1
		case a.Platform() > b.Platform():
			return 1
		}
		return 0
	})
	return out
}

// AcceptsUpload reports whether at least one platform could use a file of this
// type and size. Uploads are checked against this rather than any one
// platform's rules because a file is uploaded before its targets are chosen;
// the per-platform check happens when the post is created.
func (r *Registry) AcceptsUpload(contentType string, size int64) error {
	if size <= 0 {
		return errors.New("size must be positive")
	}
	typeKnown := false
	for _, p := range r.byPlatform {
		rules := p.Rules()
		if !slices.Contains(rules.MediaTypes, contentType) {
			continue
		}
		typeKnown = true
		if size <= rules.MaxBytes(contentType) {
			return nil
		}
	}
	if !typeKnown {
		return fmt.Errorf("no platform this server publishes to accepts %q", contentType)
	}
	return fmt.Errorf("%d bytes is larger than any platform accepts for %q", size, contentType)
}

// permanentError marks an error that retrying cannot fix: a deleted webhook, a
// rejected post, revoked credentials.
type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

func Permanent(err error) error { return permanentError{err} }

func IsPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}

// RetryAfterError is a rate limit with a known reset: the job waits exactly
// this long instead of guessing with backoff.
type RetryAfterError struct {
	After time.Duration
	Err   error
}

func (e *RetryAfterError) Error() string {
	return fmt.Sprintf("%v (retry after %s)", e.Err, e.After)
}
func (e *RetryAfterError) Unwrap() error { return e.Err }

func RetryAfter(after time.Duration, err error) error {
	return &RetryAfterError{After: after, Err: err}
}
