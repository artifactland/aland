package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthorizeURLIncludesRequiredParams(t *testing.T) {
	c := &Client{APIBase: "https://artifact.land", ClientID: "artifactland-cli"}
	url := c.AuthorizeURL("http://127.0.0.1:5555/callback", "read publish:draft", "xyz", "abcchallenge")

	// Must start with the right endpoint.
	if !strings.HasPrefix(url, "https://artifact.land/oauth/authorize?") {
		t.Errorf("URL does not start with the authorize endpoint: %s", url)
	}

	// All critical params must appear.
	for _, needle := range []string{
		"response_type=code",
		"client_id=artifactland-cli",
		"scope=read+publish%3Adraft",
		"state=xyz",
		"code_challenge=abcchallenge",
		"code_challenge_method=S256",
		"redirect_uri=http%3A%2F%2F127.0.0.1%3A5555%2Fcallback",
	} {
		if !strings.Contains(url, needle) {
			t.Errorf("authorize URL missing %q\nfull URL: %s", needle, url)
		}
	}
}

func TestExchangeSuccess(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		receivedBody = r.Form.Encode()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "abc",
			"refresh_token": "def",
			"token_type": "Bearer",
			"expires_in": 3600,
			"scope": "read publish:draft",
			"created_at": 1700000000
		}`))
	}))
	defer srv.Close()

	c := &Client{APIBase: srv.URL, ClientID: "artifactland-cli", HTTP: &http.Client{Timeout: time.Second}}

	tok, err := c.Exchange(context.Background(), "code-123", "http://127.0.0.1:4000/callback", "verifier-xyz")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tok.AccessToken != "abc" || tok.RefreshToken != "def" || tok.ExpiresIn != 3600 {
		t.Errorf("token fields off: %+v", tok)
	}

	for _, needle := range []string{
		"grant_type=authorization_code",
		"code=code-123",
		"client_id=artifactland-cli",
		"code_verifier=verifier-xyz",
	} {
		if !strings.Contains(receivedBody, needle) {
			t.Errorf("request body missing %q (got %s)", needle, receivedBody)
		}
	}
}

func TestExchangeReturnsTypedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Code expired"}`))
	}))
	defer srv.Close()

	c := &Client{APIBase: srv.URL, ClientID: "artifactland-cli"}
	_, err := c.Exchange(context.Background(), "code", "http://127.0.0.1/callback", "verifier")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	te, ok := err.(*TokenError)
	if !ok {
		t.Fatalf("expected *TokenError, got %T: %v", err, err)
	}
	if te.ErrorCode != "invalid_grant" {
		t.Errorf("code = %q, want invalid_grant", te.ErrorCode)
	}
}
