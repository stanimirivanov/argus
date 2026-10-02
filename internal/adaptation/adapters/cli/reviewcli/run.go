// Package reviewcli implements the adaptation review command's driving adapter.
package reviewcli

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

	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/endpointrepair"
	"github.com/stanimirivanov/argus/internal/adaptation/review"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
)

const (
	maxInputBytes = 16 << 20
	maxTimeout    = 10 * time.Minute
)

// GatewayFactory binds separate read and write authorities after inputs validate.
type GatewayFactory func(apiURL, host, readToken, writeToken string) (review.SourceReader, review.Publisher, error)

// Run validates local evidence before constructing the external-write adapter.
func Run(
	ctx context.Context,
	arguments []string,
	stdout io.Writer,
	getenv func(string) string,
	open GatewayFactory,
) error {
	flags := flag.NewFlagSet("open-functional-api-repair-pr", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	proposalPath := flags.String("proposal", "", "adaptation proposal JSON path")
	evidencePath := flags.String("validation-evidence", "", "successful validation evidence JSON path")
	baseBranch := flags.String("base-branch", "", "review pull request base branch")
	timeout := flags.Duration("timeout", 2*time.Minute, "maximum GitHub publication runtime")
	if err := commandline.Parse(flags, arguments); err != nil {
		return fmt.Errorf("open functional API repair pull request: %w", err)
	}
	if *proposalPath == "" || *evidencePath == "" || *baseBranch == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: open-functional-api-repair-pr -proposal <path> " +
			"-validation-evidence <path> -base-branch <branch>")
	}
	if *timeout <= 0 || *timeout > maxTimeout {
		return errors.New("timeout must be greater than zero and at most 10m")
	}
	proposal, err := readProposal(*proposalPath)
	if err != nil {
		return err
	}
	evidence, err := readEvidence(*evidencePath)
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
	readToken, writeToken := reviewTokens(getenv)
	if readToken == "" || writeToken == "" {
		return errors.New("ARGUS_GITHUB_READ_TOKEN and ARGUS_GITHUB_WRITE_TOKEN are required, or ARGUS_GITHUB_TOKEN for legacy single-token operation")
	}
	source, publisher, err := open(apiURL, host, readToken, writeToken)
	if err != nil {
		return fmt.Errorf("configure GitHub review publication: %w", err)
	}
	publicationContext, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	publication, err := review.NewService(source, publisher).Publish(publicationContext, proposal, evidence, *baseBranch)
	if err != nil {
		return fmt.Errorf("open functional API repair pull request: %w", err)
	}
	document, err := adaptationcontract.ExportReviewV1(publication)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode adaptation review: %w", err)
	}

	return nil
}

func reviewTokens(getenv func(string) string) (string, string) {
	legacy := strings.TrimSpace(getenv("ARGUS_GITHUB_TOKEN"))
	readToken := strings.TrimSpace(getenv("ARGUS_GITHUB_READ_TOKEN"))
	writeToken := strings.TrimSpace(getenv("ARGUS_GITHUB_WRITE_TOKEN"))
	if readToken == "" {
		readToken = legacy
	}
	if writeToken == "" {
		writeToken = legacy
	}

	return readToken, writeToken
}

func readProposal(path string) (endpointrepair.Proposal, error) {
	data, err := readBounded(path, "adaptation proposal")
	if err != nil {
		return endpointrepair.Proposal{}, err
	}
	document, err := contracts.DecodeAdaptationProposalV1(data)
	if err != nil {
		return endpointrepair.Proposal{}, err
	}

	return adaptationcontract.ImportProposalV1(document)
}

func readEvidence(path string) (adaptation.ValidationEvidence, error) {
	data, err := readBounded(path, "validation evidence")
	if err != nil {
		return adaptation.ValidationEvidence{}, err
	}
	document, err := contracts.DecodeValidationEvidenceV1(data)
	if err != nil {
		return adaptation.ValidationEvidence{}, err
	}

	return adaptationcontract.ImportValidationEvidenceV1(document)
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
