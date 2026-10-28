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

// CancellationCommand carries submitted intent or a read request, never authority.
type CancellationCommand struct {
	Position int64
	Record   bool
	Payload  []byte
}

// CancellationResult is owned local evidence, never provider termination state.
type CancellationResult struct {
	Scope   requestintent.Scope
	Intent  requestintent.Cancellation
	Created bool
}

// CancellationOperation consumes authenticated host authority before opening scoped storage.
// Errors/output failures may leave a committed intent; callers never retry here.
type CancellationOperation func(context.Context, CancellationCommand) (CancellationResult, error)

// RunCancellation parses leading -cancel/-cancellation modes before effects and
// emits one diagnostic JSON document after success. Require exit zero: short output
// or cancellation cannot roll back intent and must not be interpreted as success.
func RunCancellation(ctx context.Context, args []string, output io.Writer, operation CancellationOperation) error {
	if output == nil || operation == nil {
		return commandline.UsageText("invalid cancellation configuration")
	}
	command, err := cancellationInput(ctx, args)
	if err != nil {
		return err
	}
	result, err := operation(ctx, command)
	if err != nil {
		if errors.Is(err, requestintent.ErrNotFound) {
			return readError(ctx, err)
		}
		return creationError(ctx, err)
	}
	if result.Intent.Fingerprint() == "" || !requestintent.ValidScope(result.Scope) {
		return requestintent.ErrUnavailable
	}
	var created *bool
	if command.Record {
		created = &result.Created
	}
	raw, err := json.Marshal(struct {
		Scope       requestintent.Scope            `json:"scope"`
		Position    int64                          `json:"position"`
		Created     *bool                          `json:"created,omitempty"`
		Fingerprint string                         `json:"fingerprint"`
		Intent      requestintent.CancellationData `json:"intent"`
	}{result.Scope, command.Position, created, result.Intent.Fingerprint(), result.Intent.Data()})
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

func cancellationInput(ctx context.Context, args []string) (CancellationCommand, error) {
	flags := flag.NewFlagSet("performance-requests cancellation", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	record := flags.Bool("cancel", false, "record explicitly approved cancellation intent")
	inspect := flags.Bool("cancellation", false, "inspect approved scoped cancellation intent")
	position := flags.Int64("position", 0, "exact database-local pending position")
	message := flags.String("message", "", "cancel-requested.v1 file")
	if err := commandline.Parse(flags, args); err != nil {
		return CancellationCommand{}, err
	}
	messageSet := false
	flags.Visit(func(f *flag.Flag) { messageSet = messageSet || f.Name == "message" })
	if flags.NArg() != 0 || *position <= 0 || *record == *inspect || (*record && *message == "") || (*inspect && messageSet) {
		return CancellationCommand{}, commandline.UsageText("supply -cancel -position N -message FILE or -cancellation -position N")
	}
	if err := ctx.Err(); err != nil {
		return CancellationCommand{}, err
	}
	command := CancellationCommand{Position: *position, Record: *record}
	if *record {
		var err error
		command.Payload, err = ReadCreationFile(*message)
		if err != nil {
			return CancellationCommand{}, err
		}
		if len(command.Payload) == 0 || len(command.Payload) > 64<<10 {
			return CancellationCommand{}, requestintent.ErrInvalid
		}
	}
	if err := ctx.Err(); err != nil {
		return CancellationCommand{}, err
	}

	return command, nil
}
