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
	if errors.Is(err, pgx.ErrNoRows) {
		user, err = s.q.CreateUser(ctx, email)
	}
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("user: %w", err)
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
