package contract

import (
	"encoding/json"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

func exportBrowserStaging(value *browser.Staging) contracts.BrowserStagedProvenanceV2 {
	return contracts.BrowserStagedProvenanceV2{StagedProvenanceV2: exportStaging(&value.Staging), DiscoverySHA256: value.DiscoverySHA256}
}
func importBrowserStaging(value contracts.BrowserStagedProvenanceV2) *browser.Staging {
	return &browser.Staging{Staging: *importStaging(value.StagedProvenanceV2), DiscoverySHA256: value.DiscoverySHA256}
}
func legacyBrowserRequest(request browser.Request) browser.Request {
	request.APIVersion, request.Stage, request.Staging, request.Candidates = browser.RequestVersion, "selected", nil, nil
	return request
}
func wireBrowserTests(tests []browser.Test) []contracts.RequestedFunctionalAPITest {
	output := make([]contracts.RequestedFunctionalAPITest, 0, len(tests))
	for _, test := range tests {
		output = append(output, contracts.RequestedFunctionalAPITest(test))
	}

	return output
}

// ExportRecordedBrowserRequest dispatches explicit v1/v2 protocols.
func ExportRecordedBrowserRequest(request browser.Request) (any, error) {
	if request.APIVersion != browser.StagedRequestVersion {
		return ExportBrowserRequest(request)
	}
	if err := browser.ValidateStagedRequest(request); err != nil {
		return nil, err
	}
	doc, err := ExportBrowserRequest(legacyBrowserRequest(request))
	if err != nil {
		return nil, err
	}
	doc.APIVersion, doc.Stage = request.APIVersion, string(request.Stage)
	output := contracts.BrowserAdapterRequestV2{BrowserAdapterRequestV1: doc, Staging: exportBrowserStaging(request.Staging), Candidates: wireBrowserTests(request.Candidates)}

	return output, contracts.ValidateStagedDocument(output, "browser-adapter-request")
}

// ExportRecordedBrowserAttempt retains v2 provenance without loosening v1.
func ExportRecordedBrowserAttempt(attempt browser.Attempt) (any, error) {
	if err := browser.ValidateRecordedAttempt(attempt); err != nil {
		return nil, err
	}
	if attempt.APIVersion == browser.AttemptVersion {
		return ExportBrowserAttempt(attempt)
	}
	legacy := attempt
	legacy.APIVersion = browser.AttemptVersion
	legacy.Request = legacyBrowserRequest(attempt.Request)
	doc, err := ExportBrowserAttempt(legacy)
	if err != nil {
		return nil, err
	}
	doc.APIVersion, doc.Stage, doc.Tests = attempt.APIVersion, string(attempt.Request.Stage), wireBrowserTests(attempt.Request.Tests)
	output := contracts.BrowserExecutionAttemptV2{BrowserExecutionAttemptV1: doc, Staging: exportBrowserStaging(attempt.Request.Staging), Candidates: wireBrowserTests(attempt.Request.Candidates)}

	return output, contracts.ValidateStagedDocument(output, "browser-execution-attempt")
}

// DecodeRecordedBrowserAttempt strictly validates the selected wire version.
func DecodeRecordedBrowserAttempt(data []byte) (browser.Attempt, error) {
	var header struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return browser.Attempt{}, err
	}
	if header.APIVersion != browser.StagedAttemptVersion {
		doc, err := contracts.DecodeBrowserExecutionAttemptV1(data)
		if err != nil {
			return browser.Attempt{}, err
		}

		return ImportBrowserAttempt(doc)
	}
	var doc contracts.BrowserExecutionAttemptV2
	if err := contracts.DecodeStagedDocument(data, "browser-execution-attempt", &doc); err != nil {
		return browser.Attempt{}, err
	}
	legacy := doc.BrowserExecutionAttemptV1
	legacy.APIVersion, legacy.Stage = browser.AttemptVersion, "selected"
	attempt, err := ImportBrowserAttempt(legacy)
	if err != nil {
		return browser.Attempt{}, err
	}
	attempt.APIVersion, attempt.Request.APIVersion, attempt.Request.Stage = browser.StagedAttemptVersion, browser.StagedRequestVersion, execution.Stage(doc.Stage)
	attempt.Request.Staging = importBrowserStaging(doc.Staging)
	// V1 conversion canonicalizes sets; restore v2's execution order explicitly.
	attempt.Request.Tests = []browser.Test{}
	for _, test := range doc.Tests {
		attempt.Request.Tests = append(attempt.Request.Tests, browser.Test(test))
	}
	for _, test := range doc.Candidates {
		attempt.Request.Candidates = append(attempt.Request.Candidates, browser.Test(test))
	}

	return browser.CanonicalAttempt(attempt), browser.ValidateRecordedAttempt(attempt)
}
