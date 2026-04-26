package oauth

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoopbackHappyPath(t *testing.T) {
	server, err := StartLoopback("expected-state")
	if err != nil {
		t.Fatal(err)
	}

	// Fire the callback in a goroutine so Await can receive it.
	go func() {
		// small pause so the server is definitely listening; Serve is
		// started in a goroutine, so there's a theoretical race.
		time.Sleep(20 * time.Millisecond)
		resp, err := http.Get(server.RedirectURI() + "?code=hunter2&state=expected-state")
		if err != nil {
			t.Errorf("callback GET: %v", err)
			return
		}
		defer resp.Body.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	code, err := server.Await(ctx)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if code != "hunter2" {
		t.Errorf("code = %q, want hunter2", code)
	}
}

func TestLoopbackStateMismatchIsRejected(t *testing.T) {
	server, err := StartLoopback("the-right-state")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		resp, _ := http.Get(server.RedirectURI() + "?code=abc&state=WRONG")
		if resp != nil {
			defer resp.Body.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = server.Await(ctx)
	if err == nil {
		t.Fatal("expected state mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "state mismatch") {
		t.Errorf("error = %q, want state mismatch", err.Error())
	}
}

func TestLoopbackAuthorizeErrorPropagates(t *testing.T) {
	server, err := StartLoopback("s")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		resp, _ := http.Get(server.RedirectURI() + "?error=access_denied&error_description=User+declined")
		if resp != nil {
			defer resp.Body.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = server.Await(ctx)
	if err == nil {
		t.Fatal("expected authorize error, got nil")
	}
	if !strings.Contains(err.Error(), "access_denied") {
		t.Errorf("error = %q should contain access_denied", err.Error())
	}
}

func TestLoopbackFailurePageEscapesHTML(t *testing.T) {
	server, err := StartLoopback("s")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(20 * time.Millisecond)
	resp, err := http.Get(server.RedirectURI() + "?error=" + url.QueryEscape("<script>alert(1)</script>") + "&error_description=" + url.QueryEscape("<b>nope</b>"))
	if err != nil {
		t.Fatalf("callback GET: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	body := string(raw)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = server.Await(ctx)
	if err == nil {
		t.Fatal("expected authorize error, got nil")
	}
	if strings.Contains(body, "<script>alert(1)</script>") || strings.Contains(body, "<b>nope</b>") {
		t.Fatalf("failure page should escape HTML, got body %q", body)
	}
	if !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("failure page missing escaped error code: %q", body)
	}
	if !strings.Contains(body, "&lt;b&gt;nope&lt;/b&gt;") {
		t.Fatalf("failure page missing escaped description: %q", body)
	}
}

func TestLoopbackPrivateNetworkPreflight(t *testing.T) {
	server, err := StartLoopback("s")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, _ = server.Await(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	req, _ := http.NewRequest(http.MethodOptions, server.RedirectURI(), nil)
	req.Header.Set("Origin", "https://artifact.land")
	req.Header.Set("Access-Control-Request-Private-Network", "true")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS preflight: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Errorf("Access-Control-Allow-Private-Network = %q, want true", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
}

func TestLoopbackCORSHeadersOnCallback(t *testing.T) {
	server, err := StartLoopback("cors-state")
	if err != nil {
		t.Fatal(err)
	}

	var respHeaders http.Header
	go func() {
		time.Sleep(20 * time.Millisecond)
		resp, err := http.Get(server.RedirectURI() + "?code=tok&state=cors-state")
		if err != nil {
			t.Errorf("callback GET: %v", err)
			return
		}
		defer resp.Body.Close()
		respHeaders = resp.Header
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := server.Await(ctx); err != nil {
		t.Fatalf("Await: %v", err)
	}
	time.Sleep(20 * time.Millisecond) // let goroutine capture headers
	if got := respHeaders.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if got := respHeaders.Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Errorf("Access-Control-Allow-Private-Network = %q, want true", got)
	}
}

func TestLoopbackTimesOut(t *testing.T) {
	server, err := StartLoopback("s")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = server.Await(ctx)
	if err == nil {
		t.Fatal("expected context timeout error")
	}
}

func TestLoopbackRedirectURIIsLoopback(t *testing.T) {
	server, err := StartLoopback("s")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		// drain by canceling a fresh context
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, _ = server.Await(ctx)
	}()

	uri := server.RedirectURI()
	if !strings.HasPrefix(uri, "http://127.0.0.1:") {
		t.Errorf("redirect URI %q should start with http://127.0.0.1:", uri)
	}
	if !strings.HasSuffix(uri, "/callback") {
		t.Errorf("redirect URI %q should end with /callback", uri)
	}
}
