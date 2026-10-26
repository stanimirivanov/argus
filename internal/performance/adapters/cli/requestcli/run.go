// Package requestcli provides reviewed creation and scoped performance inspection.
package requestcli

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strings"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// Reader contains only scoped immutable reads, never creation or dispatch authority.
type Reader struct {
	Get     func(context.Context, requestintent.Scope, string) (requestintent.Intent, error)
	Pending func(context.Context, requestintent.Scope, int64, int) ([]requestintent.Pending, error)
}

// Open acquires reads after argument validation; its nonnil release function owns
// cleanup on every later path. Credentials and authorization belong to the host.
type Open func(context.Context) (Reader, func(), error)

// Document is local diagnostic evidence, not a portable data contract or provider
// interchange message. Its shape follows this executable's internal projection.
// Data retains the version-one inward fingerprint projection, including base64
// binary evidence/payloads. Pending positions are local scan cursors, not leases.
type Document struct {
	Scope       requestintent.Scope      `json:"scope"`
	Fingerprint string                   `json:"fingerprint,omitempty"`
	Intent      *requestintent.Data      `json:"intent,omitempty"`
	Pending     *[]requestintent.Pending `json:"pending,omitempty"`
	NextAfter   *int64                   `json:"nextAfter,omitempty"`
}

// Run validates a mutually exclusive lookup/page operation before opening storage.
// It writes one complete document plus newline only after successful reads and a
// cancellation check. An output failure can leave partial JSON: require exit zero.
func Run(ctx context.Context, args []string, output io.Writer, open Open) error {
	flags := flag.NewFlagSet("performance-requests", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	review := flags.String("review-sha256", "", "exact stored review digest")
	pending := flags.Bool("pending", false, "inspect a pending evidence page")
	after := flags.Int64("after", 0, "database-local exclusive pending cursor")
	limit := flags.Int("limit", 100, "pending page size (1..100)")
	var scope requestintent.Scope
	flags.StringVar(&scope.Issuer, "issuer", "", "independently authorized provider issuer")
	flags.StringVar(&scope.PrincipalIssuer, "principal-issuer", "", "authorized principal issuer")
	flags.StringVar(&scope.PrincipalID, "principal-id", "", "authorized principal ID")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	pageOption := false
	flags.Visit(func(f *flag.Flag) { pageOption = pageOption || f.Name == "after" || f.Name == "limit" })
	raw, digestError := hex.DecodeString(*review)
	if flags.NArg() != 0 || !requestintent.ValidScope(scope) || output == nil || open == nil ||
		(*pending && *review != "") || (!*pending && (digestError != nil || len(raw) != 32 || strings.ToLower(*review) != *review || pageOption)) || *after < 0 || *limit < 1 || *limit > 100 {
		return commandline.UsageText("supply an authorized scope and either -review-sha256 or -pending [-after N] [-limit 1..100]")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	reader, release, err := open(ctx)
	if err != nil {
		return readError(ctx, err)
	}
	if release == nil {
		return requestintent.ErrUnavailable
	}
	defer release()
	document, err := read(ctx, reader, scope, *review, *pending, *after, *limit)
	if err != nil {
		return readError(ctx, err)
	}
	encoded, err := json.Marshal(document)
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

func read(ctx context.Context, reader Reader, scope requestintent.Scope, review string, pending bool, after int64, limit int) (Document, error) {
	document := Document{Scope: scope}
	if pending {
		if reader.Pending == nil {
			return Document{}, requestintent.ErrUnavailable
		}
		page, err := reader.Pending(ctx, scope, after, limit)
		if err != nil {
			return Document{}, err
		}
		// A nonnil cursor distinguishes an explicit empty page from an intent lookup.
		if page == nil {
			page = []requestintent.Pending{}
		}
		document.Pending, document.NextAfter = &page, &after
		if len(page) != 0 {
			*document.NextAfter = page[len(page)-1].After
		}

		return document, nil
	}
	if reader.Get == nil {
		return Document{}, requestintent.ErrUnavailable
	}
	intent, err := reader.Get(ctx, scope, review)
	if err != nil {
		return Document{}, err
	}
	data := intent.Data()
	if intent.Fingerprint() == "" || data.Scope != scope || data.ReviewSHA256 != review {
		return Document{}, requestintent.ErrUnavailable
	}
	document.Fingerprint, document.Intent = intent.Fingerprint(), &data

	return document, nil
}

func readError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, requestintent.ErrNotFound} {
		if errors.Is(err, known) {
			return known
		}
	}

	return requestintent.ErrUnavailable
}
