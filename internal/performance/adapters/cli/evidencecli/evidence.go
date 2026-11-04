// Package evidencecli owns the argument/output boundary for authorized metadata export.
package evidencecli

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// Query identifies a resource; it never grants access.
type Query struct {
	Scope    requestintent.Scope
	Position int64
}

// Approval is independently issued protected host policy, not submitted evidence.
// Canonical JSON uses the existing inward scope's Issuer/PrincipalIssuer/PrincipalID.
type Approval struct {
	Scope             requestintent.Scope `json:"scope"`
	Position          int64               `json:"position"`
	IntentFingerprint string              `json:"intentFingerprint"`
	RequestSHA256     string              `json:"requestSha256"`
	ExpectedReviewer  string              `json:"expectedReviewer"`
}

// ErrDenied exposes no policy values or resource-existence detail.
var ErrDenied = errors.New("performance evidence access denied")

// Authorize matches an exact scope/position before any database acquisition.
// The host must authenticate issuance and protect the file independently of input.
func Authorize(raw []byte, query Query) (Approval, error) {
	var approval Approval
	if len(raw) > 64<<10 || !utf8.Valid(raw) || json.Unmarshal(raw, &approval) != nil {
		return Approval{}, ErrDenied
	}
	canonical, err := json.Marshal(approval)
	if err != nil || !bytes.Equal(bytes.TrimSpace(raw), canonical) || approval.Position <= 0 || approval.Position != query.Position || !requestintent.ValidScope(approval.Scope) || approval.Scope != query.Scope || !digest(approval.IntentFingerprint) || !digest(approval.RequestSHA256) || strings.TrimSpace(approval.ExpectedReviewer) == "" || utf8.RuneCountInString(approval.ExpectedReviewer) > 256 {
		return Approval{}, ErrDenied
	}
	for _, r := range approval.ExpectedReviewer {
		if unicode.IsControl(r) {
			return Approval{}, ErrDenied
		}
	}

	return approval, nil
}

func digest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && strings.ToLower(value) == value
}

// Export must authorize before storage, return only a complete validated public
// document, and perform no writes. Failure may never return private diagnostics.
type Export func(context.Context, Query) ([]byte, error)

// Run emits one validated document only after export succeeds. A failed/short
// output write requires discarding the destination; stdout is not transactional.
func Run(ctx context.Context, args []string, output io.Writer, export Export) error {
	if output == nil || export == nil {
		return commandline.UsageText("invalid performance evidence configuration")
	}
	flags := flag.NewFlagSet("performance-evidence", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var query Query
	flags.StringVar(&query.Scope.Issuer, "issuer", "", "provider issuer")
	flags.StringVar(&query.Scope.PrincipalIssuer, "principal-issuer", "", "principal issuer")
	flags.StringVar(&query.Scope.PrincipalID, "principal-id", "", "principal identity")
	flags.Int64Var(&query.Position, "position", 0, "retained child position")
	if flags.Parse(args) != nil || flags.NArg() != 0 || query.Position <= 0 || !requestintent.ValidScope(query.Scope) {
		return commandline.UsageText("explicit performance evidence scope and position required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := export(ctx, query)
	if err != nil {
		if errors.Is(err, ErrDenied) {
			return ErrDenied
		}
		return errors.New("performance evidence unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(raw) == 0 || len(raw) > 8<<20 {
		return errors.New("performance evidence unavailable")
	}
	n, err := output.Write(raw)
	if err == nil && n != len(raw) {
		return io.ErrShortWrite
	}

	return err
}
