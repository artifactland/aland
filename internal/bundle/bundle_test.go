package bundle

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Magic-byte prefix for a "PNG" — the linter doesn't actually validate
// pixel data, just structural rules.
var fakePNG = append([]byte("\x89PNG\r\n\x1A\n"), make([]byte, 16)...)

// --- Validate happy paths ------------------------------------------------

func TestValidate_HTMLOnly(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html": []byte("<!doctype html><body>Hi"),
	})
	r := mustValidate(t, dir)
	if r.HasErrors() {
		t.Fatalf("expected no errors, got %v", r.Errors())
	}
	if r.EntryPath != "index.html" {
		t.Errorf("expected entry index.html, got %q", r.EntryPath)
	}
	if r.FileCount != 1 {
		t.Errorf("expected 1 file, got %d", r.FileCount)
	}
}

func TestValidate_HTMLWithImage(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html":      []byte(`<img src="assets/hero.png">`),
		"assets/hero.png": fakePNG,
	})
	r := mustValidate(t, dir)
	if r.HasErrors() {
		t.Fatalf("expected no errors, got %v", r.Errors())
	}
	if r.FileCount != 2 {
		t.Errorf("expected 2 files, got %d", r.FileCount)
	}
}

func TestValidate_NestedAssets(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html":               []byte(`<img src="assets/photos/sunset.png">`),
		"assets/photos/sunset.png": fakePNG,
	})
	r := mustValidate(t, dir)
	if r.HasErrors() {
		t.Fatalf("expected no errors, got %v", r.Errors())
	}
}

// --- Validate failure modes ----------------------------------------------

func TestValidate_NoEntry(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"assets/orphan.png": fakePNG,
	})
	r := mustValidate(t, dir)
	if !hasError(r, "no_entry") {
		t.Errorf("expected no_entry error, got %v", r.Issues)
	}
}

func TestValidate_BadStructure_RootNonEntry(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html": []byte("<html>"),
		"extra.html": []byte("<html>"),
	})
	r := mustValidate(t, dir)
	if !hasError(r, "bad_structure") {
		t.Errorf("expected bad_structure error on extra.html, got %v", r.Issues)
	}
}

func TestValidate_BadStructure_NestedOutsideAssets(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html":      []byte("<html>"),
		"images/hero.png": fakePNG,
	})
	r := mustValidate(t, dir)
	if !hasError(r, "bad_structure") {
		t.Errorf("expected bad_structure error on images/, got %v", r.Issues)
	}
}

func TestValidate_DisallowedType(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html":      []byte("<html>"),
		"assets/data.txt": []byte("hi"),
	})
	r := mustValidate(t, dir)
	if !hasError(r, "disallowed_type") {
		t.Errorf("expected disallowed_type error, got %v", r.Issues)
	}
}

func TestValidate_MultipleEntries(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html": []byte("<html>"),
		"index.jsx":  []byte("export default null"),
	})
	r := mustValidate(t, dir)
	if !hasError(r, "multiple_entries") {
		t.Errorf("expected multiple_entries error, got %v", r.Issues)
	}
}

// --- Warnings ------------------------------------------------------------

func TestValidate_ImageOversized(t *testing.T) {
	big := append([]byte("\x89PNG\r\n\x1A\n"), make([]byte, ImageWeightWarnBytes+1)...)
	dir := writeBundle(t, map[string][]byte{
		"index.html":     []byte("<html>"),
		"assets/big.png": big,
	})
	r := mustValidate(t, dir)
	if r.HasErrors() {
		t.Fatalf("oversized image should warn, not error, got %v", r.Errors())
	}
	if !hasWarning(r, "image_oversized") {
		t.Errorf("expected image_oversized warning, got %v", r.Warnings())
	}
}

func TestValidate_OverFreeCap_Warns(t *testing.T) {
	body := bytes.Repeat([]byte("a"), int(FreeCaps.MaxBytes)+10_000)
	dir := writeBundle(t, map[string][]byte{
		"index.html": body,
	})
	r := mustValidate(t, dir)
	if r.HasErrors() {
		t.Fatalf("over-free should warn, not error, got %v", r.Errors())
	}
	if !hasWarning(r, "over_free_cap") {
		t.Errorf("expected over_free_cap warning, got %v", r.Warnings())
	}
}

