// Package analytics records the few product events that measure whether
// strangers use Postitheka: someone signs up, connects a destination, and a
// post lands (or does not).
//
// The rules are Seshat's. Capture never blocks and never fails a request or a
// job: an analytics outage must be invisible. Nothing a person wrote is
// captured (no email, no post text, no error text from a platform): an event
// is the shape of the funnel, not its contents. An unset API key disables the
// whole thing, so development needs no credential and sends nothing.
package analytics

import (
	"context"
	"log/slog"
	"time"
)

// Event is one thing that happened. DistinctID is the user's id, so one
// person's events join up across devices.
type Event struct {
	Name       string
	DistinctID string
	Props      map[string]any
	Timestamp  time.Time
}

// Client records events. Implementations are safe for concurrent use and do
// not block the caller.
type Client interface {
	Capture(Event)
	Close(ctx context.Context) error
}

// The events, a closed set: each answers a question the P1 gate asks. Adding
// one is a decision, not an afterthought. Targets rather than posts, because
// a post that reaches Discord and fails on X is both.
const (
	EventSignedUp         = "signed_up"
	EventAccountConnected = "account_connected"
	EventTargetPublished  = "target_published"
	EventTargetFailed     = "target_failed"
)

// Nop discards everything. It is what an unconfigured server gets, and what
// tests get when they do not care.
func Nop() Client { return nopClient{} }

type nopClient struct{}

func (nopClient) Capture(Event)               {}
func (nopClient) Close(context.Context) error { return nil }

// Props is a small helper so call sites read as one line rather than five.
func Props(pairs ...any) map[string]any {
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			continue
		}
		m[key] = pairs[i+1]
	}
	return m
}

// New returns a PostHog client, or Nop when no API key is configured.
func New(apiKey, host string, log *slog.Logger) Client {
	if apiKey == "" {
		return Nop()
	}
	return newPostHog(apiKey, host, log)
}
