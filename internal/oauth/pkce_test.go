package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestNewPKCEGeneratesValidS256Pair(t *testing.T) {
	p, err := NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE: %v", err)
	}

	if len(p.Verifier) < 43 || len(p.Verifier) > 128 {
		t.Errorf("verifier length %d outside RFC-mandated 43..128", len(p.Verifier))
	}
	// Challenge must equal base64url(sha256(verifier)) per RFC 7636 §4.2.
	expected := base64.RawURLEncoding.EncodeToString(func() []byte {
		sum := sha256.Sum256([]byte(p.Verifier))
		return sum[:]
	}())
	if p.Challenge != expected {
		t.Errorf("challenge = %q, want %q", p.Challenge, expected)
	}
}

func TestPKCEValuesAreDistinctAcrossCalls(t *testing.T) {
	a, _ := NewPKCE()
	b, _ := NewPKCE()
	if a.Verifier == b.Verifier {
		t.Fatal("two successive PKCE verifiers collided — randomness broken")
	}
}

func TestRandomStateIsUrlSafe(t *testing.T) {
	s, err := RandomState()
	if err != nil {
		t.Fatal(err)
	}
	if s == "" {
		t.Fatal("empty state")
	}
	// Raw url-safe base64 uses only [A-Za-z0-9_-].
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			t.Fatalf("state %q contains disallowed char %q", s, r)
		}
	}
}
