package evidencecli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func approvalFixture(t *testing.T) (Approval, []byte, Query) {
	t.Helper()
	scope := requestintent.Scope{Issuer: "https://provider.example", PrincipalIssuer: "https://auth.example", PrincipalID: "argus"}
	approval := Approval{Scope: scope, Position: 1, IntentFingerprint: strings.Repeat("a", 64), RequestSHA256: strings.Repeat("b", 64), ExpectedReviewer: "trusted-owner"}
	raw, err := json.Marshal(approval)
	if err != nil {
		t.Fatal(err)
	}

	return approval, raw, Query{Scope: scope, Position: 1}
}

func TestAuthorizeRequiresExactProtectedPolicy(t *testing.T) {
	_, raw, query := approvalFixture(t)
	if _, err := Authorize(raw, query); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Query){func(q *Query) { q.Scope.PrincipalID = "other" }, func(q *Query) { q.Scope.PrincipalIssuer += "/other" }, func(q *Query) { q.Scope.Issuer += "/other" }, func(q *Query) { q.Position++ }} {
		wrong := query
		mutate(&wrong)
		if _, err := Authorize(raw, wrong); !errors.Is(err, ErrDenied) {
			t.Fatal("scope granted", err)
		}
	}
	for _, input := range [][]byte{[]byte(`{}`), bytes.Replace(raw, []byte(`"position":1`), []byte(`"position":1,"position":1`), 1), bytes.Replace(raw, []byte(`"position":1`), []byte(`"position":1,"token":"secret"`), 1), bytes.Replace(raw, []byte(strings.Repeat("a", 64)), []byte("main"), 1), bytes.Repeat([]byte("x"), 65537)} {
		if _, err := Authorize(input, query); !errors.Is(err, ErrDenied) {
			t.Fatal("malformed authority accepted", err)
		}
	}
}

type shortOutput struct{}

func (shortOutput) Write(raw []byte) (int, error) { return len(raw) - 1, nil }

func TestRunDiscardsFailedExportAndDetectsPartialWrites(t *testing.T) {
	_, _, query := approvalFixture(t)
	args := []string{"-issuer", query.Scope.Issuer, "-principal-issuer", query.Scope.PrincipalIssuer, "-principal-id", query.Scope.PrincipalID, "-position", "1"}
	var output bytes.Buffer
	export := func(_ context.Context, got Query) ([]byte, error) {
		if got != query {
			t.Fatal("changed query")
		}
		return []byte("private"), errors.New("driver secret")
	}
	if err := Run(t.Context(), args, &output, export); err == nil || strings.Contains(err.Error(), "secret") || output.Len() != 0 {
		t.Fatal("failed export disclosed bytes", err)
	}
	// Serialization/conformance is tested at the pure projection boundary. This
	// injected operation proves argument and single-write behavior independently.
	export = func(context.Context, Query) ([]byte, error) { return []byte("{}\n"), nil }
	if err := Run(t.Context(), args, shortOutput{}, export); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
	if err := Run(t.Context(), args, &output, export); err != nil || output.String() != "{}\n" {
		t.Fatal(err)
	}
	called := false
	export = func(context.Context, Query) ([]byte, error) { called = true; return nil, nil }
	if err := Run(t.Context(), []string{"-position", "1"}, &output, export); err == nil || called {
		t.Fatal("invalid arguments opened export")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Run(ctx, args, &output, export); !errors.Is(err, context.Canceled) || called {
		t.Fatal("cancelled export")
	}
}
