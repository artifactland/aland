package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artifactland/aland/internal/project"
)

func TestInitScaffoldsHTMLProject(t *testing.T) {
	dir := t.TempDir()

	root := NewRoot("test")
	root.SetArgs([]string{"init", dir})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		t.Errorf("expected index.html: %v", err)
	}
	p, err := project.Load(dir)
	if err != nil {
		t.Fatalf("project.Load: %v", err)
	}
	a, err := p.Only()
	if err != nil {
		t.Fatalf("Only: %v", err)
	}
	if a.SourceFile != "index.html" {
		t.Errorf("source_file = %q, want index.html", a.SourceFile)
	}
}

func TestInitJSXFlagCreatesJSXStarter(t *testing.T) {
	dir := t.TempDir()

	root := NewRoot("test")
	root.SetArgs([]string{"init", "--jsx", dir})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "index.jsx"))
	if err != nil {
		t.Fatalf("reading index.jsx: %v", err)
	}
	if !strings.Contains(string(content), "useState") {
		t.Errorf("jsx starter should use React; got: %s", content)
	}
}

func TestInitRefusesToOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	p := project.New(dir)
	if err := p.Add(project.DefaultArtifactName, &project.Artifact{SourceFile: "already.html"}); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}

	root := NewRoot("test")
	root.SetArgs([]string{"init", dir})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected init to refuse overwrite, got nil error")
	}
	if !strings.Contains(err.Error(), "already contains") {
		t.Errorf("unexpected error: %v", err)
	}
}
