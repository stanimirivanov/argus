package perfeng

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type publicationAdmissionFixtureData struct {
	frozen, wire []byte
	retained     requestintent.Outcome
	provider     string
	principal    performance.Identity
}

func publicationAdmissionFixture(t *testing.T) publicationAdmissionFixtureData {
	t.Helper()
	exchange := integrationExchange(t, "completed")
	accepted := lifecycleReply(t, exchange[1])
	retained, err := DispatchOutcome(exchange[0], accepted, accepted.Source)
	if err != nil {
		t.Fatal(err)
	}
	message, err := DecodePublication(publicationFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	message.Source, message.Data.Context, message.Data.Run = accepted.Source, accepted.Data.Context, *accepted.Data.Run
	message.Time = "2026-10-22T10:00:01.5Z"
	if message.Data.Context.Baseline == nil {
		message.Data.Native.Baseline = nil
	}
	raw, err := EncodePublication(message)
	if err != nil {
		t.Fatal(err)
	}

	return publicationAdmissionFixtureData{exchange[0], raw, retained, accepted.Source, performance.Identity(accepted.Data.Context.Principal)}
}

func bindPublicationFixture(f publicationAdmissionFixtureData) (PublicationAdmission, error) {
	return BindPublicationAdmission(f.frozen, f.retained, f.wire, f.provider, f.principal)
}

func TestPublicationAdmissionOwnsExactBytesAndRetainedPins(t *testing.T) {
	f := publicationAdmissionFixture(t)
	f.wire = append(append([]byte(" \n"), f.wire...), '\n')
	expected := bytes.Clone(f.wire)
	bound, err := bindPublicationFixture(f)
	if err != nil {
		t.Fatal(err)
	}
	data := bound.Data()
	digest, requestDigest := sha256.Sum256(expected), sha256.Sum256(f.frozen)
	if !bytes.Equal(data.Payload, expected) || data.SHA256 != hex.EncodeToString(digest[:]) || data.Admission.Data().RequestSHA256 != hex.EncodeToString(requestDigest[:]) || data.Admission.Data().Evidence.Fingerprint() != f.retained.Fingerprint() {
		t.Fatal("binding normalized bytes or lost authoritative pins")
	}
	data.Payload[0], f.wire[0] = '!', '!'
	if !bytes.Equal(bound.Data().Payload, expected) {
		t.Fatal("bound payload aliases caller/returned bytes")
	}
	var wait sync.WaitGroup
	for range 4 {
		wait.Go(func() {
			copy := bound.Data()
			copy.Payload[0] = '!'
			if !bytes.Equal(bound.Data().Payload, expected) {
				t.Error("concurrent access shared mutable evidence")
			}
		})
	}
	wait.Wait()
}

func TestPublicationAdmissionRejectsScopeContextAndRunSubstitution(t *testing.T) {
	for _, mode := range []string{"provider", "principal", "principal-issuer", "run", "request", "attempt", "profile", "policy", "baseline", "evidence", "candidate", "context-time"} {
		t.Run(mode, func(t *testing.T) {
			f := publicationAdmissionFixture(t)
			message, err := DecodePublication(f.wire)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "provider":
				f.provider += "/other"
			case "principal":
				f.principal.ID += "-other"
			case "principal-issuer":
				f.principal.Issuer += "/other"
			case "run":
				message.Data.Run.ID += "-other"
			case "request":
				message.Data.Context.Request.ID += "-other"
			case "attempt":
				message.Data.Context.Attempt.ID += "-other"
			case "profile":
				message.Data.Context.Profile.Revision += "-other"
			case "policy":
				message.Data.Context.Policy.SHA256 = strings.Repeat("f", 64)
			case "baseline":
				message.Data.Context.Baseline.Revision += "-other"
			case "evidence":
				message.Data.Context.Evidence[0].Revision += "-other"
			case "candidate":
				message.Data.Context.Candidate.ImageDigest = "sha256:" + strings.Repeat("f", 64)
			case "context-time":
				message.Data.Context.RequestedAt = "2026-10-22T10:00:00+00:00"
			}
			f.wire, err = EncodePublication(message)
			if err != nil {
				t.Fatal("mutation must remain codec-valid", err)
			}
			assertPublicationAdmissionFailure(t, f)
		})
	}
}

func assertPublicationAdmissionFailure(t *testing.T, f publicationAdmissionFixtureData) {
	t.Helper()
	got, err := bindPublicationFixture(f)
	if !errors.Is(err, ErrPublication) || !reflect.DeepEqual(got, PublicationAdmission{}) || got.Data().SHA256 != "" || strings.Contains(err.Error(), "private") {
		t.Fatal("invalid correlation returned partial evidence", err)
	}
}

func TestPublicationAdmissionAcceptanceOrderingAndLifecycleRecovery(t *testing.T) {
	f := publicationAdmissionFixture(t)
	message, err := DecodePublication(f.wire)
	if err != nil {
		t.Fatal(err)
	}
	message.Time = "2026-10-22T10:00:00.5Z"
	f.wire, err = EncodePublication(message)
	if err != nil {
		t.Fatal(err)
	}
	assertPublicationAdmissionFailure(t, f)
	message.Time = "2026-10-23T10:00:00Z"
	f.wire, err = EncodePublication(message)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bindPublicationFixture(f); err != nil {
		t.Fatal("late analysis discarded", err)
	}
	exchange := integrationExchange(t, "completed")
	for _, raw := range exchange[2:] {
		f = publicationAdmissionFixture(t) // Publication precedes the later recovery snapshot.
		reply := lifecycleReply(t, raw)
		f.retained, err = DispatchOutcome(f.frozen, reply, f.provider)
		if err != nil {
			t.Fatal(err)
		}
		bound, err := bindPublicationFixture(f)
		if err != nil || bound.Data().Admission.Data().Initial.Data().State == "" {
			t.Fatal("lifecycle time was invented as admission time", err)
		}
	}
}

func TestPublicationAdmissionRejectsUnsettledRejectedAndMalformedEvidence(t *testing.T) {
	for _, mode := range []string{"zero", "forged", "rejected", "frozen", "publication", "large-frozen", "large-publication"} {
		f := publicationAdmissionFixture(t)
		switch mode {
		case "zero":
			f.retained = requestintent.Outcome{}
		case "forged":
			f.retained = changedAdmission(t, f.retained)
		case "rejected":
			reply := lifecycleReply(t, integrationExchange(t, "rejected")[1])
			original, err := DecodeIntegration(f.frozen)
			if err != nil {
				t.Fatal(err)
			}
			reply.Data.Context, reply.Source = original.Data.Context, f.provider
			reply.Time = "2026-10-22T10:00:01Z"
			f.retained, err = DispatchOutcome(f.frozen, reply, f.provider)
			if err != nil {
				t.Fatal(err)
			}
		case "frozen":
			f.frozen = []byte(`{}`)
		case "publication":
			f.wire = []byte(`{"secret":"private"}`)
		case "large-frozen":
			f.frozen = bytes.Repeat([]byte(" "), IntegrationMaximumBytes+1)
		case "large-publication":
			f.wire = bytes.Repeat([]byte(" "), PublicationMaximumBytes+1)
		}
		assertPublicationAdmissionFailure(t, f)
	}
}
