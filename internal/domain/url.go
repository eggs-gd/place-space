package domain

import (
	"fmt"
	"net/url"
	"strings"
)

// CanonicalURL strips tracking query and fragment, lowercases the host, and
// forces https. The path is preserved as published, including its case.
func CanonicalURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty url")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	if parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("unsupported url %q", raw)
	}
	host := strings.ToLower(parsed.Hostname())
	if port := parsed.Port(); port != "" && port != "80" && port != "443" {
		host += ":" + port
	}
	parsed.Scheme = "https"
	parsed.Host = host
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), nil
}
