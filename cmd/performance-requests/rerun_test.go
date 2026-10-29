package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	changecontract "github.com/stanimirivanov/argus/internal/change/adapters/contract"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/requestcli"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/adapters/selectionio"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type operatorRerunMemory struct {
	t         *testing.T
	parent    requestintent.RerunParent
	record    requestintent.RerunRecord
	successor requestintent.Intent
	writes    int
	err       error
}

func (s *operatorRerunMemory) GetRequest(_ context.Context, scope requestintent.Scope, review string) (requestintent.Intent, error) {
	if scope != s.parent.Intent.Data().Scope || review != s.parent.Intent.Data().ReviewSHA256 {
		return requestintent.Intent{}, requestintent.ErrNotFound
	}
	return s.parent.Intent, nil
}
func (s *operatorRerunMemory) PendingRequests(_ context.Context, scope requestintent.Scope, after int64, limit int) ([]requestintent.Pending, error) {
	if scope != s.parent.Intent.Data().Scope || after != 6 || limit != 1 {
		s.t.Fatal("unscoped parent read")
	}
	return []requestintent.Pending{s.parent.Pending}, nil
}
func (s *operatorRerunMemory) RequestOutcome(context.Context, requestintent.Scope, int64) (requestintent.Outcome, error) {
	return s.parent.Admission.Data().Evidence, nil
}
func (s *operatorRerunMemory) LifecycleHistory(_ context.Context, _ requestintent.Scope, _ int64, a requestintent.LifecycleAdmission, verify requestintent.LifecycleVerifier) ([]requestintent.LifecycleEvent, error) {
	if _, err := verify(a.Data().Evidence); err != nil {
		s.t.Fatal(err)
	}
	return s.parent.History, nil
}
func (s *operatorRerunMemory) CreateReruns(_ context.Context, intent requestintent.Intent, bindings []requestintent.RerunCreation) (bool, error) {
	if intent.Fingerprint() != s.successor.Fingerprint() || len(bindings) != 1 || bindings[0].Evidence.Fingerprint() != s.record.Evidence.Fingerprint() {
		s.t.Fatal("unapproved atomic write")
	}
	s.writes++

	return s.writes == 1, s.err
}
func (s *operatorRerunMemory) RerunSuccessor(_ context.Context, scope requestintent.Scope, position int64) (requestintent.RerunRecord, error) {
	if scope != s.parent.Intent.Data().Scope || position != 7 {
		s.t.Fatal("unscoped lineage read")
	}
	return s.record, s.err
}

