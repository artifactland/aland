package cli

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/artifactland/aland/internal/config"
)

func resolveAPIBase(flag string) (string, error) {
	raw := config.DefaultAPIBase
	if env := strings.TrimSpace(os.Getenv("ALAND_API")); env != "" {
		raw = env
	}
	if flag = strings.TrimSpace(flag); flag != "" {
		raw = flag
	}
	return validateAPIBase(raw)
}

func effectiveAPIBase(stored, flag string) (string, error) {
	if strings.TrimSpace(stored) != "" {
		return validateAPIBase(stored)
	}
	return resolveAPIBase(flag)
}

func validateAPIBase(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parsing API base URL %q: %w", raw, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("API base URL %q must include a scheme and host", raw)
	}

	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return "", fmt.Errorf("refusing insecure API base URL %q: plain HTTP is only allowed for localhost or loopback IPs", raw)
		}
	default:
		return "", fmt.Errorf("unsupported API base URL scheme %q", u.Scheme)
	}

	return strings.TrimRight(u.String(), "/"), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
