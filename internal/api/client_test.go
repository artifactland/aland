package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMeReturnsUser(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {"id": "u1", "username": "scott", "display_name": "Scott"},
			"meta": {"request_id": "req_ABC"}
		}`))
	}))
	defer srv.Close()

	c := &Client{APIBase: srv.URL, Token: "tok"}
	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if me.Username != "scott" {
		t.Errorf("username = %q, want scott", me.Username)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization header = %q, want Bearer tok", gotAuth)
	}
}

// 401 from any endpoint converts to ErrUnauthenticated regardless of the
// body shape (Doorkeeper sends OAuth-style errors, not the API envelope).
// The sentinel's Error() text guides the user to `aland login`.
func TestMeReturns401AsErrUnauthenticated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token","error_description":"token revoked"}`))
	}))
	defer srv.Close()

	c := &Client{APIBase: srv.URL, Token: "tok"}
	_, err := c.Me(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("expected ErrUnauthenticated, got %T: %v", err, err)
	}
}

// 422 (and other non-401 envelope errors) still surface as a typed *Err so
// callers can pattern-match on Code (e.g. visibility_downgrade_blocked,
// unknown_library).
func TestNon401EnvelopeErrorsStillTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":{"code":"unknown_library","message":"x"},"meta":{"request_id":"r"}}`))
	}))
	defer srv.Close()

	c := &Client{APIBase: srv.URL, Token: "tok"}
	_, err := c.Me(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	e, ok := err.(*Err)
	if !ok {
		t.Fatalf("expected *Err, got %T: %v", err, err)
	}
	if e.Code != "unknown_library" {
		t.Errorf("code = %q, want unknown_library", e.Code)
	}
}
