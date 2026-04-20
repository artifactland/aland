package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestContextCommandEmitsMarkdown(t *testing.T) {
	root := NewRoot("test")
	root.SetArgs([]string{"context"})

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	body := out.String()
	for _, needle := range []string{
		"Safety invariant",
		"Supported runtime",
		"unknown_library",
		"connect-src",
		"aland push",
	} {
		if !strings.Contains(body, needle) {
			t.Errorf("context output missing %q", needle)
		}
	}
}

func TestSkillInstallPrintMode(t *testing.T) {
	root := NewRoot("test")
	root.SetArgs([]string{"skill", "install", "--print"})

	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "artifactland") {
		t.Errorf("skill content should be printed to stdout; got: %s", out.String())
	}
	if !strings.Contains(errBuf.String(), ".claude/skills/artifactland") {
		t.Errorf("destination path should be printed to stderr; got: %s", errBuf.String())
	}
}
