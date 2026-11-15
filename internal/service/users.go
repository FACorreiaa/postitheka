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
	"github.com/FACorreiaa/postitheka/internal/db"
)

// tokenPrefix makes a leaked token recognisable to secret scanners and to a
// human reading a log.
const tokenPrefix = "pk_"

var ErrUnauthenticated = errors.New("unauthenticated")

// CreateUserWithToken creates a user, or finds an existing one by email, and
// mints a new API token for it. The token is returned once and never stored.
//
// This is the stand-in for sign-up until Sign in with Apple exists: run
// `postitheka user create <email>` and paste the token into the client.
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

	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	token := tokenPrefix + base64.RawURLEncoding.EncodeToString(secret)

	if err := s.q.CreateAPIToken(ctx, db.CreateAPITokenParams{
		UserID:    user.ID,
		TokenHash: hashToken(token),
		Name:      tokenName,
	}); err != nil {
		return uuid.Nil, "", fmt.Errorf("token: %w", err)
	}
	return user.ID, token, nil
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
