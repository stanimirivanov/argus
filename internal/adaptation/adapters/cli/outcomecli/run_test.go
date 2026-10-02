package outcomecli

import "testing"

func TestObservationTokenUsesReadOnlyCredential(t *testing.T) {
	t.Parallel()
	values := map[string]string{
		"ARGUS_GITHUB_READ_TOKEN":  " read ",
		"ARGUS_GITHUB_WRITE_TOKEN": "write",
		"ARGUS_GITHUB_TOKEN":       "legacy",
	}
	getenv := func(name string) string { return values[name] }
	if got := observationToken(getenv); got != "read" {
		t.Fatalf("observation token = %q, want read token", got)
	}
	delete(values, "ARGUS_GITHUB_READ_TOKEN")
	if got := observationToken(getenv); got != "legacy" {
		t.Fatalf("legacy observation token = %q, want legacy token", got)
	}
}
