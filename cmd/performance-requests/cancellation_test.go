package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/requestcli"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type cancellationMemory struct {
	pending requestintent.Pending
	intent  requestintent.Cancellation
	scope   requestintent.Scope
	calls   int
	failure error
}

func (s *cancellationMemory) PendingRequests(_ context.Context, scope requestintent.Scope, after int64, limit int) ([]requestintent.Pending, error) {
	if scope != s.scope || after != 41 || limit != 1 {
		return nil, requestintent.ErrInvalid
	}
	return []requestintent.Pending{s.pending}, nil
}
func (s *cancellationMemory) RequestCancellation(_ context.Context, _ requestintent.Scope, _ int64, intent requestintent.Cancellation, _ requestintent.CancellationVerifier) (bool, error) {
	s.calls++
	created := s.intent.Fingerprint() == ""
	s.intent = intent
	return created, s.failure
}
func (s *cancellationMemory) GetCancellation(context.Context, requestintent.Scope, int64, requestintent.CancellationVerifier) (requestintent.Cancellation, error) {
	return s.intent, nil
}

func cancellationFixture(t *testing.T) (requestcli.CancellationCommand, cancellationApproval, *cancellationMemory) {
	t.Helper()
	var messages []json.RawMessage
	if err := json.Unmarshal(operatorFile(t, "../../internal/performance/adapters/perfeng/snapshot/integration/cancelled.json"), &messages); err != nil {
		t.Fatal(err)
	}
	original, err := perfeng.DecodeIntegration(messages[0])
	if err != nil {
		t.Fatal(err)
	}
	intent, err := perfeng.BindCancellation(messages[0], messages[2])
	if err != nil {
		t.Fatal(err)
	}
	c := original.Data.Context
	requested, err := time.Parse(time.RFC3339Nano, c.RequestedAt)
	if err != nil {
		t.Fatal(err)
	}
	deadline, err := time.Parse(time.RFC3339Nano, c.Deadline)
	if err != nil {
		t.Fatal(err)
	}
	a := cancellationApproval{Scope: requestintent.Scope{Issuer: "https://provider.example/identity", PrincipalIssuer: c.Principal.Issuer, PrincipalID: c.Principal.ID}, Position: 42, RequestSHA256: cancellationHash(messages[0]), CancellationSHA256: cancellationHash(intent.Data().Payload)}
	child := requestintent.Child{Request: performance.Identity(c.Request), Attempt: performance.Identity(c.Attempt), EventID: original.ID, SHA256: a.RequestSHA256, Payload: messages[0], RequestedAt: requested, Deadline: deadline, Reasons: []requestintent.Reason{{MappingID: "mapping", Capability: "load", Reason: "reviewed"}}}

	return requestcli.CancellationCommand{Position: 42, Record: true, Payload: messages[2]}, a, &cancellationMemory{scope: a.Scope, pending: requestintent.Pending{After: 42, Child: child}}
}

func TestCancellationOperatorPreflightDeniesUnapprovedInputBeforeStorage(t *testing.T) {
	for _, change := range []string{"read-only", "position", "digest", "unknown", "duplicate", "alias", "bad-uri", "kind"} {
		command, a, _ := cancellationFixture(t)
		switch change {
		case "read-only":
			a.CancellationSHA256 = ""
		case "position":
			a.Position++
		case "digest":
			a.CancellationSHA256 = strings.Repeat("a", 64)
		case "bad-uri":
			a.Scope.Issuer = "relative"
		case "kind":
			command.Payload = []byte("{}")
		}
		raw, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		switch change {
		case "unknown":
			raw = append(raw[:len(raw)-1], []byte(`,"unknown":true}`)...)
		case "duplicate":
			raw = append(raw[:len(raw)-1], []byte(`,"position":42}`)...)
		case "alias":
			raw = []byte(strings.Replace(string(raw), `"position"`, `"Position"`, 1))
		}
		file := filepath.Join(t.TempDir(), "approval.json")
		if err := os.WriteFile(file, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("ARGUS_PERFORMANCE_CANCELLATION_APPROVAL_FILE", file)
		t.Setenv("ARGUS_DATABASE_URL", "invalid-must-not-open")
		if _, err := cancellationOperation(t.Context(), command); !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatal("unapproved input reached storage", change, err)
		}
	}
}

func TestCancellationOperatorRetainsReplayAndInspectsReadOnlyWithExactScope(t *testing.T) {
	command, a, store := cancellationFixture(t)
	for i := 0; i < 2; i++ {
		result, err := cancellationStored(t.Context(), command, a, store)
		if err != nil || result.Created != (i == 0) || result.Scope != a.Scope {
			t.Fatal("record/replay", err)
		}
	}
	command.Record, command.Payload = false, nil
	a.CancellationSHA256 = ""
	result, err := cancellationStored(t.Context(), command, a, store)
	if err != nil || result.Created || result.Intent.Fingerprint() != store.intent.Fingerprint() || store.calls != 2 {
		t.Fatal("inspection changed intent", err)
	}
	store.pending.After++
	if _, err := cancellationStored(t.Context(), command, a, store); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("different position accepted", err)
	}
	store.pending.After--
	a.RequestSHA256 = strings.Repeat("a", 64)
	if _, err := cancellationStored(t.Context(), command, a, store); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("changed frozen bytes accepted", err)
	}
}

func TestCancellationOperatorUncertainCommitRequiresExplicitInspection(t *testing.T) {
	command, a, store := cancellationFixture(t)
	store.failure = requestintent.ErrUnavailable
	if _, err := cancellationStored(t.Context(), command, a, store); !errors.Is(err, requestintent.ErrUnavailable) || store.calls != 1 {
		t.Fatal("uncertain commit retried", err)
	}
	command.Record, command.Payload = false, nil
	result, err := cancellationStored(t.Context(), command, a, store)
	if err != nil || result.Intent.Fingerprint() != store.intent.Fingerprint() || store.calls != 1 {
		t.Fatal("inspection lost committed intent", err)
	}
}

func TestCancellationOperatorRejectsApprovedButUncorrelatedWireAndCorruptReads(t *testing.T) {
	command, a, store := cancellationFixture(t)
	m, err := perfeng.DecodeIntegration(command.Payload)
	if err != nil {
		t.Fatal(err)
	}
	m.Data.Context.Attempt.ID = "another-attempt"
	command.Payload, err = perfeng.EncodeIntegration(m)
	if err != nil {
		t.Fatal(err)
	}
	a.CancellationSHA256 = cancellationHash(command.Payload)
	if _, err := cancellationStored(t.Context(), command, a, store); !errors.Is(err, requestintent.ErrInvalid) || store.calls != 0 {
		t.Fatal("approved wrong context wrote intent", err)
	}
	command, a, store = cancellationFixture(t)
	intent, err := perfeng.BindCancellation(store.pending.Child.Payload, command.Payload)
	if err != nil {
		t.Fatal(err)
	}
	d := intent.Data()
	d.EventID = "forged-metadata"
	store.intent, err = requestintent.NewCancellation(d)
	if err != nil {
		t.Fatal(err)
	}
	command.Record, command.Payload = false, nil
	a.CancellationSHA256 = ""
	if result, err := cancellationStored(t.Context(), command, a, store); !errors.Is(err, requestintent.ErrUnavailable) || result.Intent.Fingerprint() != "" {
		t.Fatal("corrupt read returned evidence", err)
	}
}
