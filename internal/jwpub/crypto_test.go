package jwpub

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestCardString(t *testing.T) {
	cases := []struct {
		card Card
		want string
	}{
		{Card{1, "mwb26", 2026, 20260900}, "1_mwb26_2026_20260900"},
		{Card{1, "nwtsty", 2026, 0}, "1_nwtsty_2026"},
		{Card{0, "w13", 2013, 20130115}, "0_w13_2013_20130115"},
	}
	for _, c := range cases {
		if got := c.card.String(); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.card, got, c.want)
		}
	}
}

func TestNewCipherKnownVectors(t *testing.T) {
	cases := []struct {
		card    Card
		key, iv string
	}{
		{Card{1, "mwb26", 2026, 20260900}, "4d542734b2abee94ca897f7ea49ae821", "d0fa71758a4de744846f00b6e4801987"},
		{Card{1, "nwtsty", 2026, 0}, "79df371f3e76e450bb281f2b1fa29021", "5abb00e9dda748350030b482437de445"},
	}
	for _, c := range cases {
		ci := NewCipher(c.card)
		if got := hex.EncodeToString(ci.Key[:]); got != c.key {
			t.Errorf("%s key: got %s, want %s", c.card, got, c.key)
		}
		if got := hex.EncodeToString(ci.IV[:]); got != c.iv {
			t.Errorf("%s iv: got %s, want %s", c.card, got, c.iv)
		}
	}
}

// No ciphertext from a real publication lives here. The scheme is proved end to end by
// encrypting synthetic text and decrypting it back (TestEncryptDecryptRoundTrip below),
// which exercises the same key derivation, AES-CBC and inflate path.

func TestDecryptWrongKeyFails(t *testing.T) {
	blob, err := NewCipher(Card{1, "nwtsty", 2026, 0}).Encrypt([]byte("<span class=\"v\">texto sintético</span>"))
	if err != nil {
		t.Fatal(err)
	}
	// The card with a trailing "_0" is the mistake the issue-less rule avoids.
	if _, err := NewCipher(Card{1, "nwtsty_2026_0", 0, 0}).Decrypt(blob); err == nil {
		t.Fatal("expected an error with the wrong key")
	}
}

func TestEncryptRoundTrip(t *testing.T) {
	ci := NewCipher(Card{1, "w26", 2026, 20260700})
	for _, plain := range []string{"", "a", strings.Repeat("texto sintético ", 500), "<p id=\"p1\">¿Qué diría?</p>"} {
		blob, err := ci.Encrypt([]byte(plain))
		if err != nil {
			t.Fatal(err)
		}
		got, err := ci.DecryptString(blob)
		if err != nil {
			t.Fatal(err)
		}
		if got != plain {
			t.Fatalf("round trip: got %q, want %q", got, plain)
		}
	}
}

func TestDecryptRejectsBadSizes(t *testing.T) {
	ci := NewCipher(Card{1, "x", 2000, 0})
	for _, b := range [][]byte{nil, {1, 2, 3}} {
		if _, err := ci.Decrypt(b); err == nil {
			t.Errorf("expected error for %d bytes", len(b))
		}
	}
}
