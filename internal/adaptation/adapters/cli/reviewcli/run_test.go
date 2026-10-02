package reviewcli

import "testing"

func TestReviewTokensPreferScopedCredentialsAndRetainLegacyFallback(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		values    map[string]string
		wantRead  string
		wantWrite string
	}{
		{name: "legacy", values: map[string]string{"ARGUS_GITHUB_TOKEN": "legacy"}, wantRead: "legacy", wantWrite: "legacy"},
		{name: "scoped", values: map[string]string{
			"ARGUS_GITHUB_TOKEN": "legacy", "ARGUS_GITHUB_READ_TOKEN": " read ", "ARGUS_GITHUB_WRITE_TOKEN": " write ",
		}, wantRead: "read", wantWrite: "write"},
		{name: "partial migration", values: map[string]string{
			"ARGUS_GITHUB_TOKEN": "legacy", "ARGUS_GITHUB_READ_TOKEN": "read",
		}, wantRead: "read", wantWrite: "legacy"},
		{name: "missing", values: map[string]string{}, wantRead: "", wantWrite: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			read, write := reviewTokens(func(name string) string { return test.values[name] })
			if read != test.wantRead || write != test.wantWrite {
				t.Fatalf("tokens = %q/%q, want %q/%q", read, write, test.wantRead, test.wantWrite)
			}
		})
	}
}
