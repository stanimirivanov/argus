package requestcli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/requestcli"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestCreationPreservesInputsAndReportsReplayAndFailure(t *testing.T) {
	t.Parallel()
	intent := fixture(t)
	path := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(path, []byte(" {} \n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-delivery-id", "delivery", "-policy", path, "-review", path}
	for _, failure := range []error{nil, requestintent.ErrConflict, requestintent.ErrExpired, errors.New("private SQL details")} {
		var output bytes.Buffer
		create := func(_ context.Context, request requestcli.Creation) (requestintent.Intent, bool, error) {
			if request.DeliveryID != "delivery" || string(request.Policy) != " {} \n" || !bytes.Equal(request.Policy, request.Review) || !request.Now().Equal(intent.Data().Children[0].RequestedAt) {
				t.Fatal("changed inputs or clock")
			}
			return intent, false, failure
		}
		err := requestcli.RunCreate(t.Context(), args, &output, create, func() time.Time { return intent.Data().Children[0].RequestedAt })
		if failure != nil {
			if err == nil || output.Len() != 0 || err.Error() == "private SQL details" {
				t.Fatalf("unsafe failed creation: %v", err)
			}
			continue
		}
		var result struct {
			Created                   bool
			Fingerprint, ReviewSHA256 string
		}
		if err != nil || json.Unmarshal(output.Bytes(), &result) != nil || result.Created || result.Fingerprint != intent.Fingerprint() || result.ReviewSHA256 != intent.Data().ReviewSHA256 {
			t.Fatalf("missing replay result: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := requestcli.RunCreate(ctx, args, io.Discard, func(context.Context, requestcli.Creation) (requestintent.Intent, bool, error) {
		t.Fatal("write after cancellation")
		return intent, true, nil
	}, time.Now); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCreationRejectsReadModeAndUnboundedFiles(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"-pending"}, {"-authority", "self-approved"}, {"-issuer", "untrusted"}, {"-delivery-id", "d", "-policy", "missing"}, {"unexpected"}} {
		err := requestcli.RunCreate(t.Context(), args, io.Discard, func(context.Context, requestcli.Creation) (requestintent.Intent, bool, error) {
			t.Fatal("invalid input wrote")
			return requestintent.Intent{}, false, nil
		}, time.Now)
		if !errors.Is(err, commandline.ErrUsage) {
			t.Fatalf("usage: %v", err)
		}
	}
	path := filepath.Join(t.TempDir(), "large")
	if err := os.WriteFile(path, make([]byte, (16<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := requestcli.ReadCreationFile(path); err == nil {
		t.Fatal("accepted oversized file")
	}
}
