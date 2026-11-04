package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/evidencecli"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
	"github.com/stanimirivanov/argus/internal/postgres"
)

func TestAuthorizationPrecedesDatabaseAcquisition(t *testing.T) {
	scope := requestintent.Scope{Issuer: "https://provider.example", PrincipalIssuer: "https://auth.example", PrincipalID: "argus"}
	approval := evidencecli.Approval{Scope: scope, Position: 1, IntentFingerprint: strings.Repeat("a", 64), RequestSHA256: strings.Repeat("b", 64), ExpectedReviewer: "owner"}
	raw, err := json.Marshal(approval)
	if err != nil {
		t.Fatal(err)
	}
	query := evidencecli.Query{Scope: scope, Position: 2}
	open := func(context.Context, string) (*postgres.Runtime, error) {
		t.Fatal("database opened before authorization")
		return nil, nil
	}
	if result, err := authorizedExport(t.Context(), query, raw, open); err == nil || result != nil {
		t.Fatal("unauthorized output")
	}
}

func TestProtectedApprovalRejectsLinksPermissionsAndNonregularInputs(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "approval.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if raw, err := protectedApproval(path); err != nil || string(raw) != "{}" {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := protectedApproval(link); err == nil {
		t.Fatal("followed symlink")
	}
	hard := filepath.Join(root, "hard")
	if err := os.Link(path, hard); err != nil {
		t.Fatal(err)
	}
	if _, err := protectedApproval(path); err == nil {
		t.Fatal("hard link approved")
	}
	if err := os.Remove(hard); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := protectedApproval(path); err == nil {
		t.Fatal("public policy approved")
	}
	if _, err := protectedApproval(root); err == nil {
		t.Fatal("directory approved")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := protectedApproval(path); err == nil {
		t.Fatal("untrusted ancestor approved")
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
}
