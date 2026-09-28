// Package evidencecli owns durable adaptation evidence command boundaries.
package evidencecli

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/stanimirivanov/argus/contracts"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/outcome"
	"github.com/stanimirivanov/argus/internal/adaptation/validation"
)

const maxEvidenceDocumentBytes = 16 << 20

// Runtime owns the durable adaptation evidence ports used by this command.
type Runtime interface {
	outcome.EvidenceStore
	validation.RejectionStore
	Close()
}

// OpenRuntime creates infrastructure only after command input is validated.
type OpenRuntime func(context.Context, string) (Runtime, error)

// Run dispatches adaptation-evidence subcommands.
func Run(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	stdin io.Reader,
	stdout io.Writer,
	open OpenRuntime,
) error {
	if len(arguments) == 0 {
		return usageError()
	}
	switch arguments[0] {
	case "ingest":
		return runIngest(ctx, arguments[1:], databaseURL, stdin, stdout, open)
	case "get":
		return runGet(ctx, arguments[1:], databaseURL, stdout, open)
	case "ingest-validation-rejection":
		return runIngestValidationRejection(ctx, arguments[1:], databaseURL, stdin, stdout, open)
	case "get-validation-rejection":
		return runGetValidationRejection(ctx, arguments[1:], databaseURL, stdout, open)
	default:
		return usageError()
	}
}

func runIngestValidationRejection(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	stdin io.Reader,
	stdout io.Writer,
	open OpenRuntime,
) error {
	flags := flag.NewFlagSet("adaptation-evidence ingest-validation-rejection", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	filePath := flags.String("file", "", "validation rejection path, or - for stdin")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("adaptation-evidence ingest-validation-rejection: %w", err)
	}
	if *filePath == "" || flags.NArg() != 0 {
		return errors.New("usage: adaptation-evidence ingest-validation-rejection -file <path|->")
	}
	data, err := readEvidenceDocument(*filePath, stdin)
	if err != nil {
		return err
	}
	document, err := contracts.DecodeValidationRejectionV1(data)
	if err != nil {
		return fmt.Errorf("read validation rejection: %w", err)
	}
	evidence, err := adaptationcontract.ImportValidationRejectionV1(document)
	if err != nil {
		return fmt.Errorf("import validation rejection: %w", err)
	}
	runtime, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer runtime.Close()

	created, err := validation.NewEvidenceService(runtime).Ingest(ctx, evidence)
	if err != nil {
		return fmt.Errorf("ingest validation rejection: %w", err)
	}

	return encodeJSON(stdout, struct {
		ValidationID string `json:"validationId"`
		Created      bool   `json:"created"`
	}{ValidationID: evidence.ValidationID, Created: created})
}

func runGetValidationRejection(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	stdout io.Writer,
	open OpenRuntime,
) error {
	flags := flag.NewFlagSet("adaptation-evidence get-validation-rejection", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	validationID := flags.String("validation-id", "", "validation rejection SHA-256 identity")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("adaptation-evidence get-validation-rejection: %w", err)
	}
	if !validOutcomeID(*validationID) || flags.NArg() != 0 {
		return errors.New("usage: adaptation-evidence get-validation-rejection -validation-id <sha256>")
	}
	runtime, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer runtime.Close()

	evidence, err := validation.NewEvidenceService(runtime).Get(ctx, *validationID)
	if err != nil {
		return fmt.Errorf("get validation rejection: %w", err)
	}
	document, err := adaptationcontract.ExportValidationRejectionV1(evidence)
	if err != nil {
		return err
	}

	return encodeJSON(stdout, document)
}

func runIngest(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	stdin io.Reader,
	stdout io.Writer,
	open OpenRuntime,
) error {
	flags := flag.NewFlagSet("adaptation-evidence ingest", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	filePath := flags.String("file", "", "review outcome path, or - for stdin")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("adaptation-evidence ingest: %w", err)
	}
	if *filePath == "" || flags.NArg() != 0 {
		return errors.New("usage: adaptation-evidence ingest -file <path|->")
	}
	data, err := readEvidenceDocument(*filePath, stdin)
	if err != nil {
		return err
	}
	document, err := contracts.DecodeReviewOutcomeV1(data)
	if err != nil {
		return fmt.Errorf("read review outcome: %w", err)
	}
	reviewOutcome, err := adaptationcontract.ImportReviewOutcomeV1(document)
	if err != nil {
		return fmt.Errorf("import review outcome: %w", err)
	}
	runtime, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer runtime.Close()

	created, err := outcome.NewEvidenceService(runtime).Ingest(ctx, reviewOutcome)
	if err != nil {
		return fmt.Errorf("ingest review outcome: %w", err)
	}

	return encodeJSON(stdout, struct {
		OutcomeID string `json:"outcomeId"`
		Created   bool   `json:"created"`
	}{OutcomeID: reviewOutcome.OutcomeID, Created: created})
}

func runGet(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	stdout io.Writer,
	open OpenRuntime,
) error {
	flags := flag.NewFlagSet("adaptation-evidence get", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	outcomeID := flags.String("outcome-id", "", "review outcome SHA-256 identity")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("adaptation-evidence get: %w", err)
	}
	if !validOutcomeID(*outcomeID) || flags.NArg() != 0 {
		return errors.New("usage: adaptation-evidence get -outcome-id <sha256>")
	}
	runtime, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer runtime.Close()

	reviewOutcome, err := outcome.NewEvidenceService(runtime).Get(ctx, *outcomeID)
	if err != nil {
		return fmt.Errorf("get review outcome: %w", err)
	}
	document, err := adaptationcontract.ExportReviewOutcomeV1(reviewOutcome)
	if err != nil {
		return err
	}

	return encodeJSON(stdout, document)
}

func openStore(ctx context.Context, databaseURL string, open OpenRuntime) (Runtime, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("ARGUS_DATABASE_URL is required")
	}
	if open == nil {
		return nil, errors.New("adaptation evidence runtime factory is required")
	}
	runtime, err := open(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open adaptation evidence store: %w", err)
	}

	return runtime, nil
}

func readEvidenceDocument(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		if stdin == nil {
			return nil, errors.New("review outcome stdin is required")
		}
		return readBounded(stdin)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open review outcome: %w", err)
	}
	data, readErr := readBounded(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close review outcome: %w", closeErr)
	}

	return data, nil
}

func readBounded(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxEvidenceDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read review outcome: %w", err)
	}
	if len(data) > maxEvidenceDocumentBytes {
		return nil, errors.New("review outcome exceeds 16 MiB limit")
	}

	return data, nil
}

func validOutcomeID(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)

	return err == nil && strings.ToLower(value) == value
}

func encodeJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode adaptation evidence: %w", err)
	}

	return nil
}

func usageError() error {
	return errors.New("usage: adaptation-evidence " +
		"<ingest|get|ingest-validation-rejection|get-validation-rejection> [options]")
}
