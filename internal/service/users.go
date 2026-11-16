package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/FACorreiaa/postitheka/internal/analytics"
	"github.com/FACorreiaa/postitheka/internal/appleid"
	"github.com/FACorreiaa/postitheka/internal/db"
)

// tokenPrefix makes a leaked token recognisable to secret scanners and to a
// human reading a log.
const tokenPrefix = "pk_"

var ErrUnauthenticated = errors.New("unauthenticated")

// CreateUserWithToken creates a user, or finds an existing one by email, and
// mints a new API token for it. The token is returned once and never stored.
//
// The operator's path (`server user create EMAIL`): people sign in with Apple
// (SignInWithApple), which claims a user made here by its email.
func (s *Service) CreateUserWithToken(ctx context.Context, email, tokenName string) (uuid.UUID, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") {
		return uuid.Nil, "", &BadRequest{Message: "not an email address"}
	}

	user, err := s.q.GetUserByEmail(ctx, email)
	created := false
	if errors.Is(err, pgx.ErrNoRows) {
		user, err = s.q.CreateUser(ctx, email)
		created = err == nil
	}
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("user: %w", err)
	}
	if created {
		s.events.Capture(analytics.Event{Name: analytics.EventSignedUp, DistinctID: user.ID.String()})
	}

	token, err := s.mintToken(ctx, user.ID, tokenName)
	if err != nil {
		return uuid.Nil, "", err
	}
	return user.ID, token, nil
}

// mintToken creates an API token for userID. The token is returned once and only its hash is
// stored.
func (s *Service) mintToken(ctx context.Context, userID uuid.UUID, name string) (string, error) {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	token := tokenPrefix + base64.RawURLEncoding.EncodeToString(secret)
	if err := s.q.CreateAPIToken(ctx, db.CreateAPITokenParams{
		UserID:    userID,
		TokenHash: hashToken(token),
		Name:      name,
	}); err != nil {
		return "", fmt.Errorf("token: %w", err)
	}
	return token, nil
}

// AppleVerifier checks a Sign in with Apple identity token; appleid.Verifier is the real one.
type AppleVerifier interface {
	Configured() bool
	Verify(ctx context.Context, identityToken, rawNonce string) (appleid.Identity, error)
}

// ErrAppleOff is Apple sign-in asked for on a server with no Apple client id configured.
var ErrAppleOff = errors.New("sign in with Apple is not configured")

// SignInWithApple signs a person in with an identity token from Apple, creating their user on
// the first sign-in, and returns a new API token: the same bearer token the rest of the API
// already takes (AUTH-001). client names the token ("apple-ios", "apple-web").
//
// The user is found by Apple's subject, never by email. A user made before Apple sign-in (by
// `server user create`) is claimed by the first Apple sign-in whose verified email matches, so
// the owner's existing account carries over. An Apple id with no user yet and no email to
// create one with is refused: an account needs an address to be reached at.
func (s *Service) SignInWithApple(ctx context.Context, identityToken, rawNonce, client string) (string, bool, error) {
	if s.apple == nil || !s.apple.Configured() {
		return "", false, ErrAppleOff
	}
	id, err := s.apple.Verify(ctx, identityToken, rawNonce)
	if errors.Is(err, appleid.ErrInvalid) {
		return "", false, ErrUnauthenticated
	}
	if err != nil {
		return "", false, fmt.Errorf("sign in with apple: %w", err)
	}

	created := false
	user, err := s.q.GetUserByAppleSub(ctx, &id.Subject)
	if errors.Is(err, pgx.ErrNoRows) && id.Email != "" && id.EmailVerified {
		user, err = s.q.ClaimUserForApple(ctx, db.ClaimUserForAppleParams{Email: id.Email, AppleSub: &id.Subject})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if id.Email == "" {
			return "", false, &BadRequest{Message: "Apple did not share an email address; sign in again and allow it"}
		}
		user, err = s.q.CreateAppleUser(ctx, db.CreateAppleUserParams{Email: id.Email, AppleSub: &id.Subject})
		created = err == nil
	}
	if err != nil {
		// An email already held by another Apple id (a unique violation) lands here too; it
		// cannot happen with Apple's own addresses, which are unique to one Apple id.
		return "", false, fmt.Errorf("sign in with apple: user: %w", err)
	}
	if created {
		s.events.Capture(analytics.Event{Name: analytics.EventSignedUp, DistinctID: user.ID.String()})
	}
	token, err := s.mintToken(ctx, user.ID, client)
	if err != nil {
		return "", false, err
	}
	return token, created, nil
}

// Authenticate resolves a bearer token to its user.
//
// The lookup is by hash, so the comparison happens inside a B-tree on a
// value the caller cannot choose the preimage of; there is no byte-by-byte
// compare for a timing attack to measure.
func (s *Service) Authenticate(ctx context.Context, token string) (uuid.UUID, error) {
	if !strings.HasPrefix(token, tokenPrefix) {
		return uuid.Nil, ErrUnauthenticated
	}
	userID, err := s.q.UseAPIToken(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrUnauthenticated
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("authenticate: %w", err)
	}
	return userID, nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// DeleteUserByEmail deletes the user with that email address. See DeleteUser.
func (s *Service) DeleteUserByEmail(ctx context.Context, email string) (uuid.UUID, int, error) {
	user, err := s.q.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, 0, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, 0, fmt.Errorf("user: %w", err)
	}
	objects, err := s.DeleteUser(ctx, user.ID)
	return user.ID, objects, err
}

// DeleteUser removes a user and everything they own: tokens, accounts with
// their credentials, posts, and the uploads in the bucket. It reports how many
// objects it removed. Nothing is deleted from a platform.
//
// The bucket is emptied twice, around the row. Before, so that a failure
// leaves the user in place and the call can simply be made again; with the
// row gone first, nothing would remember whose objects were left. After,
// because until the row is gone the user's tokens still work and an upload
// could land in between.
//
// Publish jobs still queued for the user's posts find no target and stop.
// One gap remains: an upload URL handed out before the deletion stays valid
// for its 30 minutes, and a file sent to it afterwards has no row and no
// owner. Nothing finds it again short of deleting the prefix once more.
func (s *Service) DeleteUser(ctx context.Context, userID uuid.UUID) (int, error) {
	prefix := uploadPrefix(userID)
	objects, err := s.storage.DeletePrefix(ctx, prefix)
	if err != nil {
		return objects, fmt.Errorf("delete user: uploads: %w", err)
	}
	n, err := s.q.DeleteUser(ctx, userID)
	if err != nil {
		return objects, fmt.Errorf("delete user: %w", err)
	}
	if n == 0 {
		return objects, ErrNotFound
	}
	late, err := s.storage.DeletePrefix(ctx, prefix)
	if err != nil {
		// The user is gone; only these objects are left, and the prefix is
		// the one thing that still identifies them.
		return objects + late, fmt.Errorf("delete user: the user is deleted, but objects under %s may remain: %w", prefix, err)
	}
	return objects + late, nil
}
