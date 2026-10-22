// Package secretbox encrypts platform credentials before they reach the
// database.
//
// The key lives outside the database (an environment variable from a sealed
// secret), so a dump of the database alone yields ciphertext. Every blob is
// also bound to associated data — the id of the row it belongs to — so a blob
// copied onto another row fails to open instead of quietly granting that row
// someone else's account.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// version prefixes every blob. There is one key today; when a second arrives
// for rotation, the byte says which key sealed a blob, and old blobs stay
// readable while they are re-sealed.
const version byte = 1

// KeySize is the length of an AES-256 key.
const KeySize = 32

var ErrOpen = errors.New("secretbox: cannot open: wrong key, wrong associated data, or corrupt")

type Box struct {
	aead cipher.AEAD
}

// New builds a Box from a raw 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("secretbox: key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	return &Box{aead: aead}, nil
}

// NewFromBase64 builds a Box from the key as it is stored in the environment.
// Generate one with `openssl rand -base64 32`.
func NewFromBase64(encoded string) (*Box, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("secretbox: key is not valid base64: %w", err)
	}
	return New(key)
}

// Seal encrypts plaintext, bound to aad. The output is version || nonce ||
// ciphertext. A fresh random nonce per call is safe for GCM well past the
// number of credentials this application will ever store.
func (b *Box) Seal(plaintext, aad []byte) []byte {
	nonce := make([]byte, b.aead.NonceSize(), 1+b.aead.NonceSize()+len(plaintext)+b.aead.Overhead())
	// crypto/rand.Read never returns an error since Go 1.24; it crashes the
	// program instead of handing back a predictable nonce.
	_, _ = rand.Read(nonce)

	out := append([]byte{version}, nonce...)
	return b.aead.Seal(out, nonce, plaintext, aad)
}

// Open reverses Seal. Any mismatch — key, aad, or a flipped byte — returns
// ErrOpen without saying which, since the caller can do nothing different.
func (b *Box) Open(sealed, aad []byte) ([]byte, error) {
	ns := b.aead.NonceSize()
	if len(sealed) < 1+ns+b.aead.Overhead() || sealed[0] != version {
		return nil, ErrOpen
	}
	plaintext, err := b.aead.Open(nil, sealed[1:1+ns], sealed[1+ns:], aad)
	if err != nil {
		return nil, ErrOpen
	}
	return plaintext, nil
}
