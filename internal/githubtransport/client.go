// Package githubtransport applies the credential-bearing HTTP policy shared
// by Argus's GitHub adapters. It owns transport configuration, not API calls.
package githubtransport

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var errRedirect = errors.New("GitHub API redirect rejected")

// Configure validates one API origin and clones the supplied client. A cloned
// client prevents later caller changes to CheckRedirect from weakening the
// credential boundary. No redirect is followed, including same-origin ones:
// callers must deliberately configure the final canonical API endpoint.
func Configure(rawURL string, supplied *http.Client, timeout time.Duration) (*url.URL, *http.Client, error) {
	baseURL, err := url.Parse(rawURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" || baseURL.Opaque != "" ||
		baseURL.User != nil || strings.ContainsAny(rawURL, "?#") ||
		baseURL.Scheme != "https" && !(baseURL.Scheme == "http" &&
			(baseURL.Hostname() == "localhost" || baseURL.Hostname() == "127.0.0.1")) {
		return nil, nil, errors.New("invalid GitHub API URL")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/"
	baseURL.RawPath = ""
	client := &http.Client{Timeout: timeout}
	if supplied != nil {
		clone := *supplied
		if clone.Timeout <= 0 || clone.Timeout > timeout {
			clone.Timeout = timeout
		}
		client = &clone
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errRedirect }

	return baseURL, client, nil
}

// ResolvePath accepts only an API-base-relative path. Provider data can never
// replace the configured origin or escape an Enterprise API path prefix.
func ResolvePath(base *url.URL, relative string) (*url.URL, error) {
	if base == nil || relative == "" || strings.HasPrefix(relative, "/") ||
		strings.ContainsAny(relative, "\\#") {
		return nil, errors.New("invalid GitHub API path")
	}
	reference, err := url.Parse(relative)
	if err != nil || reference.IsAbs() || reference.Host != "" || reference.User != nil || reference.Fragment != "" {
		return nil, errors.New("invalid GitHub API path")
	}
	escapedPath := strings.ToLower(reference.EscapedPath())
	if strings.Contains(reference.Path, "\\") || strings.Contains(escapedPath, "%2f") ||
		strings.Contains(escapedPath, "%5c") || strings.Contains(escapedPath, "%2e") {
		return nil, errors.New("invalid GitHub API path")
	}
	endpoint := base.ResolveReference(reference)
	if !strings.HasPrefix(endpoint.Path, base.Path) {
		return nil, errors.New("GitHub API path escapes configured base")
	}

	return endpoint, nil
}
