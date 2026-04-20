package oauth

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenURL attempts to open the given URL in the user's default browser.
// Falls back to returning an error so the caller can print the URL for the
// user to open manually (happens in SSH sessions and some headless setups).
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// /c lets cmd exit after start returns. "" is the empty window
		// title start expects as its first positional arg; without it a URL
		// containing spaces would be misparsed.
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening browser: %w", err)
	}
	// Don't wait — let the browser process run detached.
	go func() { _ = cmd.Wait() }()
	return nil
}
