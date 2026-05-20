// Package bundle handles the multi-file artifact bundles that ship to
// artifact.land's Cloudflare-Worker-fronted content origin. The bundle
// shape is the same one the server enforces in Rails BundleUnpacker:
// entry file (index.html or index.jsx) at the bundle root, every other
// file under assets/, image-only allowlist for v1.
//
// Two responsibilities:
//   - Validate(dir) — offline lint of a bundle directory; produces a
//     Report of blocking errors and warnings before any network round
//     trip. Used by `aland validate` and called automatically by
//     `aland push <directory>` before upload.
//   - Build(dir) — produces a deterministic zip of the directory:
//     sorted entries, zero timestamps. The server's
//     BundleUnpacker re-validates; if the local validate was clean,
//     the upload is too.
package bundle

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// IssueLevel separates blocking issues from advisories.
type IssueLevel int

const (
	LevelError   IssueLevel = iota // refuses publish
	LevelWarning                   // prints but doesn't block
)

// Issue is a single thing the linter wants to tell the user. Code is
// machine-readable; Message is human-readable; Path points at the
// offending file (relative to the bundle root) when applicable.
type Issue struct {
	Level   IssueLevel
	Code    string
	Path    string
	Message string
}

// Report is what Validate returns. EntryPath is only set when an entry
// was found; FileCount and TotalBytes always reflect what the walk saw.
type Report struct {
	Issues     []Issue
	EntryPath  string
	FileCount  int
	TotalBytes int64
}

// HasErrors returns true if any issue is blocking.
func (r *Report) HasErrors() bool {
	for _, i := range r.Issues {
		if i.Level == LevelError {
			return true
		}
	}
	return false
}

// Errors filters down to blocking issues.
func (r *Report) Errors() []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.Level == LevelError {
			out = append(out, i)
		}
	}
	return out
}

// Warnings filters down to non-blocking issues.
func (r *Report) Warnings() []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.Level == LevelWarning {
			out = append(out, i)
		}
	}
	return out
}

// Caps mirrors the server-side per-tier caps. The CLI doesn't know the
// user's tier offline, so validation reports against both — anything
// over the free cap gets a warning, anything over the Pro cap blocks.
type Caps struct {
	Tier     string
	MaxBytes int64
	MaxFiles int
}

var (
	FreeCaps = Caps{Tier: "free", MaxBytes: 5 * 1024 * 1024, MaxFiles: 50}
	ProCaps  = Caps{Tier: "pro", MaxBytes: 25 * 1024 * 1024, MaxFiles: 200}
)

// ImageWeightWarnBytes is the threshold above which the linter
// suggests compressing an image. Generous enough that small hero
// photos don't trip it, tight enough that "I dropped a 5 MB raw
// photograph" gets flagged.
const ImageWeightWarnBytes = 500 * 1024

// EntryNames lists the only filenames a bundle's entry file can have.
var EntryNames = []string{"index.html", "index.jsx"}

// AssetExtensions is the v1 allowlist for asset file types. Mirrors
// the server-side BundleUnpacker::ASSET_EXTENSIONS.
var AssetExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".avif": true,
}

