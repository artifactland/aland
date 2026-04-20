// Package ui renders output for humans and agents. Styling is disabled
// automatically when stdout isn't a TTY (pipes, agents, CI) so the same
// commands produce clean machine-readable text without flags.
package ui

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
)

var (
	// Pre-built styles. Lip Gloss renders them as plain strings when
	// ColorProfile is NoTTY, which we force below for piped output.
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	ErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	WarnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	MutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	BoldStyle    = lipgloss.NewStyle().Bold(true)
	AccentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
)

// Init decides once whether the current stdout is a real terminal. If not —
// because the user piped us into jq, an agent is capturing our output, or the
// NO_COLOR env is set — we drop every ANSI escape. Call once from main().
func Init() {
	if !isTerminal(os.Stdout) || os.Getenv("NO_COLOR") != "" {
		lipgloss.SetColorProfile(0) // Ascii — no color codes emitted.
	}
}

// isTerminal reports whether f is attached to a TTY. Used to gate spinners
// and colored output in commands.
func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// IsStdoutTerminal is exposed for commands that need to branch behavior —
// e.g., pretty-print a table interactively but emit JSON when piped.
func IsStdoutTerminal() bool {
	return isTerminal(os.Stdout)
}

// Success prints a "✓ msg" line styled in green. Writes to stderr so stdout
// stays reserved for data (Unix convention: data out, status err).
func Success(format string, args ...any) {
	fmt.Fprintln(os.Stderr, SuccessStyle.Render("✓")+" "+fmt.Sprintf(format, args...))
}

// Info prints a muted status line. Goes to stderr for the same reason.
func Info(format string, args ...any) {
	fmt.Fprintln(os.Stderr, MutedStyle.Render("·")+" "+fmt.Sprintf(format, args...))
}

// Warn prints a warning with a yellow bullet.
func Warn(format string, args ...any) {
	fmt.Fprintln(os.Stderr, WarnStyle.Render("!")+" "+fmt.Sprintf(format, args...))
}

// Errorf prints an error heading and returns an error with the same message
// so callers can `return ui.Errorf(...)` and let cobra propagate the exit code.
func Errorf(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, ErrorStyle.Render("✗ "+msg))
	return fmt.Errorf("%s", msg)
}

// Writef writes plain text to stdout (data channel). Use this for anything
// the caller might want to pipe into a file or another process.
func Writef(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, format, args...)
}