func operatorRerunFixture(t *testing.T) (rerunApproval, requestcli.Creation, selectionio.RequestReview, creationStorage, *operatorRerunMemory) {
	t.Helper()
	approval, err := selectionio.DecodeRequestApproval(operatorFile(t, "testdata/approval.json"))
	if err != nil {
		t.Fatal(err)
	}
	review, err := selectionio.DecodeRequestReview(operatorFile(t, "testdata/review.json"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := contracts.DecodeCapabilityImpactV1(operatorFile(t, "../../contracts/fixtures/capability-impact/v1/valid/orders-api.json"))
	if err != nil {
		t.Fatal(err)
	}
	impact, err := changecontract.ImportCapabilityImpactV1(document)
	if err != nil {
		t.Fatal(err)
	}
	var pages []json.RawMessage
	if err := json.Unmarshal(operatorFile(t, "../../internal/performance/adapters/perfeng/snapshot/pages.json"), &pages); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	snapshot, err := perfeng.Import(pages, approval.Expected, at)
	if err != nil {
		t.Fatal(err)
	}
	var recorder creationRecorder
	storage := recorder.storage(t, impact, snapshot)
	request := requestcli.Creation{DeliveryID: impact.Change.Trigger.DeliveryID, Policy: operatorFile(t, "testdata/policy.json"), Review: operatorFile(t, "testdata/review.json"), Now: func() time.Time { return at }}
	successor, _, err := createStored(t.Context(), request, review, approval, storage)
	if err != nil {
		t.Fatal(err)
	}
	old := successor.Data()
	child := old.Children[0]
	original, err := perfeng.DecodeIntegration(child.Payload)
	if err != nil {
		t.Fatal(err)
	}
	original.ID, original.Data.Context.Request.ID, original.Data.Context.Attempt.ID = "old-requested", "old-request", "old-attempt"
	original.Time = at.Add(-time.Minute).Format(time.RFC3339Nano)
	original.Data.Context.RequestedAt = original.Time
	child.Payload, err = perfeng.EncodeIntegration(original)
	if err != nil {
		t.Fatal(err)
	}
	child.Request.ID, child.Attempt.ID, child.EventID = "old-request", "old-attempt", original.ID
	child.RequestedAt = at.Add(-time.Minute)
	child.SHA256 = cancellationHash(child.Payload)
	old.Children = []requestintent.Child{child}
	old.ReviewSHA256 = cancellationHash([]byte("previous-review"))
	previous, err := requestintent.New(old)
	if err != nil {
		t.Fatal(err)
	}
	var exchange []json.RawMessage
	if err := json.Unmarshal(operatorFile(t, "../../internal/performance/adapters/perfeng/snapshot/integration/cancelled.json"), &exchange); err != nil {
		t.Fatal(err)
	}
	accepted, err := perfeng.DecodeIntegration(exchange[1])
	if err != nil {
		t.Fatal(err)
	}
	accepted.ID, accepted.Time, accepted.Data.Context = "old-admission", original.Time, original.Data.Context
	accepted.Data.Run.ID = "old-run"
	outcome, err := perfeng.DispatchOutcome(child.Payload, accepted, approval.Expected.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := perfeng.BindLifecycleAdmission(child.Payload, outcome, approval.Expected.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := perfeng.DecodeIntegration(exchange[4])
	if err != nil {
		t.Fatal(err)
	}
	terminal.ID, terminal.Time, terminal.Data.Context, terminal.Data.Run = "old-terminal", at.Add(-time.Second).Format(time.RFC3339Nano), original.Data.Context, accepted.Data.Run
	event, err := perfeng.LifecycleObservation(child.Payload, terminal, approval.Expected.Issuer)
	if err != nil {
		t.Fatal(err)
	}
	parent := requestintent.RerunParent{Pending: requestintent.Pending{After: 7, Child: child}, Intent: previous, Admission: admission, History: []requestintent.LifecycleEvent{event}}
	proof, err := requestintent.NewRerun(parent, successor, 0)
	if err != nil {
		t.Fatal(err)
	}
	a := rerunApproval{Request: approval, Parents: []rerunParentPin{{Position: 7, ReviewSHA256: old.ReviewSHA256, Ordinal: 0, RerunSHA256: proof.Fingerprint()}}}

	return a, request, review, storage, &operatorRerunMemory{t: t, parent: parent, record: requestintent.RerunRecord{SuccessorPosition: 8, Evidence: proof}, successor: successor}
}

func TestOperatorRerunFreshlyCompilesAndChecksEveryProtectedPin(t *testing.T) {
	a, request, review, storage, memory := operatorRerunFixture(t)
	storage.create = func(ctx context.Context, intent requestintent.Intent) (bool, error) {
		return createApprovedRerun(ctx, intent, a, memory)
	}
	if _, fresh, err := createStored(t.Context(), request, review, a.Request, storage); err != nil || !fresh || memory.writes != 1 {
		t.Fatal("approved rerun failed", err)
	}
	a.Parents[0].RerunSHA256 = strings.Repeat("d", 64)
	if _, _, err := createStored(t.Context(), request, review, a.Request, storage); !errors.Is(err, requestintent.ErrConflict) || memory.writes != 1 {
		t.Fatal("changed parent pin wrote", err)
	}
	a.Parents[0].RerunSHA256 = memory.record.Evidence.Fingerprint()
	request.Now = func() time.Time { return time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC) }
	if _, _, err := createStored(t.Context(), request, review, a.Request, storage); err == nil || memory.writes != 1 {
		t.Fatal("stale compilation wrote")
	}
	if got, err := lineageStored(t.Context(), a.scope(), a.Parents[0], memory); err != nil || got.Evidence.Fingerprint() != memory.record.Evidence.Fingerprint() {
		t.Fatal("approved inspection failed", err)
	}
	a.Parents[0].RerunSHA256 = ""
	if _, err := lineageStored(t.Context(), a.scope(), a.Parents[0], memory); err != nil {
		t.Fatal("read-only grant rejected", err)
	}
}

func TestOperatorRerunApprovalCanonicalAndReadOnlyBoundaries(t *testing.T) {
	a, _, _, _, _ := operatorRerunFixture(t)
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeRerunApproval(raw, true); err != nil {
		t.Fatal(err)
	}
	writeFingerprint := a.Parents[0].RerunSHA256
	for _, bad := range [][]byte{append([]byte(`{"unknown":1,`), raw[1:]...), append([]byte(`{"parents":[],`), raw[1:]...), bytes.Replace(raw, []byte(`"position"`), []byte(`"Position"`), 1), append(append([]byte(nil), raw...), 0xff)} {
		if _, err := decodeRerunApproval(bad, true); !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatal("noncanonical approval accepted", err)
		}
	}
	a.Parents[0].RerunSHA256 = ""
	raw, err = json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeRerunApproval(raw, false); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeRerunApproval(raw, true); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("read-only grant authorized write", err)
	}
	path := filepath.Join(t.TempDir(), "approval.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARGUS_PERFORMANCE_RERUN_APPROVAL_FILE", path)
	t.Setenv("ARGUS_DATABASE_URL", "private invalid database")
	if _, _, err := rerunOperation(t.Context(), requestcli.Creation{}); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("read-only creation opened database", err)
	}
	if _, err := lineageOperation(t.Context(), 99); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("unapproved position opened database", err)
	}
	a.Parents[0].RerunSHA256 = writeFingerprint
	raw, err = json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	review, err := selectionio.DecodeRequestReview(operatorFile(t, "testdata/review.json"))
	if err != nil {
		t.Fatal(err)
	}
	review.Reviewer.ID = "unapproved"
	raw, err = json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := rerunOperation(t.Context(), requestcli.Creation{Review: raw}); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("unapproved review opened database", err)
	}
}

