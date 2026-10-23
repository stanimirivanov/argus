package catalogcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
)

func TestInspectOutputsOnlyAfterSuccessfulFetch(t *testing.T) {
	args := []string{"-endpoint", "https://provider.example/v1/performance-catalog", "-issuer", "https://provider.example/identity", "-principal-issuer", "https://auth.example/tenant", "-principal-id", "argus-service", "-producer-version", "1.0.0"}
	for _, failed := range []bool{false, true} {
		var output bytes.Buffer
		fetch := func(_ context.Context, endpoint, token string, expected perfeng.Expectations) ([]json.RawMessage, error) {
			if endpoint != args[1] || token != "test-token" || expected.PrincipalID != "argus-service" || expected.ProducerID != "perfeng-control-plane" {
				t.Error("lost independent configuration")
			}
			if failed {
				return []json.RawMessage{[]byte(`{}`)}, errors.New("fetch failed")
			}

			return []json.RawMessage{[]byte(`{"complete":true}`)}, nil
		}
		err := Run(t.Context(), args, "test-token", &output, fetch)
		if failed {
			if err == nil || output.Len() != 0 {
				t.Fatal("exposed failed snapshot")
			}
			continue
		}
		if err != nil || output.String() != "[{\"complete\":true}]\n" {
			t.Fatalf("inspection = %s, %v", output.String(), err)
		}
	}
}

func TestRejectsInvalidInvocationAndMissingCredential(t *testing.T) {
	args := []string{"-endpoint", "https://provider.example/v1/performance-catalog", "-issuer", "https://provider.example/identity", "-principal-issuer", "https://auth.example/tenant", "-principal-id", "argus-service", "-producer-version", "1.0.0"}
	called := false
	fetch := func(context.Context, string, string, perfeng.Expectations) ([]json.RawMessage, error) {
		called = true
		return nil, nil
	}
	for _, invalid := range [][]string{nil, {"--unknown"}, append(append([]string{}, args...), "extra"), args[:len(args)-2]} {
		if err := Run(t.Context(), invalid, "test-token", &bytes.Buffer{}, fetch); !errors.Is(err, commandline.ErrUsage) {
			t.Fatalf("usage = %v", err)
		}
	}
	if err := Run(t.Context(), args, "", &bytes.Buffer{}, fetch); err == nil {
		t.Fatal("accepted missing credential")
	}
	if called {
		t.Fatal("fetched before validating usage and credentials")
	}
}
