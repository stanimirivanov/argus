package browser_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

func checkBrowserStageWire(t *testing.T, attempt browser.Attempt) {
	t.Helper()
	doc, err := executioncontract.ExportRecordedBrowserAttempt(attempt)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := executioncontract.DecodeRecordedBrowserAttempt(data)
	if err != nil || !reflect.DeepEqual(got, browser.CanonicalAttempt(attempt)) {
		t.Fatalf("wire mismatch: %+v %v", got, err)
	}
	if _, err := contracts.DecodeBrowserExecutionAttemptV1(data); err == nil {
		t.Fatal("v1 accepted staged evidence")
	}
}
