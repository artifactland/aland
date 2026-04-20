// Package project manages the .aland.json file that marks a directory as
// an artifactland project. One file tracks one or more artifacts, keyed
// by a short name under `artifacts`. Every source_file is validated
// against the project dir on load and save — relative paths only, no `..`
// escapes, no symlinks whose target lands outside — so a committed or
// forked .aland.json cannot redirect the CLI at files it shouldn't read.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Filename is the fixed project-file name.
const Filename = ".aland.json"

// CurrentVersion is the shape written by Save and required on load.
// Bumped only if the on-disk layout ever needs to change incompatibly.
const CurrentVersion = "1"

// DefaultArtifactName is the key used for single-artifact projects
// created by `aland init` or a first `aland push`. Users can rename it
// freely in the JSON.
const DefaultArtifactName = "main"

// ForkOf records where a fork came from.
type ForkOf struct {
	PostID   string `json:"post_id"`
	Username string `json:"user"`
	Slug     string `json:"slug"`
	Title    string `json:"title,omitempty"`
}

// Artifact is one artifact tracked by a project — typically one source file
// bound to one server-side post.
type Artifact struct {
	PostID      string   `json:"post_id,omitempty"`
	ForkOf      *ForkOf  `json:"fork_of,omitempty"`
	SourceFile  string   `json:"source_file"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Visibility  string   `json:"visibility,omitempty"`

	name string `json:"-"`
}

// Name returns the key this artifact has in the enclosing project.
func (a *Artifact) Name() string { return a.name }

// AbsSource resolves the artifact's source file against the given project dir.
// Callers should pass the project's Dir().
func (a *Artifact) AbsSource(projectDir string) string {
	return filepath.Join(projectDir, filepath.FromSlash(a.SourceFile))
}

// Project is the in-memory shape of .aland.json: a map of artifacts plus
// the directory it was loaded from (so Save can write back to the same place
// without a separate dir argument).
type Project struct {
	Version   string               `json:"version"`
	Artifacts map[string]*Artifact `json:"artifacts"`

	dir string `json:"-"`
}

// ErrNotAProject is returned by Load when the directory has no .aland.json.
var ErrNotAProject = errors.New("no .aland.json found")

// New returns an empty project rooted at dir. The project has no artifacts;
// callers populate it via Add and then call Save.
func New(dir string) *Project {
	return &Project{
		Version:   CurrentVersion,
		Artifacts: map[string]*Artifact{},
		dir:       dir,
	}
}

// Load reads and validates the project file in dir.
func Load(dir string) (*Project, error) {
	path := filepath.Join(dir, Filename)
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotAProject
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	p := &Project{dir: dir}
	if err := json.Unmarshal(raw, p); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if p.Version == "" {
		return nil, fmt.Errorf("%s is missing a `version` field", path)
	}
	if p.Version != CurrentVersion {
		return nil, fmt.Errorf("%s declares version %q; this CLI only understands %q (upgrade aland?)", path, p.Version, CurrentVersion)
	}
	if len(p.Artifacts) == 0 {
		return nil, fmt.Errorf("%s has no artifacts", path)
	}
	for name, a := range p.Artifacts {
		if a == nil {
			return nil, fmt.Errorf("%s: artifact %q is null", path, name)
		}
		a.name = name
		if err := validateArtifact(dir, name, a); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return p, nil
}

// Save writes the project file atomically (temp file + rename). Every
// artifact's source_file is re-validated before writing, so a program
// that hand-constructs a Project can't slip a bad path past Load's check.
func (p *Project) Save() error {
	if p.dir == "" {
		return errors.New("project has no directory; use project.New(dir) or Load(dir)")
	}
	if p.Artifacts == nil {
		p.Artifacts = map[string]*Artifact{}
	}
	for name, a := range p.Artifacts {
		if a == nil {
			return fmt.Errorf("artifact %q is nil", name)
		}
		if err := validateArtifact(p.dir, name, a); err != nil {
			return err
		}
	}

	p.Version = CurrentVersion
	path := filepath.Join(p.dir, Filename)
	tmp, err := os.CreateTemp(p.dir, ".aland-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	p.Version = CurrentVersion
	return nil
}

// Dir returns the directory the project lives in.
func (p *Project) Dir() string { return p.dir }

// Add inserts or replaces an artifact under name. source_file is validated
// against the project dir; an invalid path is rejected rather than written.
func (p *Project) Add(name string, a *Artifact) error {
	if name == "" {
		return errors.New("artifact name required")
	}
	if a == nil {
		return errors.New("artifact is nil")
	}
	if p.Artifacts == nil {
		p.Artifacts = map[string]*Artifact{}
	}
	if err := validateArtifact(p.dir, name, a); err != nil {
		return err
	}
	a.name = name
	p.Artifacts[name] = a
	return nil
}

// Get returns the artifact under name, or nil if absent.
func (p *Project) Get(name string) *Artifact {
	if p == nil || p.Artifacts == nil {
		return nil
	}
	return p.Artifacts[name]
}

// Only returns the sole artifact when there's exactly one, or an error whose
// message lists the alternatives when there are multiple. Used by commands
// that accept no explicit artifact selector.
func (p *Project) Only() (*Artifact, error) {
	switch len(p.Artifacts) {
	case 0:
		return nil, errors.New("project has no artifacts yet")
	case 1:
		for _, a := range p.Artifacts {
			return a, nil
		}
	}
	return nil, fmt.Errorf(
		"project has %d artifacts; pass a name or path:\n%s",
		len(p.Artifacts), p.listing(),
	)
}

// Resolve picks the artifact matching query — either by name (exact key in
// Artifacts) or by source_file path (any form the user might type: relative
// to cwd, relative to project dir, or absolute). On miss, the error lists
// candidates so the caller can bubble up a helpful message.
func (p *Project) Resolve(query string) (*Artifact, error) {
	if query == "" {
		return p.Only()
	}
	if a, ok := p.Artifacts[query]; ok {
		return a, nil
	}
	// Try path-based match. Accept any form the shell might hand us and
	// normalize to project-relative slash-form before comparing.
	normalized, err := normalizeSourcePath(p.dir, query)
	if err == nil {
		for _, a := range p.Artifacts {
			if filepath.ToSlash(filepath.Clean(a.SourceFile)) == normalized {
				return a, nil
			}
		}
	}
	return nil, fmt.Errorf("no artifact matches %q:\n%s", query, p.listing())
}

// Exists reports whether a .aland.json is present in dir without parsing it.
func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, Filename))
	return err == nil
}

// listing renders the artifact map as "  - name  (source_file)" lines for
// error messages. Sorted by name for a stable UI.
func (p *Project) listing() string {
	names := make([]string, 0, len(p.Artifacts))
	for n := range p.Artifacts {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		a := p.Artifacts[n]
		fmt.Fprintf(&b, "  - %s  (%s)\n", n, a.SourceFile)
	}
	return strings.TrimRight(b.String(), "\n")
}

// validateArtifact enforces the invariants we care about on every load and
// save: non-empty source_file, path confined to the project dir, no symlink
// escape. It does not require the file to exist — publish/preview report
// that separately with better context.
func validateArtifact(dir, name string, a *Artifact) error {
	if a.SourceFile == "" {
		return fmt.Errorf("artifact %q has no source_file", name)
	}
	if _, err := normalizeSourcePath(dir, a.SourceFile); err != nil {
		return fmt.Errorf("artifact %q: %w", name, err)
	}
	return nil
}

// normalizeSourcePath validates a source_file value against the project dir
// and returns its cleaned, slash-form, project-relative representation. It
// rejects absolute paths, `..` escapes, and symlinks whose target lands
// outside the project dir. The returned string is suitable for equality
// comparison in Resolve.
func normalizeSourcePath(dir, sourceFile string) (string, error) {
	if sourceFile == "" {
		return "", errors.New("source_file is required")
	}

	// Treat both / and \ as separators on input so a hand-edited JSON with
	// either works, but normalize to slash-form for storage and comparison.
	native := filepath.FromSlash(sourceFile)

	if filepath.IsAbs(native) {
		return "", fmt.Errorf("source_file %q must be relative to the project dir", sourceFile)
	}

	cleaned := filepath.Clean(native)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source_file %q escapes the project dir", sourceFile)
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving project dir: %w", err)
	}
	absSource := filepath.Join(absDir, cleaned)
	rel, err := filepath.Rel(absDir, absSource)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source_file %q escapes the project dir", sourceFile)
	}

	// If the path already exists and is a symlink, resolve it and re-check.
	// We walk manually rather than using EvalSymlinks because EvalSymlinks
	// requires every intermediate component to exist; here, only the bits
	// that do exist need to be safe.
	if info, err := os.Lstat(absSource); err == nil && info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(absSource)
		if err != nil {
			return "", fmt.Errorf("resolving symlink %q: %w", sourceFile, err)
		}
		resolvedAbs, err := filepath.Abs(resolved)
		if err != nil {
			return "", fmt.Errorf("resolving symlink %q: %w", sourceFile, err)
		}
		rel2, err := filepath.Rel(absDir, resolvedAbs)
		if err != nil || rel2 == ".." || strings.HasPrefix(rel2, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("source_file %q resolves (via symlink) outside the project dir", sourceFile)
		}
	}

	return filepath.ToSlash(cleaned), nil
}
