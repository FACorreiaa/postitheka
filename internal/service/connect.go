package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/publisher"
)

// oauthStateTTL is how long a consent screen may stay open. Platforms' own
// authorization codes last about as long (Threads: an hour, most: minutes).
const oauthStateTTL = 15 * time.Minute

// ConnectClient is where the user started connecting, which is where they are
// sent back to: the web app, or the iOS app through its URL scheme.
type ConnectClient string

const (
	ConnectFromWeb ConnectClient = "web"
	ConnectFromIOS ConnectClient = "ios"
)

// ConnectFailure says why a connect did not complete, in words a client can
// map to its own message. It travels in a redirect URL, so it is a code and
// never a platform's error text.
type ConnectFailure string

const (
	ConnectCancelled ConnectFailure = "cancelled" // the user declined on the platform
	ConnectExpired   ConnectFailure = "expired"   // the state is unknown, used or too old
	ConnectFailed    ConnectFailure = "failed"    // the platform refused the exchange
)

func (f ConnectFailure) Error() string { return "connect " + string(f) }

// StartConnect opens a consent screen: it records a state and a PKCE verifier
// for this user and returns the platform URL to send them to.
func (s *Service) StartConnect(ctx context.Context, userID uuid.UUID, platform publisher.Platform, client ConnectClient) (string, error) {
	connector, ok := s.oauthConnector(platform)
	if !ok {
		return "", ErrNotFound
	}
	// Abandoned consent screens are cleared here rather than by a job: a user
	// has a handful at most, and this is the only place that adds them.
	if err := s.q.DeleteStaleOAuthStates(ctx, db.DeleteStaleOAuthStatesParams{UserID: userID, CreatedAt: s.now().Add(-oauthStateTTL)}); err != nil {
		return "", err
	}

	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	state := base64.RawURLEncoding.EncodeToString(raw)
	hash := stateHash(state)
	verifier := oauth2.GenerateVerifier()

	if err := s.q.CreateOAuthState(ctx, db.CreateOAuthStateParams{
		StateHash: hash,
		UserID:    userID,
		Platform:  string(platform),
		Verifier:  s.box.Seal([]byte(verifier), hash),
		Client:    string(client),
	}); err != nil {
		return "", fmt.Errorf("store oauth state: %w", err)
	}
	return connector.AuthorizeURL(state, verifier, s.redirectURL(platform)), nil
}

// FinishConnect completes a consent screen from the platform's callback. It
// always reports the client the flow started from when the state is known,
// so the caller can send the user back there with the outcome, including on
// failure. platformError is the callback's error parameter, if any.
func (s *Service) FinishConnect(ctx context.Context, platform publisher.Platform, state, code, platformError string) (ConnectClient, Account, error) {
	connector, ok := s.oauthConnector(platform)
	if !ok {
		return "", Account{}, ErrNotFound
	}
	hash := stateHash(state)
	row, err := s.q.TakeOAuthState(ctx, db.TakeOAuthStateParams{StateHash: hash, Platform: string(platform)})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Account{}, ConnectExpired
	}
	if err != nil {
		return "", Account{}, err
	}
	client := ConnectClient(row.Client)
	if s.now().Sub(row.CreatedAt) > oauthStateTTL {
		return client, Account{}, ConnectExpired
	}
	if platformError != "" || code == "" {
		return client, Account{}, ConnectCancelled
	}
	verifier, err := s.box.Open(row.Verifier, hash)
	if err != nil {
		return client, Account{}, fmt.Errorf("open oauth verifier: %w", err)
	}

	conn, err := connector.Exchange(ctx, code, string(verifier), s.redirectURL(platform))
	if err != nil {
		// Logged here because the user only sees "failed": the platform's
		// reason is what an operator needs. Never the code or tokens.
		slog.WarnContext(ctx, "oauth exchange failed", "platform", platform, "error", err)
		return client, Account{}, ConnectFailed
	}
	acct, err := s.storeAccount(ctx, row.UserID, platform, conn, nil)
	return client, acct, err
}

// ConnectsByOAuth reports whether a platform's accounts connect through its
// consent page rather than a pasted webhook.
func (s *Service) ConnectsByOAuth(p publisher.Platform) bool {
	_, ok := s.oauthConnector(p)
	return ok
}

func (s *Service) oauthConnector(p publisher.Platform) (publisher.OAuthConnector, bool) {
	pub, ok := s.registry.Get(p)
	if !ok {
		return nil, false
	}
	c, ok := pub.(publisher.OAuthConnector)
	return c, ok
}

func (s *Service) redirectURL(p publisher.Platform) string {
	return s.publicURL + "/oauth/" + string(p) + "/callback"
}

func stateHash(state string) []byte {
	h := sha256.Sum256([]byte(state))
	return h[:]
}
