package contract

import (
	"encoding/json"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/functionalapi"
)

func exportStaging(value *execution.Staging) contracts.StagedProvenanceV2 {
	return contracts.StagedProvenanceV2{Schedule: contracts.ManifestDigestReference{APIVersion: value.Schedule.APIVersion, SHA256: value.Schedule.SHA256},
		EpisodeID: value.EpisodeID, GroupKey: value.GroupKey, Environment: value.Environment, Mode: value.Mode}
}

func importStaging(value contracts.StagedProvenanceV2) *execution.Staging {
	return &execution.Staging{Schedule: execution.ScheduleReference{APIVersion: value.Schedule.APIVersion, SHA256: value.Schedule.SHA256},
		EpisodeID: value.EpisodeID, GroupKey: value.GroupKey, Environment: value.Environment, Mode: value.Mode}
}

// ExportRequest selects the explicit v1/v2 input contract. Results retain the
// same v1 normalized shape and require exact correlation in either protocol.
func ExportRequest(request functionalapi.Request) (any, error) {
	if request.APIVersion != functionalapi.StagedRequestAPIVersion {
		return ExportRequestV1(request)
	}
	if err := functionalapi.ValidateStagedRequest(request); err != nil {
		return nil, err
	}
	legacy := request
	legacy.APIVersion, legacy.Stage, legacy.Staging = functionalapi.AdapterRequestAPIVersion, execution.StageSelected, nil
	doc, err := ExportRequestV1(legacy)
	if err != nil {
		return nil, err
	}
	doc.APIVersion, doc.Stage = request.APIVersion, string(request.Stage)
	// v2 order is the portfolio order rather than v1 set canonicalization.
	doc.Tests = []contracts.RequestedFunctionalAPITest{}
	for _, test := range request.Tests {
		doc.Tests = append(doc.Tests, contracts.RequestedFunctionalAPITest{SuiteKey: test.SuiteKey, TestKey: test.TestKey, Name: test.Name})
	}
	output := contracts.FunctionalAPIAdapterRequestV2{FunctionalAPIAdapterRequestV1: doc, Staging: exportStaging(request.Staging)}

	return output, contracts.ValidateStagedDocument(output, "functional-api-adapter-request")
}

// ExportRecordedAttempt preserves strict v1 and versioned staged evidence.
func ExportRecordedAttempt(attempt execution.Attempt) (any, error) {
	if err := execution.ValidateRecordedAttempt(attempt); err != nil {
		return nil, err
	}
	if attempt.APIVersion == execution.AttemptAPIVersion {
		return ExportAttemptV1(attempt)
	}
	legacy := attempt
	legacy.APIVersion, legacy.Stage, legacy.Staging = execution.AttemptAPIVersion, execution.StageSelected, nil
	doc, err := ExportAttemptV1(legacy)
	if err != nil {
		return nil, err
	}
	doc.APIVersion, doc.Stage = attempt.APIVersion, string(attempt.Stage)
	output := contracts.ExecutionAttemptV2{ExecutionAttemptV1: doc, Staging: exportStaging(attempt.Staging)}

	return output, contracts.ValidateStagedDocument(output, "execution-attempt")
}

// DecodeRecordedAttempt explicitly dispatches versions before strict decoding.
func DecodeRecordedAttempt(data []byte) (execution.Attempt, error) {
	var header struct {
		APIVersion string `json:"apiVersion"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return execution.Attempt{}, err
	}
	if header.APIVersion != execution.StagedAttemptAPIVersion {
		doc, err := contracts.DecodeExecutionAttemptV1(data)
		if err != nil {
			return execution.Attempt{}, err
		}

		return ImportAttemptV1(doc)
	}
	var doc contracts.ExecutionAttemptV2
	if err := contracts.DecodeStagedDocument(data, "execution-attempt", &doc); err != nil {
		return execution.Attempt{}, err
	}
	legacy := doc.ExecutionAttemptV1
	legacy.APIVersion, legacy.Stage = execution.AttemptAPIVersion, string(execution.StageSelected)
	attempt, err := ImportAttemptV1(legacy)
	if err != nil {
		return execution.Attempt{}, err
	}
	attempt.APIVersion, attempt.Stage, attempt.Staging = header.APIVersion, execution.Stage(doc.Stage), importStaging(doc.Staging)

	return attempt, execution.ValidateRecordedAttempt(attempt)
}
