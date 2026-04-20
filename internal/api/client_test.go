package api

import (
	"context"
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

func TestMeReturnsEnvelopeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"bad token"},"meta":{"request_id":"req_X"}}`))
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
	if e.Code != "unauthorized" {
		t.Errorf("code = %q, want unauthorized", e.Code)
	}
}
