package cli

import (
	"strings"
	"testing"
)

func TestResolveAPIBaseDefaultsToProductionHTTPS(t *testing.T) {
	t.Setenv("ALAND_API", "")

	got, err := resolveAPIBase("")
	if err != nil {
		t.Fatalf("resolveAPIBase: %v", err)
	}
	if got != "https://artifact.land" {
		t.Fatalf("resolveAPIBase = %q, want https://artifact.land", got)
	}
}

func TestResolveAPIBaseAllowsLoopbackHTTP(t *testing.T) {
	t.Setenv("ALAND_API", "http://127.0.0.1:3000/")

	got, err := resolveAPIBase("")
	if err != nil {
		t.Fatalf("resolveAPIBase: %v", err)
	}
	if got != "http://127.0.0.1:3000" {
		t.Fatalf("resolveAPIBase = %q, want trimmed loopback URL", got)
	}
}

func TestResolveAPIBaseRejectsRemoteHTTP(t *testing.T) {
	t.Setenv("ALAND_API", "")

	_, err := resolveAPIBase("http://staging.artifact.land")
	if err == nil {
		t.Fatal("resolveAPIBase should reject plain HTTP for non-loopback hosts")
	}
	if !strings.Contains(err.Error(), "plain HTTP is only allowed") {
		t.Fatalf("resolveAPIBase error = %q, want HTTP validation message", err)
	}
}

func TestEffectiveAPIBaseValidatesStoredProfilesToo(t *testing.T) {
	_, err := effectiveAPIBase("http://artifact.land", "")
	if err == nil {
		t.Fatal("effectiveAPIBase should reject insecure stored profile URLs")
	}
}