func TestValidate_OverProCap_Errors(t *testing.T) {
	body := bytes.Repeat([]byte("a"), int(ProCaps.MaxBytes)+10_000)
	dir := writeBundle(t, map[string][]byte{
		"index.html": body,
	})
	r := mustValidate(t, dir)
	if !hasError(r, "too_large") {
		t.Errorf("expected too_large error, got %v", r.Issues)
	}
}

// --- Reference integrity -------------------------------------------------

func TestValidate_BrokenLocalReference(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html":         []byte(`<img src="assets/missing.png">`),
		"assets/present.png": fakePNG,
	})
	r := mustValidate(t, dir)
	if !hasError(r, "broken_reference") {
		t.Errorf("expected broken_reference for assets/missing.png, got %v", r.Issues)
	}
}

func TestValidate_ExternalReferenceWarns(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html": []byte(`<img src="https://example.com/photo.jpg">`),
	})
	r := mustValidate(t, dir)
	if r.HasErrors() {
		t.Fatalf("external URL should warn, not error, got %v", r.Errors())
	}
	if !hasWarning(r, "external_reference") {
		t.Errorf("expected external_reference warning, got %v", r.Warnings())
	}
}

func TestValidate_QueryStringStripped(t *testing.T) {
	// `assets/hero.png?v=2` should resolve to assets/hero.png — query
	// params get stripped before file-existence check.
	dir := writeBundle(t, map[string][]byte{
		"index.html":      []byte(`<img src="assets/hero.png?v=2">`),
		"assets/hero.png": fakePNG,
	})
	r := mustValidate(t, dir)
	if r.HasErrors() {
		t.Fatalf("query-stripped ref should resolve, got %v", r.Errors())
	}
}

func TestValidate_DataURIIgnored(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html": []byte(`<img src="data:image/png;base64,iVBORw0KGgo=">`),
	})
	r := mustValidate(t, dir)
	if r.HasErrors() {
		t.Fatalf("data: URI must not produce a broken_reference, got %v", r.Errors())
	}
}

// --- Build ---------------------------------------------------------------

func TestBuild_Deterministic(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html":      []byte(`<img src="assets/hero.png">`),
		"assets/hero.png": fakePNG,
	})

	a, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("Build should be byte-deterministic on unchanged input")
	}
}

func TestBuild_SkipsDotFiles(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html":      []byte("<html>"),
		".DS_Store":       []byte("metadata"),
		"assets/hero.png": fakePNG,
	})
	zipBytes, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.File {
		if strings.HasPrefix(filepath.Base(f.Name), ".") {
			t.Errorf("dotfile %q should not be in the bundle", f.Name)
		}
	}
	if len(r.File) != 2 {
		t.Errorf("expected 2 files in zip, got %d", len(r.File))
	}
}

func TestBuild_PreservesContent(t *testing.T) {
	dir := writeBundle(t, map[string][]byte{
		"index.html":      []byte(`<img src="assets/hero.png">`),
		"assets/hero.png": fakePNG,
	})
	zipBytes, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string][]byte{}
	for _, f := range r.File {
		rc, _ := f.Open()
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(rc)
		_ = rc.Close()
		contents[f.Name] = buf.Bytes()
	}
	if string(contents["index.html"]) != `<img src="assets/hero.png">` {
		t.Errorf("entry content not preserved: %q", contents["index.html"])
	}
	if !bytes.Equal(contents["assets/hero.png"], fakePNG) {
		t.Errorf("asset content not preserved")
	}
}

// --- Helpers -------------------------------------------------------------

func writeBundle(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func mustValidate(t *testing.T, dir string) *Report {
	t.Helper()
	r, err := Validate(dir)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return r
}

func hasError(r *Report, code string) bool {
	for _, i := range r.Issues {
		if i.Level == LevelError && i.Code == code {
			return true
		}
	}
	return false
}

func hasWarning(r *Report, code string) bool {
	for _, i := range r.Issues {
		if i.Level == LevelWarning && i.Code == code {
			return true
		}
	}
	return false
}
