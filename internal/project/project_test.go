package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	p := New(dir)
	if err := p.Add("deep-field", &Artifact{
		SourceFile: "deep-field/index.jsx",
		Title:      "Deep Field",
		Tags:       []string{"game", "visualization"},
		Visibility: "public_visibility",
		ForkOf:     &ForkOf{PostID: "pst_1", Username: "alice", Slug: "thing", Title: "Thing"},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := p.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(loaded.Artifacts); got != 1 {
		t.Fatalf("artifacts = %d, want 1", got)
	}
	a := loaded.Get("deep-field")
	if a == nil {
		t.Fatal("deep-field artifact missing after reload")
	}
	if a.Title != "Deep Field" {
		t.Errorf("title = %q, want Deep Field", a.Title)
	}
	if a.ForkOf == nil || a.ForkOf.Username != "alice" {
		t.Errorf("fork_of not preserved: %+v", a.ForkOf)
	}
	if a.Name() != "deep-field" {
		t.Errorf("Name() = %q, want deep-field", a.Name())
	}
	if loaded.Version != CurrentVersion {
		t.Errorf("version = %q, want %q", loaded.Version, CurrentVersion)
	}
}

func TestLoadMissingReturnsSentinel(t *testing.T) {
	_, err := Load(t.TempDir())
	if !errors.Is(err, ErrNotAProject) {
		t.Errorf("expected ErrNotAProject, got %v", err)
	}
}

func TestLoadRejectsMissingVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, Filename)
	if err := os.WriteFile(path, []byte(`{"artifacts":{"main":{"source_file":"x.html"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("expected version-missing error, got %v", err)
	}
}

func TestLoadRejectsUnknownVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, Filename)
	if err := os.WriteFile(path, []byte(`{"version":"99","artifacts":{"main":{"source_file":"x.html"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "99") {
		t.Errorf("expected unknown-version error, got %v", err)
	}
}

func TestExistsReflectsFile(t *testing.T) {
	dir := t.TempDir()
	if Exists(dir) {
		t.Fatal("Exists true on empty dir")
	}
	p := New(dir)
	if err := p.Add("main", &Artifact{SourceFile: "x.html"}); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	if !Exists(dir) {
		t.Fatal("Exists false after Save")
	}
}

// ---- path validator ------------------------------------------------------

func TestAddRejectsAbsoluteSourceFile(t *testing.T) {
	dir := t.TempDir()
	p := New(dir)
	err := p.Add("evil", &Artifact{SourceFile: "/etc/passwd"})
	if err == nil || !strings.Contains(err.Error(), "relative") {
		t.Errorf("expected absolute-path rejection, got %v", err)
	}
}

func TestAddRejectsDotDotEscape(t *testing.T) {
	dir := t.TempDir()
	p := New(dir)
	for _, escape := range []string{"../secret.txt", "../../.ssh/id_rsa", "foo/../../bar"} {
		t.Run(escape, func(t *testing.T) {
			if err := p.Add("evil", &Artifact{SourceFile: escape}); err == nil {
				t.Errorf("%q should have been rejected", escape)
			}
		})
	}
}

func TestAddAcceptsInnerPaths(t *testing.T) {
	dir := t.TempDir()
	p := New(dir)
	for _, ok := range []string{"index.html", "sub/thing.jsx", "a/b/c/deep.html", "foo/../bar.html"} {
		if err := p.Add("ok", &Artifact{SourceFile: ok}); err != nil {
			t.Errorf("%q should have been accepted: %v", ok, err)
		}
	}
}

func TestLoadRejectsMaliciousSourceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, Filename)
	poisoned := `{
		"version": "1",
		"artifacts": {
			"evil": { "source_file": "../../etc/passwd" }
		}
	}`
	if err := os.WriteFile(path, []byte(poisoned), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("expected load-time rejection of escaping path, got %v", err)
	}
}

func TestLoadRejectsSymlinkEscape(t *testing.T) {
	// Arrange a project dir with a source_file that points at a symlink
	// whose target is outside the dir. On systems without symlink support,
	// skip cleanly.
	dir := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("shh"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "hack.html")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	path := filepath.Join(dir, Filename)
	doc := `{"version":"1","artifacts":{"evil":{"source_file":"hack.html"}}}`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected symlink-escape rejection, got %v", err)
	}
}

// ---- resolution ----------------------------------------------------------

func TestOnlyReturnsSingleArtifact(t *testing.T) {
	dir := t.TempDir()
	p := New(dir)
	if err := p.Add("main", &Artifact{SourceFile: "index.html"}); err != nil {
		t.Fatal(err)
	}
	a, err := p.Only()
	if err != nil {
		t.Fatalf("Only: %v", err)
	}
	if a.Name() != "main" {
		t.Errorf("Only().Name() = %q, want main", a.Name())
	}
}

func TestOnlyErrorsOnMultiArtifact(t *testing.T) {
	dir := t.TempDir()
	p := New(dir)
	_ = p.Add("a", &Artifact{SourceFile: "a.html"})
	_ = p.Add("b", &Artifact{SourceFile: "b.html"})
	_, err := p.Only()
	if err == nil || !strings.Contains(err.Error(), "2 artifacts") {
		t.Errorf("expected multi-artifact error, got %v", err)
	}
	// Error message should list both names so the user can pick one.
	if !strings.Contains(err.Error(), "a.html") || !strings.Contains(err.Error(), "b.html") {
		t.Errorf("error should list candidates: %v", err)
	}
}

func TestResolveByNameOrPath(t *testing.T) {
	dir := t.TempDir()
	p := New(dir)
	_ = p.Add("deep-field", &Artifact{SourceFile: "deep-field/index.html"})
	_ = p.Add("epic-24", &Artifact{SourceFile: "announcements/epic-24.html"})

	// By name
	a, err := p.Resolve("deep-field")
	if err != nil || a.Name() != "deep-field" {
		t.Errorf("Resolve(name): a=%v err=%v", a, err)
	}
	// By path (exact match against stored value)
	a, err = p.Resolve("announcements/epic-24.html")
	if err != nil || a.Name() != "epic-24" {
		t.Errorf("Resolve(path): a=%v err=%v", a, err)
	}
	// By path with ./ prefix (gets normalized)
	a, err = p.Resolve("./announcements/epic-24.html")
	if err != nil || a.Name() != "epic-24" {
		t.Errorf("Resolve(./path): a=%v err=%v", a, err)
	}
	// Miss — error should include candidates.
	_, err = p.Resolve("nope")
	if err == nil || !strings.Contains(err.Error(), "deep-field") {
		t.Errorf("miss should list candidates: %v", err)
	}
}

func TestResolveEmptyFallsBackToOnly(t *testing.T) {
	dir := t.TempDir()
	p := New(dir)
	_ = p.Add("main", &Artifact{SourceFile: "index.html"})
	a, err := p.Resolve("")
	if err != nil || a.Name() != "main" {
		t.Errorf("Resolve(\"\"): a=%v err=%v", a, err)
	}
}