func TestOperatorRerunRejectsMalformedPinsAndForeignParent(t *testing.T) {
	a, _, _, _, memory := operatorRerunFixture(t)
	for _, mutate := range []func(*rerunApproval){
		func(a *rerunApproval) { a.Parents[0].Position = 0 },
		func(a *rerunApproval) { a.Parents[0].Ordinal = 1 },
		func(a *rerunApproval) { a.Parents[0].ReviewSHA256 = "bad" },
		func(a *rerunApproval) { a.Parents = append(a.Parents, a.Parents[0]) },
		func(a *rerunApproval) { a.Request.Expected.Issuer = "relative" },
	} {
		candidate := a
		candidate.Parents = append([]rerunParentPin(nil), a.Parents...)
		mutate(&candidate)
		raw, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeRerunApproval(raw, true); !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatal("invalid pin accepted", err)
		}
	}
	original, err := perfeng.DecodeIntegration(memory.parent.Pending.Child.Payload)
	if err != nil {
		t.Fatal(err)
	}
	original.Data.Context.Principal.ID = "other-principal"
	memory.parent.Pending.Child.Payload, err = perfeng.EncodeIntegration(original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createApprovedRerun(t.Context(), memory.successor, a, memory); !errors.Is(err, requestintent.ErrInvalid) || memory.writes != 0 {
		t.Fatal("foreign parent wrote", err)
	}
}

type lineageShortWriter struct{}

func (lineageShortWriter) Write(raw []byte) (int, error) { return len(raw) - 1, nil }

func TestOperatorLineageTransportValidationAndOutputFailures(t *testing.T) {
	_, _, _, _, memory := operatorRerunFixture(t)
	op := func(context.Context, int64) (requestintent.RerunRecord, error) { return memory.record, nil }
	var output bytes.Buffer
	if err := requestcli.RunLineage(t.Context(), []string{"-position", "7"}, &output, op); err != nil || !strings.Contains(output.String(), memory.record.Evidence.Fingerprint()) {
		t.Fatal("lineage output missing", err)
	}
	if err := requestcli.RunLineage(t.Context(), []string{"-position", "7"}, lineageShortWriter{}, op); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short output accepted", err)
	}
	output.Reset()
	if err := requestcli.RunLineage(t.Context(), []string{"-position", "7"}, &output, func(context.Context, int64) (requestintent.RerunRecord, error) {
		return requestintent.RerunRecord{}, errors.New("private provider details")
	}); !errors.Is(err, requestintent.ErrUnavailable) || output.Len() != 0 {
		t.Fatal("private lineage error escaped", err)
	}
	for _, args := range [][]string{nil, {"-position", "0"}, {"-position", "7", "-issuer", "untrusted"}} {
		if err := requestcli.RunLineage(t.Context(), args, &output, func(context.Context, int64) (requestintent.RerunRecord, error) {
			t.Fatal("invalid flags reached operation")
			return requestintent.RerunRecord{}, nil
		}); err == nil {
			t.Fatal("invalid arguments accepted")
		}
	}
}

func TestOperatorRerunModeRoutingRejectsInvalidInputsBeforeEffects(t *testing.T) {
	for _, args := range [][]string{nil, {"-create"}, {"-cancel"}} {
		if handled, err := runRerunMode(t.Context(), args, io.Discard); handled || err != nil {
			t.Fatal("legacy mode intercepted", err)
		}
	}
	for _, args := range [][]string{{"-rerun"}, {"-lineage", "-position", "0"}} {
		if handled, err := runRerunMode(t.Context(), args, io.Discard); !handled || err == nil {
			t.Fatal("new mode failed to validate", err)
		}
	}
}
