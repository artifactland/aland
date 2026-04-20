// Package api is a typed client for /api/v1. Every response comes back in
// the envelope { data|error, meta: { request_id } }; helpers unwrap that so
// callers work with bare domain types.
package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client holds the API base + bearer token. Zero value isn't useful; call
// New() or construct directly.
type Client struct {
	APIBase string
	Token   string
	HTTP    *http.Client
}

// Envelope is the outer shape of every API response.
type Envelope[T any] struct {
	Data  T    `json:"data"`
	Meta  Meta `json:"meta"`
	Error *Err `json:"error,omitempty"`
}

// Meta carries cross-cutting response metadata (request id, pagination).
type Meta struct {
	RequestID  string      `json:"request_id"`
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination is returned by list endpoints.
type Pagination struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Total   int `json:"total"`
}

// Err mirrors `{ error: { code, message, details } }` in the envelope. It
// implements error so callers can `return err` directly.
type Err struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *Err) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// User is what /users/me and /users/:username return.
type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name,omitempty"`
	Email       string `json:"email,omitempty"`
	PostsCount  int    `json:"posts_count,omitempty"`
}

// Post is the metadata shape returned by show / index endpoints. Fields that
// aren't universally populated are nullable.
type Post struct {
	ID            string   `json:"id"`
	Slug          string   `json:"slug"`
	Title         string   `json:"title"`
	Description   string   `json:"description,omitempty"`
	FileType      string   `json:"file_type"`
	Visibility    string   `json:"visibility"`
	Tags          []string `json:"tags"`
	LikesCount    int      `json:"likes_count"`
	CommentsCount int      `json:"comments_count"`
	PublishedAt   *string  `json:"published_at"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	User          User     `json:"user"`
	URLs          URLs     `json:"urls"`
	ForkOfID      string   `json:"remix_of_id,omitempty"`
}

// URLs is the urls block on a Post.
type URLs struct {
	Canonical string `json:"canonical,omitempty"`
	Web       string `json:"web,omitempty"`
	Cover     string `json:"cover,omitempty"`
}

// Source is the payload of GET /posts/:id/source.
type Source struct {
	ID       string `json:"id"`
	FileType string `json:"file_type"`
	Source   string `json:"source"`
}

// Me returns the authenticated user's profile.
func (c *Client) Me(ctx context.Context) (*User, error) {
	var env Envelope[User]
	if err := c.getJSON(ctx, "/api/v1/users/me", &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// GetPostByRef resolves either a UUID or a `user/slug` pair to post metadata.
// Callers should pass the UUID when they have one — the named endpoint is a
// convenience for fresh ref strings from `aland fork @user/slug`.
func (c *Client) GetPostByRef(ctx context.Context, username, slug string) (*Post, error) {
	var env Envelope[Post]
	if err := c.getJSON(ctx, fmt.Sprintf("/api/v1/users/%s/posts/%s", username, slug), &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// GetPost fetches by UUID.
func (c *Client) GetPost(ctx context.Context, id string) (*Post, error) {
	var env Envelope[Post]
	if err := c.getJSON(ctx, "/api/v1/posts/"+id, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// GetSource fetches the raw source file for a post the caller can see.
func (c *Client) GetSource(ctx context.Context, id string) (*Source, error) {
	var env Envelope[Source]
	if err := c.getJSON(ctx, "/api/v1/posts/"+id+"/source", &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// ForkPost calls POST /posts/:id/fork. Attributes (title/description/etc)
// are optional and fall through to the source post's values when omitted.
func (c *Client) ForkPost(ctx context.Context, id string, attrs map[string]any) (*Post, error) {
	body := map[string]any{}
	if len(attrs) > 0 {
		body["post"] = attrs
	}
	var env Envelope[Post]
	if err := c.postJSON(ctx, "/api/v1/posts/"+id+"/fork", body, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// CreateDraft uploads a new draft via JSON + base64 (the path that doesn't
// require multipart). Always produces a draft — the server ignores any
// client-supplied published_at.
func (c *Client) CreateDraft(ctx context.Context, filename string, sourceBytes []byte, attrs map[string]any) (*Post, error) {
	body := map[string]any{
		"source":   base64.StdEncoding.EncodeToString(sourceBytes),
		"filename": filename,
	}
	if len(attrs) > 0 {
		body["post"] = attrs
	}
	var env Envelope[Post]
	if err := c.postJSON(ctx, "/api/v1/posts", body, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// UpdateDraft PATCHes an existing draft with new source and/or metadata. If
// sourceBytes is non-nil, the file is re-compiled server-side via the same
// pipeline as CreateDraft.
func (c *Client) UpdateDraft(ctx context.Context, id, filename string, sourceBytes []byte, attrs map[string]any) (*Post, error) {
	body := map[string]any{}
	if sourceBytes != nil {
		body["source"] = base64.StdEncoding.EncodeToString(sourceBytes)
		body["filename"] = filename
	}
	if len(attrs) > 0 {
		body["post"] = attrs
	}
	var env Envelope[Post]
	if err := c.doJSON(ctx, http.MethodPatch, "/api/v1/posts/"+id, body, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) postJSON(ctx context.Context, path string, body, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, body, out)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling %s body: %w", path, err)
		}
		reqBody = bytesReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.APIBase, "/")+path, reqBody)
	if err != nil {
		return err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading %s response: %w", path, err)
	}

	if resp.StatusCode/100 != 2 {
		var errEnv Envelope[json.RawMessage]
		if jerr := json.Unmarshal(raw, &errEnv); jerr == nil && errEnv.Error != nil {
			return errEnv.Error
		}
		return fmt.Errorf("%s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("parsing %s response: %w", path, err)
		}
	}
	return nil
}

// bytesReader avoids importing bytes for one reader; keeps the package's
// direct imports smaller.
func bytesReader(b []byte) io.Reader {
	return &byteSliceReader{b: b}
}

type byteSliceReader struct {
	b []byte
	i int
}

func (r *byteSliceReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}
