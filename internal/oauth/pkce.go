// Package oauth provides the CLI side of the OAuth 2.1 Authorization Code
// flow with PKCE. It speaks to Doorkeeper running on the Rails server;
// artifactland-cli is pre-registered there with loopback redirect URIs.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// PKCE carries the verifier + challenge generated for one Authorization Code
// request. Verifier stays client-side until the token exchange; Challenge
// travels to the authorize endpoint.
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE builds a fresh verifier/challenge pair. The verifier is 64 random
// bytes, base64url-encoded (~86 chars), well within the spec's 43–128 char
// window. Challenge is SHA256(verifier) base64url-encoded — required because
// the server enforces S256 and rejects plain.
func NewPKCE() (*PKCE, error) {
	buf := make([]byte, 64)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("reading random bytes for PKCE verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(buf)

	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])

	return &PKCE{Verifier: verifier, Challenge: challenge}, nil
}

// RandomState returns a URL-safe random string used as the OAuth `state`
// parameter. 24 bytes ≈ 32 chars after base64url, which is plenty of entropy
// against a forged callback.
func RandomState() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("reading random bytes for state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
