// Package catalogcli implements read-only, complete performance catalog inspection.
package catalogcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strings"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
)

// Fetch is the transport capability injected by the command composition root.
type Fetch func(context.Context, string, string, perfeng.Expectations) ([]json.RawMessage, error)

// Run fetches a complete validated snapshot and writes its page array only after
// success. token comes from ARGUS_PERFENG_TOKEN, never a command-line argument.
func Run(ctx context.Context, args []string, token string, output io.Writer, fetch Fetch) error {
	flags := flag.NewFlagSet("performance-catalog", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	endpoint := flags.String("endpoint", "", "reviewed full performance catalog endpoint")
	var expected perfeng.Expectations
	flags.StringVar(&expected.Issuer, "issuer", "", "expected provider issuer")
	flags.StringVar(&expected.PrincipalIssuer, "principal-issuer", "", "authenticated principal issuer")
	flags.StringVar(&expected.PrincipalID, "principal-id", "", "authenticated principal identity")
	flags.StringVar(&expected.ProducerID, "producer-id", "perfeng-control-plane", "expected producer identity")
	flags.StringVar(&expected.ProducerVersion, "producer-version", "", "reviewed producer version")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	for _, value := range []string{*endpoint, expected.Issuer, expected.PrincipalIssuer,
		expected.PrincipalID, expected.ProducerID, expected.ProducerVersion} {
		if strings.TrimSpace(value) == "" || flags.NArg() != 0 {
			return commandline.UsageText("performance-catalog requires endpoint and independent scope/version options")
		}
	}
	if token == "" {
		return errors.New("ARGUS_PERFENG_TOKEN is required")
	}
	pages, err := fetch(ctx, *endpoint, token, expected)
	if err != nil {
		return err
	}

	return json.NewEncoder(output).Encode(pages)
}
