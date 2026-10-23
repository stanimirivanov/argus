// Package catalogcli implements complete catalog inspection and opt-in publication.
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
	"github.com/stanimirivanov/argus/internal/performance/catalogsync"
)

// Fetch is the transport capability injected by the command composition root.
type Fetch func(context.Context, string, string, perfeng.Expectations) ([]json.RawMessage, error)

// Publication is the opt-in CLI result, distinct from source page contracts.
type Publication struct {
	Created bool              `json:"created"`
	Pages   []json.RawMessage `json:"pages"`
}

// Storage supplies scoped publication/read capabilities from the composition root.
type Storage struct {
	Publish func(context.Context, []json.RawMessage, perfeng.Expectations) (Publication, error)
	Current func(context.Context, catalogsync.Scope) ([]json.RawMessage, error)
}

// Run fetches a complete validated snapshot and writes its page array only after
// success. token comes from ARGUS_PERFENG_TOKEN, never a command-line argument.
func Run(ctx context.Context, args []string, token string, output io.Writer, fetch Fetch) error {
	return RunWithStorage(ctx, args, token, output, fetch, Storage{})
}

// RunWithStorage adds opt-in publication and stored-current inspection. Provider
// calls finish before publication starts; failed operations produce no JSON output.
func RunWithStorage(ctx context.Context, args []string, token string, output io.Writer, fetch Fetch, storage Storage) error {
	flags := flag.NewFlagSet("performance-catalog", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	endpoint := flags.String("endpoint", "", "reviewed full performance catalog endpoint")
	publish := flags.Bool("publish", false, "atomically publish the fetched view")
	stored := flags.Bool("stored", false, "inspect the stored current view without contacting Perfeng")
	var expected perfeng.Expectations
	flags.StringVar(&expected.Issuer, "issuer", "", "expected provider issuer")
	flags.StringVar(&expected.PrincipalIssuer, "principal-issuer", "", "authenticated principal issuer")
	flags.StringVar(&expected.PrincipalID, "principal-id", "", "authenticated principal identity")
	flags.StringVar(&expected.ProducerID, "producer-id", "perfeng-control-plane", "expected producer identity")
	flags.StringVar(&expected.ProducerVersion, "producer-version", "", "reviewed producer version")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *publish && *stored {
		return commandline.UsageText("publish and stored modes are mutually exclusive")
	}
	required := []string{expected.Issuer, expected.PrincipalIssuer, expected.PrincipalID}
	if !*stored {
		required = append(required, *endpoint, expected.ProducerID, expected.ProducerVersion)
	}
	for _, value := range required {
		if strings.TrimSpace(value) == "" || flags.NArg() != 0 {
			return commandline.UsageText("performance-catalog requires endpoint and independent scope/version options")
		}
	}
	if *stored {
		if storage.Current == nil {
			return errors.New("catalog storage is unavailable")
		}
		pages, err := storage.Current(ctx, catalogsync.Scope{Issuer: expected.Issuer, PrincipalIssuer: expected.PrincipalIssuer, PrincipalID: expected.PrincipalID})
		if err != nil {
			return err
		}

		return json.NewEncoder(output).Encode(pages)
	}
	if token == "" {
		return errors.New("ARGUS_PERFENG_TOKEN is required")
	}
	pages, err := fetch(ctx, *endpoint, token, expected)
	if err != nil {
		return err
	}
	if *publish {
		if storage.Publish == nil {
			return errors.New("catalog storage is unavailable")
		}
		result, err := storage.Publish(ctx, pages, expected)
		if err != nil {
			return err
		}

		return json.NewEncoder(output).Encode(result)
	}

	return json.NewEncoder(output).Encode(pages)
}