// Validate inspects a directory and returns a Report. Errors block
// publish; warnings print without blocking. Never returns an error
// for "the bundle is bad" — that lives in the Report.Issues. The
// returned `error` only fires for unrecoverable I/O problems.
func Validate(dir string) (*Report, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dir)
	}

	report := &Report{}
	var entryPath string
	var totalBytes int64
	var fileCount int

	err = filepath.Walk(abs, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		// Skip dotfiles silently — .DS_Store, .git, editor swap files.
		// If the user really wants to ship a dotfile asset, they'll
		// notice that it's not in the bundle.
		if isDotFile(rel) {
			return nil
		}

		fileCount++
		totalBytes += info.Size()

		if !strings.Contains(rel, "/") {
			// Root-level file: must be an entry.
			if !isEntryName(rel) {
				report.Issues = append(report.Issues, Issue{
					Level:   LevelError,
					Code:    "bad_structure",
					Path:    rel,
					Message: fmt.Sprintf("Root-level files must be named index.html or index.jsx — %q isn't. Move it under assets/ or rename.", rel),
				})
				return nil
			}
			if entryPath != "" {
				report.Issues = append(report.Issues, Issue{
					Level:   LevelError,
					Code:    "multiple_entries",
					Path:    rel,
					Message: fmt.Sprintf("Bundle has more than one entry file (%s and %s). Keep exactly one.", entryPath, rel),
				})
				return nil
			}
			entryPath = rel
			return nil
		}

		// Nested file: must live under assets/.
		if !strings.HasPrefix(rel, "assets/") {
			report.Issues = append(report.Issues, Issue{
				Level:   LevelError,
				Code:    "bad_structure",
				Path:    rel,
				Message: fmt.Sprintf("Non-root files must live under assets/ — found %q.", rel),
			})
			return nil
		}

		ext := strings.ToLower(filepath.Ext(rel))
		if !AssetExtensions[ext] {
			report.Issues = append(report.Issues, Issue{
				Level:   LevelError,
				Code:    "disallowed_type",
				Path:    rel,
				Message: fmt.Sprintf("File type not allowed in bundle: %s. v1 allows raster images only (.png, .jpg, .gif, .webp, .avif).", rel),
			})
			return nil
		}

		if info.Size() > ImageWeightWarnBytes {
			report.Issues = append(report.Issues, Issue{
				Level:   LevelWarning,
				Code:    "image_oversized",
				Path:    rel,
				Message: fmt.Sprintf("%s is %s — consider compressing to under 500 KB (squoosh.app, ImageOptim, or `cwebp --q 80 in.png -o out.webp`).", rel, humanizeBytes(info.Size())),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if entryPath == "" {
		report.Issues = append(report.Issues, Issue{
			Level:   LevelError,
			Code:    "no_entry",
			Message: "Bundle has no entry file. Add index.html or index.jsx at the bundle root.",
		})
	}

	// Cap reporting. Pro cap is a hard ceiling; over-free is a warning
	// because the CLI doesn't know the user's tier offline.
	if totalBytes > ProCaps.MaxBytes {
		report.Issues = append(report.Issues, Issue{
			Level:   LevelError,
			Code:    "too_large",
			Message: fmt.Sprintf("Bundle is %s — over the %s Pro cap. Trim assets.", humanizeBytes(totalBytes), humanizeBytes(ProCaps.MaxBytes)),
		})
	} else if totalBytes > FreeCaps.MaxBytes {
		report.Issues = append(report.Issues, Issue{
			Level:   LevelWarning,
			Code:    "over_free_cap",
			Message: fmt.Sprintf("Bundle is %s — over the %s free cap. Pro allows up to %s.", humanizeBytes(totalBytes), humanizeBytes(FreeCaps.MaxBytes), humanizeBytes(ProCaps.MaxBytes)),
		})
	}

	if fileCount > ProCaps.MaxFiles {
		report.Issues = append(report.Issues, Issue{
			Level:   LevelError,
			Code:    "too_many_files",
			Message: fmt.Sprintf("Bundle has %d files — over the %d Pro cap.", fileCount, ProCaps.MaxFiles),
		})
	} else if fileCount > FreeCaps.MaxFiles {
		report.Issues = append(report.Issues, Issue{
			Level:   LevelWarning,
			Code:    "over_free_file_count",
			Message: fmt.Sprintf("Bundle has %d files — over the %d free cap. Pro allows %d.", fileCount, FreeCaps.MaxFiles, ProCaps.MaxFiles),
		})
	}

	// Reference integrity: parse the entry HTML for relative src/href
	// references and confirm each points at a real file in the bundle.
	if entryPath != "" {
		refs := checkReferences(abs, entryPath)
		report.Issues = append(report.Issues, refs...)
	}

	report.EntryPath = entryPath
	report.FileCount = fileCount
	report.TotalBytes = totalBytes
	return report, nil
}

// Build zips the directory into a deterministic byte slice. Entries
// sorted by path; timestamps zeroed. Re-running on the same content
// produces identical bytes (so the SHA can serve as a cache key, and
// re-pushing an unchanged bundle doesn't bump content_version
// upstream for no reason).
func Build(dir string) ([]byte, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	var paths []string
	err = filepath.Walk(abs, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if isDotFile(rel) {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(paths)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, rel := range paths {
		if err := addZipEntry(zw, abs, rel); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func addZipEntry(zw *zip.Writer, root, rel string) error {
	full := filepath.Join(root, rel)
	f, err := os.Open(full)
	if err != nil {
		return err
	}
	defer f.Close()

	header := &zip.FileHeader{
		Name:     rel,
		Method:   zip.Deflate,
		Modified: time.Unix(0, 0).UTC(),
	}
	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

func isEntryName(rel string) bool {
	for _, n := range EntryNames {
		if rel == n {
			return true
		}
	}
	return false
}

func isDotFile(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// HumanizeBytes is exported so the CLI command can format cap values
// the same way reports do.
func HumanizeBytes(b int64) string { return humanizeBytes(b) }

func humanizeBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}

// --- Reference integrity ----------------------------------------------

// Regex-based extraction. Not a full HTML/CSS parser — picks up the
// common cases (src/href on tags, url() in inline styles). False
// negatives are fine; false positives (warning on something that
// works) are the bad direction, so the matchers stay conservative.
var referenceRe = regexp.MustCompile(
	`(?i)(?:src|href)\s*=\s*["']([^"']+)["']|url\(\s*["']?([^"')\s]+)`,
)

func checkReferences(root, entryRel string) []Issue {
	full := filepath.Join(root, entryRel)
	content, err := os.ReadFile(full)
	if err != nil {
		return nil
	}

	var issues []Issue
	seen := map[string]bool{}

	for _, m := range referenceRe.FindAllStringSubmatch(string(content), -1) {
		ref := m[1]
		if ref == "" {
			ref = m[2]
		}
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true

		if isAnchorOrSpecial(ref) {
			continue
		}
		if isExternalURL(ref) {
			issues = append(issues, Issue{
				Level:   LevelWarning,
				Code:    "external_reference",
				Path:    ref,
				Message: fmt.Sprintf("Entry references external URL %q — blocked at runtime by `connect-src 'none'` and `img-src 'self' data: blob:`. Bundle the asset instead.", ref),
			})
			continue
		}

		// Strip query/fragment so `assets/photo.png?v=2` resolves.
		path := stripQueryFragment(ref)
		target := filepath.Join(root, path)
		if _, err := os.Stat(target); err != nil {
			issues = append(issues, Issue{
				Level:   LevelError,
				Code:    "broken_reference",
				Path:    ref,
				Message: fmt.Sprintf("Entry references %q, but that file isn't in the bundle.", ref),
			})
		}
	}
	return issues
}

func stripQueryFragment(ref string) string {
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		return ref[:i]
	}
	return ref
}

func isExternalURL(ref string) bool {
	return strings.HasPrefix(ref, "http://") ||
		strings.HasPrefix(ref, "https://") ||
		strings.HasPrefix(ref, "//")
}

// Anchors, absolute paths from the artifact's own root (which we treat
// as outside our scope), and protocol-style refs (data:, blob:, etc.)
// are not bundle-relative — skip the existence check.
func isAnchorOrSpecial(ref string) bool {
	if ref == "" {
		return true
	}
	switch ref[0] {
	case '#', '/':
		return true
	}
	for _, p := range []string{"data:", "blob:", "mailto:", "tel:", "javascript:"} {
		if strings.HasPrefix(ref, p) {
			return true
		}
	}
	return false
}
