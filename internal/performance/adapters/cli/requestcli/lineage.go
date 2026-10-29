package requestcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// RunLineage handles options after -lineage. The operation independently
// authorizes scope/position; submitted flags never supply approval. Require exit
// zero and complete JSON output; no retries, writes or provider calls occur here.
func RunLineage(ctx context.Context, args []string, output io.Writer, operation func(context.Context, int64) (requestintent.RerunRecord, error)) error {
	if output == nil || operation == nil {
		return commandline.UsageText("invalid lineage configuration")
	}
	flags := flag.NewFlagSet("performance-requests -lineage", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	position := flags.Int64("position", 0, "approved parent pending position")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *position <= 0 {
		return commandline.UsageText("lineage requires -position N")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	record, err := operation(ctx, *position)
	if err != nil {
		if errors.Is(err, requestintent.ErrNotFound) {
			return readError(ctx, err)
		}
		return creationError(ctx, err)
	}
	d := record.Evidence.Data()
	if record.Evidence.Fingerprint() == "" || d.ParentPosition != *position || record.SuccessorPosition <= *position {
		return requestintent.ErrUnavailable
	}
	raw, err := json.Marshal(struct {
		Scope             requestintent.Scope     `json:"scope"`
		SuccessorPosition int64                   `json:"successorPosition"`
		Fingerprint       string                  `json:"fingerprint"`
		Lineage           requestintent.RerunData `json:"lineage"`
	}{d.Scope, record.SuccessorPosition, record.Evidence.Fingerprint(), d})
	if err != nil {
		return requestintent.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	raw = append(raw, '\n')
	n, err := output.Write(raw)
	if err == nil && n != len(raw) {
		return io.ErrShortWrite
	}

	return err
}
