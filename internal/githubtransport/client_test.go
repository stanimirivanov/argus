package githubtransport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfigureRejectsAmbiguousAPIURLs(t *testing.T) {
	t.Parallel()
	for _, candidate := range []string{
		"https://user:password@api.github.com", "https://api.github.com?token=abc",
		"https://api.github.com#fragment", "http://api.github.com", "https://api.github.com.attacker.test?x=1",
	} {
		if _, _, err := Configure(candidate, nil, time.Second); err == nil {
			t.Errorf("Configure(%q) accepted an ambiguous URL", candidate)
		}
	}
}

func TestConfigureCopiesClientAndCapsTimeout(t *testing.T) {
	t.Parallel()
	supplied := &http.Client{Timeout: time.Hour}
	_, configured, err := Configure("https://api.github.com", supplied, 20*time.Second)
	if err != nil || configured == supplied || configured.Timeout != 20*time.Second ||
		supplied.Timeout != time.Hour || supplied.CheckRedirect != nil {
		t.Fatalf("configured client = %+v, supplied = %+v, error = %v", configured, supplied, err)
	}
}

func TestResolvePathKeepsEnterpriseAPIBase(t *testing.T) {
	t.Parallel()
	base, _, err := Configure("https://github.example/api/v3/", nil, time.Second)
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	endpoint, err := ResolvePath(base, "repos/example/tests?per_page=2")
	if err != nil || endpoint.String() != "https://github.example/api/v3/repos/example/tests?per_page=2" {
		t.Fatalf("ResolvePath() = %v, %v", endpoint, err)
	}
	for _, candidate := range []string{
		"https://attacker.example/steal", "//attacker.example/steal", "/repos/example/tests",
		"../../steal", "..%2f..%2fsteal", "..\\steal", "repos/test#fragment", "repos/test#",
	} {
		if _, err := ResolvePath(base, candidate); err == nil {
			t.Errorf("ResolvePath(%q) accepted an unsafe path", candidate)
		}
	}
}

func TestCredentialBearingClientNeverFollowsRedirect(t *testing.T) {
	t.Parallel()
	var reached atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		reached.Add(1)
		if request.Header.Get("Authorization") != "" {
			t.Error("credential reached redirected target")
		}
		response.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	for _, redirect := range []string{target.URL, "https://subdomain.github.com", "http://api.github.com"} {
		redirect := redirect
		t.Run(redirect, func(t *testing.T) {
			assertRedirectRejected(t, redirect)
		})
	}
	if reached.Load() != 0 {
		t.Fatalf("redirect target received %d requests", reached.Load())
	}
}

func assertRedirectRejected(t *testing.T, redirect string) {
	t.Helper()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, redirect, http.StatusFound)
	}))
	defer origin.Close()
	base, client, err := Configure(origin.URL, origin.Client(), time.Second)
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base.String()+"repos/test", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer secret")
	response, err := client.Do(request)
	if response != nil {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Errorf("close redirect response: %v", closeErr)
		}
	}
	if err == nil || !strings.Contains(err.Error(), errRedirect.Error()) {
		t.Fatalf("redirect result = %v, want rejection", err)
	}
}
