package secretbox

import (
	"bytes"
	"errors"
	"testing"
)

func newBox(t *testing.T, fill byte) *Box {
	t.Helper()
	b, err := New(bytes.Repeat([]byte{fill}, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRoundTrip(t *testing.T) {
	b := newBox(t, 1)
	sealed := b.Seal([]byte("webhook-secret"), []byte("row-1"))

	got, err := b.Open(sealed, []byte("row-1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "webhook-secret" {
		t.Fatalf("got %q", got)
	}
}

func TestSealIsRandomised(t *testing.T) {
	b := newBox(t, 1)
	if bytes.Equal(b.Seal([]byte("x"), nil), b.Seal([]byte("x"), nil)) {
		t.Fatal("two seals of the same plaintext are identical; the nonce is not random")
	}
}

func TestOpenRejects(t *testing.T) {
	b := newBox(t, 1)
	sealed := b.Seal([]byte("webhook-secret"), []byte("row-1"))

	flipped := bytes.Clone(sealed)
	flipped[len(flipped)-1] ^= 0xff

	cases := map[string]struct {
		box    *Box
		sealed []byte
		aad    string
	}{
		// The case the associated data exists for: a blob moved to another row.
		"other row":   {b, sealed, "row-2"},
		"other key":   {newBox(t, 2), sealed, "row-1"},
		"tampered":    {b, flipped, "row-1"},
		"truncated":   {b, sealed[:5], "row-1"},
		"bad version": {b, append([]byte{9}, sealed[1:]...), "row-1"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := tc.box.Open(tc.sealed, []byte(tc.aad)); !errors.Is(err, ErrOpen) {
				t.Fatalf("got %v, want ErrOpen", err)
			}
		})
	}
}

func TestKeyLength(t *testing.T) {
	if _, err := New(make([]byte, 16)); err == nil {
		t.Fatal("accepted a 16-byte key; this box is AES-256 only")
	}
	if _, err := NewFromBase64("not base64!"); err == nil {
		t.Fatal("accepted a key that is not base64")
	}
}
