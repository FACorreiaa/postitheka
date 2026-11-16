package appleid_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FACorreiaa/postitheka/internal/appleid"
)

const bundle = "com.example.postitheka"

// apple stands in for Apple's key endpoint and signs tokens with a key it publishes.
type apple struct {
	key     *rsa.PrivateKey
	kid     string
	fetches atomic.Int32
	server  *httptest.Server
}

func newApple(t *testing.T) *apple {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	a := &apple{key: key, kid: "test-kid"}
	a.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		a.fetches.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": a.kid, "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(a.server.Close)
	return a
}

func (a *apple) sign(t *testing.T, kid string, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	head := enc(map[string]string{"alg": "RS256", "kid": kid}) + "." + enc(claims)
	sum := sha256.Sum256([]byte(head))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return head + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func hashed(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func claims(now time.Time, nonce string) map[string]any {
	return map[string]any{
		"iss": appleid.Issuer, "aud": bundle, "sub": "001234.abcd",
		"exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix(),
		"nonce": hashed(nonce), "email": "Person@Example.com", "email_verified": "true",
	}
}

func TestVerifyAcceptsAGoodToken(t *testing.T) {
	a := newApple(t)
	now := time.Now()
	v := &appleid.Verifier{Audiences: []string{"web.services.id", bundle}, KeysURL: a.server.URL}
	got, err := v.Verify(context.Background(), a.sign(t, a.kid, claims(now, "raw-nonce")), "raw-nonce")
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "001234.abcd" || got.Email != "person@example.com" || !got.EmailVerified {
		t.Fatalf("identity = %+v", got)
	}
	// A second sign-in uses the cached keys.
	if _, err := v.Verify(context.Background(), a.sign(t, a.kid, claims(now, "n2")), "n2"); err != nil {
		t.Fatal(err)
	}
	if n := a.fetches.Load(); n != 1 {
		t.Fatalf("fetched Apple's keys %d times, want 1", n)
	}
}

func TestVerifyRefusesWhatApplesDocsSayToCheck(t *testing.T) {
	a := newApple(t)
	other := newApple(t) // same kid, different key: a forged signature
	now := time.Now()
	good := claims(now, "n")
	with := func(k string, v any) map[string]any {
		c := map[string]any{}
		for key, val := range good {
			c[key] = val
		}
		c[k] = v
		return c
	}
	cases := map[string]struct{ token, nonce string }{
		"wrong audience":   {a.sign(t, a.kid, with("aud", "com.someone.else")), "n"},
		"wrong issuer":     {a.sign(t, a.kid, with("iss", "https://evil.example")), "n"},
		"expired":          {a.sign(t, a.kid, with("exp", now.Add(-10*time.Minute).Unix())), "n"},
		"no subject":       {a.sign(t, a.kid, with("sub", "")), "n"},
		"nonce mismatch":   {a.sign(t, a.kid, good), "another"},
		"raw nonce in jwt": {a.sign(t, a.kid, with("nonce", "n")), "n"},
		"no nonce sent":    {a.sign(t, a.kid, good), ""},
		"unknown key":      {a.sign(t, "nope", good), "n"},
		"forged signature": {other.sign(t, a.kid, good), "n"},
		"not a jwt":        {"abc.def", "n"},
		"tampered payload": {tamper(a.sign(t, a.kid, good)), "n"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			v := &appleid.Verifier{Audiences: []string{bundle}, KeysURL: a.server.URL}
			if _, err := v.Verify(context.Background(), c.token, c.nonce); !errors.Is(err, appleid.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func tamper(token string) string {
	b := []byte(token)
	i := len(b) / 2
	if b[i] == 'A' {
		b[i] = 'B'
	} else {
		b[i] = 'A'
	}
	return string(b)
}

func TestUnknownKidsDoNotHammerApple(t *testing.T) {
	a := newApple(t)
	v := &appleid.Verifier{Audiences: []string{bundle}, KeysURL: a.server.URL}
	for range 20 {
		_, _ = v.Verify(context.Background(), a.sign(t, "forged", claims(time.Now(), "n")), "n")
	}
	if n := a.fetches.Load(); n != 1 {
		t.Fatalf("fetched Apple's keys %d times for unknown kids, want 1", n)
	}
}

func TestAppleUnreachableIsNotAnInvalidToken(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer down.Close()
	a := newApple(t)
	v := &appleid.Verifier{Audiences: []string{bundle}, KeysURL: down.URL}
	_, err := v.Verify(context.Background(), a.sign(t, a.kid, claims(time.Now(), "n")), "n")
	if err == nil || errors.Is(err, appleid.ErrInvalid) {
		t.Fatalf("err = %v, want a fetch error", err)
	}
}

func TestNoAudienceMeansOff(t *testing.T) {
	if (&appleid.Verifier{}).Configured() {
		t.Fatal("a verifier with no client id must be off")
	}
}
