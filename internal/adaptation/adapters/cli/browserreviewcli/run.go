// Package browserreviewcli exposes browser locator draft publication and outcome capture.
package browserreviewcli

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
	"github.com/stanimirivanov/argus/internal/adaptation/browservalidation"
	"github.com/stanimirivanov/argus/internal/adaptation/outcome"
	"github.com/stanimirivanov/argus/internal/adaptation/review"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
)

// PublicationFactory separates immutable source reads from draft PR writes.
type PublicationFactory func(apiURL, host, readToken, writeToken string) (review.SourceReader, review.Publisher, error)

// OutcomeFactory uses read-only provider authority for both observation and final source checks.
type OutcomeFactory func(apiURL, host, token string) (outcome.Gateway, review.SourceReader, error)

// Publish emits one draft review for a validated browser locator proof.
func Publish(ctx context.Context, args []string, stdout io.Writer, getenv func(string) string, open PublicationFactory) error {
	flags := flag.NewFlagSet("open-browser-locator-repair-pr", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	evidencePath := flags.String("validation-evidence", "", "browser validation evidence JSON path")
	baseBranch := flags.String("base-branch", "", "review base branch")
	timeout := flags.Duration("timeout", 2*time.Minute, "maximum publication runtime")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *evidencePath == "" || *baseBranch == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: open-browser-locator-repair-pr -validation-evidence <path> -base-branch <branch>")
	}
	if *timeout <= 0 || *timeout > 10*time.Minute {
		return errors.New("timeout must be greater than zero and at most 10m")
	}
	evidence, err := readEvidence(*evidencePath)
	if err != nil {
		return err
	}
	if evidence.Status != browservalidation.Validated {
		return adaptation.ErrInvalid
	}
	if open == nil || getenv == nil {
		return adaptation.ErrUnavailable
	}
	apiURL, host := provider(getenv)
	readToken := token(getenv, "ARGUS_GITHUB_READ_TOKEN")
	writeToken := token(getenv, "ARGUS_GITHUB_WRITE_TOKEN")
	if readToken == "" || writeToken == "" {
		return errors.New("GitHub read and write tokens are required")
	}
	source, publisher, err := open(apiURL, host, readToken, writeToken)
	if err != nil {
		return err
	}
	operation, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	publication, err := review.NewService(source, publisher).PublishBrowser(operation, evidence, *baseBranch)
	if err != nil {
		return err
	}
	document, err := adaptationcontract.ExportReviewV1(publication)
	if err != nil {
		return err
	}

	return encode(stdout, document)
}

// Capture emits terminal reviewer evidence after verifying any edited source.
func Capture(ctx context.Context, args []string, stdout io.Writer, getenv func(string) string, open OutcomeFactory) error {
	flags := flag.NewFlagSet("capture-browser-locator-review-outcome", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	evidencePath := flags.String("validation-evidence", "", "browser validation evidence JSON path")
	reviewPath := flags.String("review", "", "published review JSON path")
	reason := flags.String("reason-code", "", "explicit reviewer reason")
	note := flags.String("reason-note", "", "optional bounded explanation")
	timeout := flags.Duration("timeout", 2*time.Minute, "maximum observation runtime")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *evidencePath == "" || *reviewPath == "" || *reason == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: capture-browser-locator-review-outcome -validation-evidence <path> -review <path> -reason-code <code>")
	}
	if *timeout <= 0 || *timeout > 10*time.Minute {
		return errors.New("timeout must be greater than zero and at most 10m")
	}
	evidence, err := readEvidence(*evidencePath)
	if err != nil {
		return err
	}
	data, err := readBounded(*reviewPath)
	if err != nil {
		return err
	}
	document, err := contracts.DecodeAdaptationReviewV1(data)
	if err != nil {
		return err
	}
	publication, err := adaptationcontract.ImportReviewV1(document)
	if err != nil {
		return err
	}
	if open == nil || getenv == nil {
		return adaptation.ErrUnavailable
	}
	apiURL, host := provider(getenv)
	readToken := token(getenv, "ARGUS_GITHUB_READ_TOKEN")
	if readToken == "" {
		return errors.New("GitHub read token is required")
	}
	gateway, source, err := open(apiURL, host, readToken)
	if err != nil {
		return err
	}
	operation, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	result, err := outcome.NewService(gateway).CaptureBrowser(operation, evidence, publication, source,
		adaptation.ReviewReasonCode(*reason), *note)
	if err != nil {
		return err
	}
	out, err := adaptationcontract.ExportReviewOutcomeV1(result)
	if err != nil {
		return err
	}

	return encode(stdout, out)
}

func readEvidence(path string) (browservalidation.Evidence, error) {
	data, err := readBounded(path)
	if err != nil {
		return browservalidation.Evidence{}, err
	}
	document, err := contracts.DecodeBrowserLocatorValidationV1(data)
	if err != nil {
		return browservalidation.Evidence{}, err
	}

	return adaptationcontract.ImportBrowserLocatorValidationV1(document, executioncontract.ImportBrowserAttempt)
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
		return nil, fmt.Errorf("%w: browser review input exceeds 16 MiB", adaptation.ErrInvalid)
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

func token(getenv func(string) string, name string) string {
	if value := strings.TrimSpace(getenv(name)); value != "" {
		return value
	}
	return strings.TrimSpace(getenv("ARGUS_GITHUB_TOKEN"))
}

func encode(writer io.Writer, document any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}
