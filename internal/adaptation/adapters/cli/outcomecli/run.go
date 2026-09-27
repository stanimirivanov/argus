// Package outcomecli implements terminal adaptation-review capture.
package outcomecli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/outcome"
)

const (
	maxInputBytes = 16 << 20
	maxTimeout    = 10 * time.Minute
)

// GatewayFactory binds secret provider configuration after all local evidence validates.
type GatewayFactory func(apiURL, host, token string) (outcome.Gateway, error)

// Run captures one terminal provider review and emits structured learning evidence.
func Run(
	ctx context.Context,
	arguments []string,
	stdout io.Writer,
	getenv func(string) string,
	open GatewayFactory,
) error {
	flags := flag.NewFlagSet("capture-functional-api-review-outcome", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	proposalPath := flags.String("proposal", "", "adaptation proposal JSON path")
	evidencePath := flags.String("validation-evidence", "", "successful validation evidence JSON path")
	reviewPath := flags.String("review", "", "adaptation review publication JSON path")
	reasonCode := flags.String("reason-code", "", "explicit reviewer reason code")
	reasonNote := flags.String("reason-note", "", "optional bounded reviewer explanation")
	timeout := flags.Duration("timeout", 2*time.Minute, "maximum GitHub observation runtime")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("capture functional API review outcome: %w", err)
	}
	if *proposalPath == "" || *evidencePath == "" || *reviewPath == "" || *reasonCode == "" ||
		flags.NArg() != 0 {
		return usageError()
	}
	if *timeout <= 0 || *timeout > maxTimeout {
		return errors.New("timeout must be greater than zero and at most 10m")
	}
	proposal, evidence, publication, err := readInputs(*proposalPath, *evidencePath, *reviewPath)
	if err != nil {
		return err
	}
	if open == nil || getenv == nil {
		return adaptation.ErrUnavailable
	}
	apiURL := strings.TrimSpace(getenv("ARGUS_GITHUB_API_URL"))
	if apiURL == "" {
		apiURL = "https://api.github.com/"
	}
	host := strings.ToLower(strings.TrimSpace(getenv("ARGUS_GITHUB_HOST")))
	if host == "" {
		host = "github.com"
	}
	token := strings.TrimSpace(getenv("ARGUS_GITHUB_TOKEN"))
	if token == "" {
		return errors.New("ARGUS_GITHUB_TOKEN is required")
	}
	gateway, err := open(apiURL, host, token)
	if err != nil {
		return fmt.Errorf("configure GitHub review outcome capture: %w", err)
	}
	observationContext, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	reviewOutcome, err := outcome.NewService(gateway).Capture(
		observationContext, proposal, evidence, publication,
		adaptation.ReviewReasonCode(*reasonCode), *reasonNote,
	)
	if err != nil {
		return fmt.Errorf("capture functional API review outcome: %w", err)
	}
	document, err := adaptationcontract.ExportReviewOutcomeV1(reviewOutcome)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode review outcome: %w", err)
	}

	return nil
}

func readInputs(
	proposalPath string,
	evidencePath string,
	reviewPath string,
) (adaptation.Proposal, adaptation.ValidationEvidence, adaptation.ReviewPublication, error) {
	proposalData, err := readBounded(proposalPath, "adaptation proposal")
	if err != nil {
		return adaptation.Proposal{}, adaptation.ValidationEvidence{}, adaptation.ReviewPublication{}, err
	}
	proposalDocument, err := contracts.DecodeAdaptationProposalV1(proposalData)
	if err != nil {
		return adaptation.Proposal{}, adaptation.ValidationEvidence{}, adaptation.ReviewPublication{}, err
	}
	proposal, err := adaptationcontract.ImportProposalV1(proposalDocument)
	if err != nil {
		return adaptation.Proposal{}, adaptation.ValidationEvidence{}, adaptation.ReviewPublication{}, err
	}
	evidenceData, err := readBounded(evidencePath, "validation evidence")
	if err != nil {
		return adaptation.Proposal{}, adaptation.ValidationEvidence{}, adaptation.ReviewPublication{}, err
	}
	evidenceDocument, err := contracts.DecodeValidationEvidenceV1(evidenceData)
	if err != nil {
		return adaptation.Proposal{}, adaptation.ValidationEvidence{}, adaptation.ReviewPublication{}, err
	}
	evidence, err := adaptationcontract.ImportValidationEvidenceV1(evidenceDocument)
	if err != nil {
		return adaptation.Proposal{}, adaptation.ValidationEvidence{}, adaptation.ReviewPublication{}, err
	}
	reviewData, err := readBounded(reviewPath, "adaptation review")
	if err != nil {
		return adaptation.Proposal{}, adaptation.ValidationEvidence{}, adaptation.ReviewPublication{}, err
	}
	reviewDocument, err := contracts.DecodeAdaptationReviewV1(reviewData)
	if err != nil {
		return adaptation.Proposal{}, adaptation.ValidationEvidence{}, adaptation.ReviewPublication{}, err
	}
	publication, err := adaptationcontract.ImportReviewV1(reviewDocument)
	if err != nil {
		return adaptation.Proposal{}, adaptation.ValidationEvidence{}, adaptation.ReviewPublication{}, err
	}

	return proposal, evidence, publication, nil
}

func readBounded(path string, description string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", description, err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read %s: %w", description, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", description, closeErr)
	}
	if len(data) > maxInputBytes {
		return nil, fmt.Errorf("%s exceeds 16 MiB limit", description)
	}

	return data, nil
}

func usageError() error {
	return errors.New("usage: capture-functional-api-review-outcome -proposal <path> " +
		"-validation-evidence <path> -review <path> -reason-code <code> [-reason-note <text>]")
}
