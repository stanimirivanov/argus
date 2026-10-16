package contracts

import (
	"encoding/json"
	"testing"
)

func TestMaintenanceAssessmentContractRejectsUnknownState(t *testing.T) {
	t.Parallel()
	valid := MaintenanceAssessmentV1{
		APIVersion:            "argus.dev/maintenance-assessment/v1",
		ImpactAPIVersion:      "argus.dev/capability-impact/v2",
		ImpactAnalyzerVersion: "argus-composite/v1",
		SourceRepositoryID:    "R_source_1", BaseRevision: "1111111111111111111111111111111111111111",
		HeadRevision: "2222222222222222222222222222222222222222",
		PolicyDigest: "", InventoryDigest: "",
		Items: []MaintenanceAssessmentItemV1{{
			RepositoryID: "R_tests_1", SuiteKey: "orders-api", TestKey: "list-orders",
			State: "INVALIDATED", Reason: "authoritative-endpoint-rename-and-test-failed",
			AttemptIDs: []string{"attempt-1"},
		}},
	}
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeMaintenanceAssessmentV1(data); err != nil {
		t.Fatalf("valid assessment: %v", err)
	}
	valid.Items[0].State = "PATCH_AUTHORIZED"
	data, err = json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeMaintenanceAssessmentV1(data); err == nil {
		t.Fatal("unknown state was accepted")
	}
}
