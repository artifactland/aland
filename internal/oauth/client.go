package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to one OAuth provider. APIBase is the server root (e.g.
// https://artifact.land); endpoint paths are fixed by Doorkeeper.
type Client struct {
	APIBase  string
	ClientID string
	HTTP     *http.Client
}

// TokenResponse mirrors Doorkeeper's /oauth/token success payload.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	CreatedAt    int64  `json:"created_at"`
}

// TokenError matches Doorkeeper's error body shape. Used for the grant and
// revoke endpoints alike.
type TokenError struct {
	ErrorCode        string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (e *TokenError) Error() string {
	if e.ErrorDescription != "" {
		return fmt.Sprintf("%s: %s", e.ErrorCode, e.ErrorDescription)
	}
	return e.ErrorCode
}

// AuthorizeURL builds the URL we open in the browser.
func (c *Client) AuthorizeURL(redirectURI, scope, state, codeChallenge string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", scope)
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")

	return strings.TrimRight(c.APIBase, "/") + "/oauth/authorize?" + q.Encode()
}

// Exchange trades a one-time authorization code + PKCE verifier for tokens.
func (c *Client) Exchange(ctx context.Context, code, redirectURI, codeVerifier string) (*TokenResponse, error) {
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", code)
	body.Set("redirect_uri", redirectURI)
	body.Set("client_id", c.ClientID)
	body.Set("code_verifier", codeVerifier)

	return c.postToken(ctx, body)
}

// Refresh uses a refresh_token to mint a new access token. The server rotates
// the refresh token on success — callers must persist the new one.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("refresh_token", refreshToken)
	body.Set("client_id", c.ClientID)

	return c.postToken(ctx, body)
}

// Revoke invalidates a token at /oauth/revoke. Used by `aland logout`.
func (c *Client) Revoke(ctx context.Context, token string) error {
	body := url.Values{}
	body.Set("token", token)
	body.Set("client_id", c.ClientID)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		strings.TrimRight(c.APIBase, "/")+"/oauth/revoke",
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("POST /oauth/revoke: %w", err)
	}
	defer resp.Body.Close()

	// Doorkeeper returns 200 for revoke regardless of whether the token was
	// valid — treat any non-5xx as success.
	if resp.StatusCode >= 500 {
		return fmt.Errorf("revoke returned %d", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *Client) postToken(ctx context.Context, body url.Values) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		strings.TrimRight(c.APIBase, "/")+"/oauth/token",
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST /oauth/token: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading /oauth/token response: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		te := &TokenError{}
		if err := json.Unmarshal(raw, te); err == nil && te.ErrorCode != "" {
			return nil, te
		}
		return nil, fmt.Errorf("/oauth/token returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out TokenResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	return &out, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}
