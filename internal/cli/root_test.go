package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	root := NewRoot("1.2.3")
	root.SetArgs([]string{"version"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got := strings.TrimSpace(out.String()); got != "1.2.3" {
		t.Errorf("version output = %q, want 1.2.3", got)
	}
}

func TestRootHelpMentionsArtifactLand(t *testing.T) {
	root := NewRoot("dev")
	root.SetArgs([]string{"--help"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	_ = root.Execute()

	help := out.String()
	if !strings.Contains(help, "artifact.land") {
		t.Errorf("help text should mention artifact.land; got:\n%s", help)
	}
}

func TestGlobalsDefaultsWhenNotSet(t *testing.T) {
	g := Globals(nil)
	if g == nil {
		t.Fatal("Globals(nil) returned nil, want empty struct")
	}
	if g.Profile != "" {
		t.Errorf("default Profile should be empty on an empty struct, got %q", g.Profile)
	}
}
