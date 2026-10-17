// Package fieldreviewcli exposes the field-specific draft and terminal review commands.
package fieldreviewcli

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
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldvalidation"
	"github.com/stanimirivanov/argus/internal/adaptation/outcome"
	"github.com/stanimirivanov/argus/internal/adaptation/review"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
)

// PublicationFactory binds scoped read and write provider authorities.
type PublicationFactory func(apiURL, host, readToken, writeToken string) (review.SourceReader, review.Publisher, error)

// OutcomeFactory binds read-only terminal review observation.
type OutcomeFactory func(apiURL, host, token string) (outcome.Gateway, error)

// Publish validates the field proof before any provider write.
func Publish(ctx context.Context, args []string, stdout io.Writer, getenv func(string) string, open PublicationFactory) error {
	flags := flag.NewFlagSet("open-functional-api-field-repair-pr", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	proposalPath := flags.String("proposal", "", "field proposal JSON path")
	evidencePath := flags.String("validation-evidence", "", "field validation evidence JSON path")
	baseBranch := flags.String("base-branch", "", "review base branch")
	timeout := flags.Duration("timeout", 2*time.Minute, "maximum publication runtime")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *proposalPath == "" || *evidencePath == "" || *baseBranch == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: open-functional-api-field-repair-pr -proposal <path> -validation-evidence <path> -base-branch <branch>")
	}
	if *timeout <= 0 || *timeout > 10*time.Minute {
		return errors.New("timeout must be greater than zero and at most 10m")
	}
	proposal, evidence, err := readInputs(*proposalPath, *evidencePath)
	if err != nil {
		return err
	}
	if open == nil || getenv == nil {
		return adaptation.ErrUnavailable
	}
	apiURL, host := provider(getenv)
	readToken, writeToken := tokens(getenv)
	if readToken == "" || writeToken == "" {
		return errors.New("GitHub read and write tokens are required")
	}
	source, publisher, err := open(apiURL, host, readToken, writeToken)
	if err != nil {
		return err
	}
	operationCtx, stop := context.WithTimeout(ctx, *timeout)
	defer stop()
	publication, err := review.NewService(source, publisher).PublishField(operationCtx, proposal, evidence, *baseBranch)
	if err != nil {
		return err
	}
	doc, err := adaptationcontract.ExportReviewV1(publication)
	if err != nil {
		return err
	}

	return encode(stdout, doc)
}

// Capture records terminal human review evidence for a field proposal.
func Capture(ctx context.Context, args []string, stdout io.Writer, getenv func(string) string, open OutcomeFactory) error {
	flags := flag.NewFlagSet("capture-functional-api-field-review-outcome", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	proposalPath := flags.String("proposal", "", "field proposal JSON path")
	evidencePath := flags.String("validation-evidence", "", "field validation evidence JSON path")
	reviewPath := flags.String("review", "", "draft review JSON path")
	reason := flags.String("reason-code", "", "explicit human review reason")
	note := flags.String("reason-note", "", "optional bounded explanation")
	timeout := flags.Duration("timeout", 2*time.Minute, "maximum observation runtime")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *proposalPath == "" || *evidencePath == "" || *reviewPath == "" || *reason == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: capture-functional-api-field-review-outcome -proposal <path> -validation-evidence <path> -review <path> -reason-code <code>")
	}
	if *timeout <= 0 || *timeout > 10*time.Minute {
		return errors.New("timeout must be greater than zero and at most 10m")
	}
	proposal, evidence, err := readInputs(*proposalPath, *evidencePath)
	if err != nil {
		return err
	}
	reviewData, err := readBounded(*reviewPath)
	if err != nil {
		return err
	}
	reviewDoc, err := contracts.DecodeAdaptationReviewV1(reviewData)
	if err != nil {
		return err
	}
	publication, err := adaptationcontract.ImportReviewV1(reviewDoc)
	if err != nil {
		return err
	}
	if open == nil || getenv == nil {
		return adaptation.ErrUnavailable
	}
	apiURL, host := provider(getenv)
	token, _ := tokens(getenv)
	if token == "" {
		return errors.New("GitHub read token is required")
	}
	gateway, err := open(apiURL, host, token)
	if err != nil {
		return err
	}
	operationCtx, stop := context.WithTimeout(ctx, *timeout)
	defer stop()
	result, err := outcome.NewService(gateway).CaptureField(operationCtx, proposal, evidence, publication,
		adaptation.ReviewReasonCode(*reason), *note)
	if err != nil {
		return err
	}
	doc, err := adaptationcontract.ExportReviewOutcomeV1(result)
	if err != nil {
		return err
	}

	return encode(stdout, doc)
}

func readInputs(proposalPath, evidencePath string) (fieldrepair.Proposal, fieldvalidation.Evidence, error) {
	proposalData, err := readBounded(proposalPath)
	if err != nil {
		return fieldrepair.Proposal{}, fieldvalidation.Evidence{}, err
	}
	proposalDoc, err := contracts.DecodeFieldAdaptationProposalV1(proposalData)
	if err != nil {
		return fieldrepair.Proposal{}, fieldvalidation.Evidence{}, err
	}
	proposal, err := adaptationcontract.ImportFieldProposalV1(proposalDoc)
	if err != nil {
		return fieldrepair.Proposal{}, fieldvalidation.Evidence{}, err
	}
	evidenceData, err := readBounded(evidencePath)
	if err != nil {
		return fieldrepair.Proposal{}, fieldvalidation.Evidence{}, err
	}
	evidenceDoc, err := contracts.DecodeFieldValidationEvidenceV1(evidenceData)
	if err != nil {
		return fieldrepair.Proposal{}, fieldvalidation.Evidence{}, err
	}
	evidence, err := adaptationcontract.ImportFieldValidationEvidenceV1(evidenceDoc)

	return proposal, evidence, err
}
func readBounded(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, 16<<20+1))
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > 16<<20 {
		return nil, fmt.Errorf("%w: field review input exceeds 16 MiB", adaptation.ErrInvalid)
	}

	return data, nil
}
func provider(getenv func(string) string) (string, string) {
	apiURL := strings.TrimSpace(getenv("ARGUS_GITHUB_API_URL"))
	if apiURL == "" {
		apiURL = "https://api.github.com/"
	}
	host := strings.ToLower(strings.TrimSpace(getenv("ARGUS_GITHUB_HOST")))
	if host == "" {
		host = "github.com"
	}

	return apiURL, host
}
func tokens(getenv func(string) string) (string, string) {
	legacy := strings.TrimSpace(getenv("ARGUS_GITHUB_TOKEN"))
	read := strings.TrimSpace(getenv("ARGUS_GITHUB_READ_TOKEN"))
	if read == "" {
		read = legacy
	}
	write := strings.TrimSpace(getenv("ARGUS_GITHUB_WRITE_TOKEN"))
	if write == "" {
		write = legacy
	}

	return read, write
}
func encode(writer io.Writer, document any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}
