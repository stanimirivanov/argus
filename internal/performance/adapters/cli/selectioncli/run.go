// Package selectioncli provides opt-in operator workload selection without writes.
package selectioncli

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
)

// Request carries reviewed input and a trusted clock evaluated after evidence I/O.
type Request struct {
	DeliveryID string
	Expected   perfeng.Expectations
	Policy     []byte
	Inspect    bool
	Now        func() time.Time
}

// Evaluate is the composition root's validated, bounded inspection/selection operation.
type Evaluate func(context.Context, Request) ([]byte, error)

// Run uses the trusted actual clock injected by the composition root. There is
// no historical-clock flag, provider request, persistence or execution side effect.
func Run(ctx context.Context, args []string, output io.Writer, evaluate Evaluate, now func() time.Time) error {
	flags := flag.NewFlagSet("select-performance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	delivery := flags.String("delivery-id", "", "persisted GitHub delivery ID")
	policy := flags.String("policy", "", "trusted reviewed mapping-policy file")
	inspect := flags.Bool("inspect", false, "inspect immutable source pins before policy review")
	var expected perfeng.Expectations
	flags.StringVar(&expected.Issuer, "issuer", "", "provider issuer")
	flags.StringVar(&expected.PrincipalIssuer, "principal-issuer", "", "principal issuer")
	flags.StringVar(&expected.PrincipalID, "principal-id", "", "principal ID")
	flags.StringVar(&expected.ProducerID, "producer-id", "perfeng-control-plane", "reviewed producer ID")
	flags.StringVar(&expected.ProducerVersion, "producer-version", "", "reviewed producer version")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 0 || output == nil || evaluate == nil || now == nil || (*inspect && *policy != "") || (!*inspect && *policy == "") {
		return commandline.UsageText("supply either -inspect or -policy and independent delivery/scope/version options")
	}
	for _, value := range []string{*delivery, expected.Issuer, expected.PrincipalIssuer, expected.PrincipalID, expected.ProducerID, expected.ProducerVersion} {
		if strings.TrimSpace(value) == "" {
			return commandline.UsageText("delivery, scope and producer version are required")
		}
	}
	var policyBytes []byte
	if err := ctx.Err(); err != nil {
		return err
	}
	if !*inspect {
		var err error
		policyBytes, err = readPolicy(*policy)
		if err != nil {
			return err
		}
	}
	encoded, err := evaluate(ctx, Request{DeliveryID: *delivery, Expected: expected, Policy: policyBytes, Inspect: *inspect, Now: now})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	data := append(encoded, '\n')
	n, err := output.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}

	return err
}

func readPolicy(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open reviewed performance policy")
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // Read error has precedence for a read-only file.
	data, err := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	if err != nil || len(data) > 16<<20 {
		return nil, errors.New("cannot read bounded reviewed performance policy")
	}

	return data, nil
}
