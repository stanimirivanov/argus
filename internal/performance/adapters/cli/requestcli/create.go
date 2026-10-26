package requestcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// Creation carries submitted evidence, never approval. Now is the actual clock
// supplied by the root, evaluated after stored evidence reads.
type Creation struct {
	DeliveryID     string
	Policy, Review []byte
	Now            func() time.Time
}

// Create compiles independently authorized evidence and atomically retains it.
// The bool distinguishes new retention from replay. An error may leave commit
// outcome uncertain; retain identities and recover with scoped inspection.
type Create func(context.Context, Creation) (requestintent.Intent, bool, error)

// ReadCreationFile bounds local operator/configuration input without exposing
// paths/content in diagnostics. Origin and access control remain host duties.
func ReadCreationFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open performance request input")
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // Read failure has precedence for read-only input.
	raw, err := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	if err != nil || len(raw) > 16<<20 {
		return nil, errors.New("cannot read bounded performance request input")
	}

	return raw, nil
}

// RunCreate handles options after leading -create; no argument supplies authority.
// Output failure/cancellation after success cannot roll back committed work.
func RunCreate(ctx context.Context, args []string, output io.Writer, create Create, now func() time.Time) error {
	if output == nil || create == nil || now == nil {
		return commandline.UsageText("invalid creation configuration")
	}
	request, err := creationInput(ctx, args)
	if err != nil {
		return err
	}
	request.Now = now
	intent, created, err := create(ctx, request)
	if err != nil {
		return creationError(ctx, err)
	}
	if intent.Fingerprint() == "" {
		return requestintent.ErrUnavailable
	}
	encoded, err := json.Marshal(struct {
		Created      bool   `json:"created"`
		Fingerprint  string `json:"fingerprint"`
		ReviewSHA256 string `json:"reviewSha256"`
	}{created, intent.Fingerprint(), intent.Data().ReviewSHA256})
	if err != nil {
		return requestintent.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	n, err := output.Write(encoded)
	if err == nil && n != len(encoded) {
		return io.ErrShortWrite
	}

	return err
}

func creationInput(ctx context.Context, args []string) (Creation, error) {
	flags := flag.NewFlagSet("performance-requests -create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var request Creation
	policy := flags.String("policy", "", "original reviewed policy file")
	review := flags.String("review", "", "canonical reviewed request file")
	flags.StringVar(&request.DeliveryID, "delivery-id", "", "exact stored GitHub delivery ID")
	if err := commandline.Parse(flags, args); err != nil {
		return Creation{}, err
	}
	if flags.NArg() != 0 || strings.TrimSpace(request.DeliveryID) == "" || *policy == "" || *review == "" {
		return Creation{}, commandline.UsageText("creation requires -delivery-id, -policy and -review")
	}
	if err := ctx.Err(); err != nil {
		return Creation{}, err
	}
	var err error
	request.Policy, err = ReadCreationFile(*policy)
	if err != nil {
		return Creation{}, err
	}
	request.Review, err = ReadCreationFile(*review)
	if err != nil {
		return Creation{}, err
	}
	if err := ctx.Err(); err != nil {
		return Creation{}, err
	}

	return request, nil
}

func creationError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, performance.ErrInvalid, requestintent.ErrInvalid, requestintent.ErrConflict, requestintent.ErrExpired} {
		if errors.Is(err, known) {
			return known
		}
	}

	return requestintent.ErrUnavailable
}
