// SPDX-License-Identifier: GPL-3.0-or-later

package browser

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Address permits normal web URLs and explicitly named local files. Bare
// hostnames receive HTTPS; arbitrary Chromium/internal protocols are rejected.
func Address(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("enter a URL")
	}
	if strings.HasPrefix(s, "~/") {
		home, _ := os.UserHomeDir()
		s = filepath.Join(home, s[2:])
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		abs, err := filepath.Abs(s)
		if err != nil {
			return "", err
		}
		return (&url.URL{Scheme: "file", Path: abs}).String(), nil
	}
	if !strings.Contains(s, "://") && s != "about:blank" {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http", "https":
		if u.Host == "" {
			return "", fmt.Errorf("URL needs a hostname")
		}
	case "file":
		if u.Path == "" {
			return "", fmt.Errorf("file URL needs a path")
		}
	case "about":
		if s != "about:blank" {
			return "", fmt.Errorf("only about:blank is supported")
		}
	default:
		return "", fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	return u.String(), nil
}
