// Package fieldvalidationcli owns the field validation command boundary.
package fieldvalidationcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldvalidation"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
)

// RuntimeFactory binds guarded workspace and runner ports after input validation.
type RuntimeFactory func(string, []string, io.Writer) (fieldvalidation.Workspace, fieldvalidation.Runner, error)

// Run validates one field proposal in an explicitly disposable workspace.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, open RuntimeFactory) error {
	flags := flag.NewFlagSet("validate-functional-api-field-repair", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	proposalPath := flags.String("proposal", "", "field proposal JSON path")
	workspaceRoot := flags.String("disposable-workspace", "", "disposable test checkout root")
	timeout := flags.Duration("timeout", 30*time.Minute, "maximum validation runtime")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *proposalPath == "" || *workspaceRoot == "" || len(flags.Args()) == 0 {
		return commandline.UsageText("usage: validate-functional-api-field-repair -proposal <path> -disposable-workspace <path> -- <adapter-command>")
	}
	if *timeout <= 0 || *timeout > 2*time.Hour {
		return errors.New("timeout must be greater than zero and at most 2h")
	}
	proposal, err := readProposal(*proposalPath)
	if err != nil {
		return err
	}

	return execute(ctx, proposal, *workspaceRoot, flags.Args(), *timeout, stdout, stderr, open)
}

func readProposal(path string) (fieldrepair.Proposal, error) {
	file, err := os.Open(path)
	if err != nil {
		return fieldrepair.Proposal{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	closeErr := file.Close()
	if readErr != nil {
		return fieldrepair.Proposal{}, readErr
	}
	if closeErr != nil {
		return fieldrepair.Proposal{}, closeErr
	}
	if len(data) > 16<<20 {
		return fieldrepair.Proposal{}, fmt.Errorf("%w: field proposal exceeds 16 MiB", adaptation.ErrInvalid)
	}
	doc, err := contracts.DecodeFieldAdaptationProposalV1(data)
	if err != nil {
		return fieldrepair.Proposal{}, err
	}

	return adaptationcontract.ImportFieldProposalV1(doc)
}

func execute(ctx context.Context, proposal fieldrepair.Proposal, root string, command []string, timeout time.Duration,
	stdout, stderr io.Writer, open RuntimeFactory) error {
	if open == nil {
		return adaptation.ErrUnavailable
	}
	workspace, runner, err := open(root, command, stderr)
	if err != nil {
		return err
	}
	validationCtx, stop := context.WithTimeout(ctx, timeout)
	defer stop()
	evidence, err := fieldvalidation.NewService(workspace, runner).Validate(validationCtx, proposal)
	if err != nil {
		var rejected *fieldvalidation.RejectedError
		if errors.As(err, &rejected) {
			doc, exportErr := adaptationcontract.ExportFieldValidationRejectionV1(rejected.Evidence)
			if exportErr != nil {
				return exportErr
			}
			if encodeErr := encode(stdout, doc); encodeErr != nil {
				return encodeErr
			}
		}

		return fmt.Errorf("validate field repair: %w", err)
	}
	docEvidence, err := adaptationcontract.ExportFieldValidationEvidenceV1(evidence)
	if err != nil {
		return err
	}

	return encode(stdout, docEvidence)
}
func encode(output io.Writer, document any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}
