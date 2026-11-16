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
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/FACorreiaa/postitheka/internal/analytics"
	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/idtoken"
)

// tokenPrefix makes a leaked token recognisable to secret scanners and to a
// human reading a log.
const tokenPrefix = "pk_"

var ErrUnauthenticated = errors.New("unauthenticated")

// CreateUserWithToken creates a user, or finds an existing one by email, and
// mints a new API token for it. The token is returned once and never stored.
//
// The operator's path (`server user create EMAIL`): people sign in with Apple or Google
// (SignInWithApple, SignInWithGoogle), which claim a user made here by its email.
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

// IdentityVerifier checks ID tokens from one sign-in provider; idtoken.Verifier is the real one.
type IdentityVerifier interface {
	Configured() bool
	Verify(ctx context.Context, idToken, rawNonce string) (idtoken.Identity, error)
}

// ErrProviderOff is a sign-in asked for with a provider that has no client id configured.
var ErrProviderOff = errors.New("this sign-in provider is not configured")

// provider is one way to sign in: its verifier and the queries that key users on its subject.
type provider struct {
	name     string // for people: "Apple", "Google"
	verifier IdentityVerifier
	bySub    func(context.Context, *string) (db.User, error)
	claim    func(context.Context, string, *string) (db.User, error)
	create   func(context.Context, string, *string) (db.User, error)
}

// SignInWithApple signs a person in with an identity token from Apple (AUTH-001). client names
// the token ("apple-ios", "apple-web"). See signIn.
func (s *Service) SignInWithApple(ctx context.Context, idToken, rawNonce, client string) (string, bool, error) {
	return s.signIn(ctx, provider{
		name: "Apple", verifier: s.apple,
		bySub: s.q.GetUserByAppleSub,
		claim: func(ctx context.Context, email string, sub *string) (db.User, error) {
			return s.q.ClaimUserForApple(ctx, db.ClaimUserForAppleParams{Email: email, AppleSub: sub})
		},
		create: func(ctx context.Context, email string, sub *string) (db.User, error) {
			return s.q.CreateAppleUser(ctx, db.CreateAppleUserParams{Email: email, AppleSub: sub})
		},
	}, idToken, rawNonce, client)
}

// SignInWithGoogle signs a person in with an ID token from Google (AUTH-002). client names the
// token ("google-ios", "google-web"). See signIn.
func (s *Service) SignInWithGoogle(ctx context.Context, idToken, rawNonce, client string) (string, bool, error) {
	return s.signIn(ctx, provider{
		name: "Google", verifier: s.google,
		bySub: s.q.GetUserByGoogleSub,
		claim: func(ctx context.Context, email string, sub *string) (db.User, error) {
			return s.q.ClaimUserForGoogle(ctx, db.ClaimUserForGoogleParams{Email: email, GoogleSub: sub})
		},
		create: func(ctx context.Context, email string, sub *string) (db.User, error) {
			return s.q.CreateGoogleUser(ctx, db.CreateGoogleUserParams{Email: email, GoogleSub: sub})
		},
	}, idToken, rawNonce, client)
}

// signIn checks an ID token, finds or creates its user, and returns a new API token: the same
// bearer token the rest of the API already takes. The bool is true when the user was created.
//
// The user is found by the provider's subject, never by email. Failing that, a user with the
// same email is claimed, but only for an email the provider has verified: one the CLI made (so
// the owner's account carries over), or one another provider made (so a person who signs in
// with Apple and then Google stays one account). Failing that, a user is created; that too
// needs a verified email, since an account needs an address to be reached at.
func (s *Service) signIn(ctx context.Context, p provider, idToken, rawNonce, client string) (string, bool, error) {
	if p.verifier == nil || !p.verifier.Configured() {
		return "", false, ErrProviderOff
	}
	id, err := p.verifier.Verify(ctx, idToken, rawNonce)
	if errors.Is(err, idtoken.ErrInvalid) {
		return "", false, ErrUnauthenticated
	}
	if err != nil {
		return "", false, fmt.Errorf("sign in with %s: %w", p.name, err)
	}

	created := false
	user, err := p.bySub(ctx, &id.Subject)
	if errors.Is(err, pgx.ErrNoRows) {
		switch {
		case id.Email == "":
			return "", false, &BadRequest{Message: p.name + " did not share an email address; sign in again and allow it"}
		case !id.EmailVerified:
			return "", false, &BadRequest{Message: p.name + " has not verified this email address"}
		}
		user, err = p.claim(ctx, id.Email, &id.Subject)
		if errors.Is(err, pgx.ErrNoRows) {
			user, err = p.create(ctx, id.Email, &id.Subject)
			created = err == nil
		}
	}
	if isUniqueViolation(err) {
		// The email belongs to a user already tied to another id at this provider.
		return "", false, &BadRequest{Message: "This email address already belongs to another account"}
	}
	if err != nil {
		return "", false, fmt.Errorf("sign in with %s: user: %w", p.name, err)
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

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
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
