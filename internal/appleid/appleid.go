// Package appleid verifies Sign in with Apple identity tokens (AUTH-001).
//
// The iOS app and the web page each get an identity token from Apple and send it here. It is a
// JWT that Apple signs with RS256; the keys are published at Keys. Verifying it needs no private
// key of ours: a .p8 key is only for exchanging an authorization code or revoking a token, which
// sign-in does not do. Same approach as Khepri and Aaru.
//
// Checked against https://developer.apple.com/documentation/signinwithapple/verifying-a-user
// (2026-11-16): signature by a key from Keys, iss is Issuer, aud is our client id (the app's
// bundle id, or the web Services ID), exp not passed, nonce matches what the client sent.
package appleid

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	Issuer = "https://appleid.apple.com"
	Keys   = "https://appleid.apple.com/auth/keys"
)

// ErrInvalid is any token that fails a check. The caller answers 401 and says no more: which
// check failed is no business of someone presenting a forged token.
var ErrInvalid = errors.New("invalid identity token")

// Identity is what a verified token says about the person.
type Identity struct {
	// Subject is Apple's stable id for the person across this team's apps. Accounts are keyed
	// on it, never on Email.
	Subject string
	// Email as Apple shares it: the real address or a private relay one. Empty if the token
	// carries none.
	Email string
	// EmailVerified is Apple's word that the address is the person's. Apple marks every email
	// it shares verified; the flag is still checked before an email claims an existing user.
	EmailVerified bool
}

// Verifier checks identity tokens. Audiences are the client ids accepted: the app's bundle id
// and, for the web, its Services ID.
type Verifier struct {
	Audiences []string
	// KeysURL defaults to Keys; tests point it at a stub.
	KeysURL string
	Client  *http.Client
	// Now defaults to time.Now; tests move it.
	Now func() time.Time

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

// keyTTL is how long Apple's key set is trusted before it is fetched again. Apple rotates keys
// rarely; a kid not in the cached set triggers a fetch sooner (refetchFloor apart, so a stream
// of forged kids cannot make us hammer Apple).
const (
	keyTTL       = 6 * time.Hour
	refetchFloor = time.Minute
	leeway       = time.Minute
)

// Configured reports whether any client id is set; with none, Apple sign-in is off.
func (v *Verifier) Configured() bool { return v != nil && len(v.Audiences) > 0 }

// Verify checks token and that its nonce is the SHA-256 (hex) of rawNonce, the random string
// the client generated and sent Apple hashed. That binds the token to this one sign-in, so a
// token lifted from elsewhere cannot be replayed here.
func (v *Verifier) Verify(ctx context.Context, token, rawNonce string) (Identity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || rawNonce == "" {
		return Identity{}, ErrInvalid
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if decodeSegment(parts[0], &header) != nil || header.Alg != "RS256" || header.Kid == "" {
		return Identity{}, ErrInvalid
	}
	key, err := v.key(ctx, header.Kid)
	if err != nil {
		return Identity{}, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Identity{}, ErrInvalid
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig) != nil {
		return Identity{}, ErrInvalid
	}

	var claims struct {
		Iss           string       `json:"iss"`
		Sub           string       `json:"sub"`
		Aud           audience     `json:"aud"`
		Exp           float64      `json:"exp"`
		Nonce         string       `json:"nonce"`
		Email         string       `json:"email"`
		EmailVerified boolOrString `json:"email_verified"`
	}
	if decodeSegment(parts[1], &claims) != nil {
		return Identity{}, ErrInvalid
	}
	now := v.now()
	if claims.Iss != Issuer || claims.Sub == "" || !claims.Aud.any(v.Audiences) {
		return Identity{}, ErrInvalid
	}
	if claims.Exp == 0 || now.After(time.Unix(int64(claims.Exp), 0).Add(leeway)) {
		return Identity{}, ErrInvalid
	}
	want := sha256.Sum256([]byte(rawNonce))
	if claims.Nonce != hex.EncodeToString(want[:]) {
		return Identity{}, ErrInvalid
	}
	return Identity{
		Subject:       claims.Sub,
		Email:         strings.ToLower(strings.TrimSpace(claims.Email)),
		EmailVerified: bool(claims.EmailVerified),
	}, nil
}

func (v *Verifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

// key returns the public key for kid, fetching Apple's set when the cache is old or lacks it.
// A failed fetch is an error, not ErrInvalid: the token may be fine and Apple unreachable.
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now()
	stale := now.Sub(v.fetched) > keyTTL
	if k, ok := v.keys[kid]; ok && !stale {
		return k, nil
	}
	if !stale && now.Sub(v.fetched) < refetchFloor {
		return nil, ErrInvalid
	}
	keys, err := v.fetch(ctx)
	if err != nil {
		if k, ok := v.keys[kid]; ok {
			return k, nil // Apple unreachable: a key we already trusted still verifies.
		}
		return nil, err
	}
	v.keys, v.fetched = keys, now
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, ErrInvalid
}

func (v *Verifier) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	url := v.KeysURL
	if url == "" {
		url = Keys
	}
	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("apple keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apple keys: status %d", resp.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return nil, fmt.Errorf("apple keys: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if errN != nil || errE != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return nil, errors.New("apple keys: no usable key")
	}
	return keys, nil
}

func decodeSegment(seg string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// audience is aud as a string or a list of strings; Apple sends a string.
type audience []string

func (a *audience) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

func (a audience) any(allowed []string) bool {
	for _, got := range a {
		for _, ok := range allowed {
			if got == ok {
				return true
			}
		}
	}
	return false
}

// boolOrString: Apple sends some booleans as true and some as "true".
type boolOrString bool

func (b *boolOrString) UnmarshalJSON(data []byte) error {
	var v bool
	if json.Unmarshal(data, &v) == nil {
		*b = boolOrString(v)
		return nil
	}
	var s string
	_ = json.Unmarshal(data, &s)
	*b = boolOrString(strings.EqualFold(s, "true"))
	return nil
}
