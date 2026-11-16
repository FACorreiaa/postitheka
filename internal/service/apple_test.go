package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FACorreiaa/postitheka/internal/appleid"
	"github.com/FACorreiaa/postitheka/internal/service"
)

// fakeApple stands in for appleid.Verifier: a token is accepted when it names an identity the
// test registered, with the nonce the test expects.
type fakeApple struct {
	ids map[string]appleid.Identity
	off bool
}

func (f *fakeApple) Configured() bool { return !f.off }

func (f *fakeApple) Verify(_ context.Context, token, nonce string) (appleid.Identity, error) {
	id, ok := f.ids[token]
	if !ok || nonce != "nonce" {
		return appleid.Identity{}, appleid.ErrInvalid
	}
	return id, nil
}

func appleEnv(t *testing.T, ids map[string]appleid.Identity) (*env, *service.Service) {
	t.Helper()
	e := setup(t)
	svc := service.New(service.Deps{Pool: e.pool, Analytics: e.events, Apple: &fakeApple{ids: ids}})
	return e, svc
}

func TestSignInWithAppleCreatesTheUserOnceAndKeysOnSubject(t *testing.T) {
	e, svc := appleEnv(t, map[string]appleid.Identity{
		"first":      {Subject: "apple.1", Email: "new@example.com", EmailVerified: true},
		"again":      {Subject: "apple.1", Email: "changed@privaterelay.appleid.com", EmailVerified: true},
		"no-email-2": {Subject: "apple.1"},
	})
	ctx := context.Background()

	token, created, err := svc.SignInWithApple(ctx, "first", "nonce", "apple-ios")
	if err != nil || !created {
		t.Fatalf("first sign-in: created %v, err %v", created, err)
	}
	user, err := svc.Authenticate(ctx, token)
	if err != nil {
		t.Fatalf("the token does not authenticate: %v", err)
	}

	// Same Apple id, different email, then no email at all: the same user every time.
	for _, tok := range []string{"again", "no-email-2"} {
		token, created, err := svc.SignInWithApple(ctx, tok, "nonce", "apple-web")
		if err != nil || created {
			t.Fatalf("%s: created %v, err %v", tok, created, err)
		}
		if got, _ := svc.Authenticate(ctx, token); got != user {
			t.Fatalf("%s signed in as %s, want %s", tok, got, user)
		}
	}
	if got := e.events.names(); len(got) != 1 || got[0] != "signed_up" {
		t.Errorf("events %v, want one signed_up", got)
	}
}

func TestSignInWithAppleClaimsAUserMadeByTheCLI(t *testing.T) {
	e, svc := appleEnv(t, map[string]appleid.Identity{
		"owner":      {Subject: "apple.owner", Email: "owner@example.com", EmailVerified: true},
		"unverified": {Subject: "apple.other", Email: "someone@example.com", EmailVerified: false},
	})
	ctx := context.Background()
	owner := e.user(t, "owner@example.com")
	someone := e.user(t, "someone@example.com")

	token, created, err := svc.SignInWithApple(ctx, "owner", "nonce", "apple-ios")
	if err != nil || created {
		t.Fatalf("created %v, err %v; want the existing user", created, err)
	}
	if got, _ := svc.Authenticate(ctx, token); got != owner {
		t.Fatalf("signed in as %s, want the CLI user %s", got, owner)
	}

	// An unverified email claims nothing; with the address taken, no user can be made for it.
	if _, _, err := svc.SignInWithApple(ctx, "unverified", "nonce", "apple-ios"); err == nil {
		t.Fatal("an unverified email took over an existing user, or made a duplicate")
	}
	var sub *string
	if err := e.pool.QueryRow(ctx, "SELECT apple_sub FROM users WHERE id = $1", someone).Scan(&sub); err != nil || sub != nil {
		t.Fatalf("someone's user got apple_sub %v (err %v)", sub, err)
	}
}

func TestSignInWithAppleRefuses(t *testing.T) {
	_, svc := appleEnv(t, map[string]appleid.Identity{
		"no-email": {Subject: "apple.2"},
	})
	ctx := context.Background()

	if _, _, err := svc.SignInWithApple(ctx, "forged", "nonce", "apple-ios"); !errors.Is(err, service.ErrUnauthenticated) {
		t.Errorf("a token Apple did not sign: err %v, want ErrUnauthenticated", err)
	}
	var bad *service.BadRequest
	if _, _, err := svc.SignInWithApple(ctx, "no-email", "nonce", "apple-ios"); !errors.As(err, &bad) {
		t.Errorf("a new Apple id with no email: err %v, want BadRequest", err)
	}

	off := service.New(service.Deps{Apple: &fakeApple{off: true}})
	if _, _, err := off.SignInWithApple(ctx, "x", "nonce", "apple-ios"); !errors.Is(err, service.ErrAppleOff) {
		t.Errorf("with no client id: err %v, want ErrAppleOff", err)
	}
	if _, _, err := service.New(service.Deps{}).SignInWithApple(ctx, "x", "nonce", "apple-ios"); !errors.Is(err, service.ErrAppleOff) {
		t.Errorf("with no verifier: err %v, want ErrAppleOff", err)
	}
}
