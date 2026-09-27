// Package validationcli owns isolated functional API repair validation input and output.
package validationcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/validation"
)

const (
	maxProposalBytes = 16 << 20
	maxTimeout       = 2 * time.Hour
)

// RuntimeFactory binds guarded workspace and process adapters after CLI input validates.
type RuntimeFactory func(
	string,
	[]string,
	io.Writer,
) (validation.Workspace, validation.Runner, error)

// Run validates one proposal in an explicitly supplied disposable checkout.
func Run(
	ctx context.Context,
	arguments []string,
	stdout io.Writer,
	stderr io.Writer,
	open RuntimeFactory,
) error {
	flags := flag.NewFlagSet("validate-functional-api-repair", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	proposalPath := flags.String("proposal", "", "adaptation proposal JSON path")
	workspaceRoot := flags.String("disposable-workspace", "", "disposable test checkout root")
	timeout := flags.Duration("timeout", 30*time.Minute, "maximum complete validation runtime")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("validate functional API repair: %w", err)
	}
	command := flags.Args()
	if *proposalPath == "" || *workspaceRoot == "" || len(command) == 0 {
		return errors.New("usage: validate-functional-api-repair -proposal <path> " +
			"-disposable-workspace <path> -- <command> [args...]")
	}
	if *timeout <= 0 || *timeout > maxTimeout {
		return errors.New("timeout must be greater than zero and at most 2h")
	}
	proposal, err := readProposal(*proposalPath)
	if err != nil {
		return err
	}
	if open == nil {
		return adaptation.ErrUnavailable
	}
	workspace, runner, err := open(*workspaceRoot, command, stderr)
	if err != nil {
		return fmt.Errorf("configure adaptation validation: %w", err)
	}
	validationContext, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	evidence, err := validation.NewService(workspace, runner).Validate(validationContext, proposal)
	if err != nil {
		return fmt.Errorf("validate functional API repair: %w", err)
	}
	document, err := adaptationcontract.ExportValidationEvidenceV1(evidence)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode validation evidence: %w", err)
	}

	return nil
}

func readProposal(path string) (adaptation.Proposal, error) {
	file, err := os.Open(path)
	if err != nil {
		return adaptation.Proposal{}, fmt.Errorf("open adaptation proposal: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxProposalBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return adaptation.Proposal{}, fmt.Errorf("read adaptation proposal: %w", readErr)
	}
	if closeErr != nil {
		return adaptation.Proposal{}, fmt.Errorf("close adaptation proposal: %w", closeErr)
	}
	if len(data) > maxProposalBytes {
		return adaptation.Proposal{}, errors.New("adaptation proposal exceeds 16 MiB limit")
	}
	document, err := contracts.DecodeAdaptationProposalV1(data)
	if err != nil {
		return adaptation.Proposal{}, err
	}

	return adaptationcontract.ImportProposalV1(document)
}
