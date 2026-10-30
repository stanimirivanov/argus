package perfeng

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func publicationFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("snapshot/publication/published.json")
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

func TestPublicationPinsAndOwnedRoundTrip(t *testing.T) {
	for name, expected := range map[string]string{
		"schema":    "cb7cbb434c8ba1b74306fe2198c8fdb4a869fccd70eaa91405d945c4d4a363cd",
		"published": "ffe6243e0d697a486f3c72631d10505830b3e9def571f7814665b5e5244c739c",
	} {
		raw, err := os.ReadFile("snapshot/publication/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		if hex.EncodeToString(digest[:]) != expected {
			t.Fatal("upstream bytes changed", name)
		}
	}
	raw := publicationFixture(t)
	message, err := DecodePublication(raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodePublication(message)
	if err != nil {
		t.Fatal(err)
	}
	again, err := DecodePublication(encoded)
	if err != nil || !reflect.DeepEqual(again, message) || message.Data.Run.ID == message.Data.Native.RunID || message.Data.Native.TestID == message.Data.Native.Workload.ID {
		t.Fatal("native/scoped identities were flattened", err)
	}
	message.Data.Context.Baseline.ID = "mutated"
	message.Data.Native.Baseline.Identity.ID = "mutated"
	message.Data.Context.Evidence[0].Revision = "mutated"
	if again.Data.Context.Baseline.ID == "mutated" || again.Data.Native.Baseline.Identity.ID == "mutated" || again.Data.Context.Evidence[0].Revision == "mutated" {
		t.Fatal("returned nested claims alias")
	}
	raw[0] = '!'
	if again.ID == "" {
		t.Fatal("decoded claims alias input")
	}
}

func TestPublicationAllowsNullableBaselineLateDeliveryAndExactNumbers(t *testing.T) {
	message, err := DecodePublication(publicationFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	message.Data.Context.Baseline, message.Data.Native.Baseline = nil, nil
	message.Time = "2026-09-03T15:03:00+02:00"
	message.Data.Artifact.SizeBytes = json.Number("2.362e3")
	message.Data.Native.Workload.ID = "another-native-workload"
	raw, err := EncodePublication(message)
	if err != nil {
		t.Fatal("late publication or explicit absence rejected", err)
	}
	got, err := DecodePublication(raw)
	if err != nil || got.Data.Native.Baseline != nil || got.Data.Context.Baseline != nil || got.Time != message.Time || got.Data.Artifact.SizeBytes.String() != "2.362e3" {
		t.Fatal("claims normalized or baseline reconstructed", err)
	}
}

func TestPublicationRejectsMalformedClaimsWithoutPartialOutput(t *testing.T) {
	base := publicationFixture(t)
	for _, mode := range []string{"source", "run", "baseline", "context-baseline", "time", "deadline", "identity", "uri", "baseline-uri"} {
		t.Run(mode, func(t *testing.T) {
			message, err := DecodePublication(base)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "source":
				message.Data.Run.Issuer += "/other"
			case "run":
				message.Data.Artifact.RunID = "perf-20260902-130000-a1b2c3d6"
			case "baseline":
				message.Data.Native.Baseline = nil
			case "context-baseline":
				message.Data.Context.Baseline = nil
			case "time":
				message.Time = "2026-09-01T00:00:00Z"
			case "deadline":
				message.Data.Context.Deadline = message.Data.Context.RequestedAt
			case "identity":
				message.Data.Context.Attempt = message.Data.Context.Request
			case "uri":
				message.Data.Artifact.URI = "https://user:secret@example.test/report.json"
			case "baseline-uri":
				message.Data.Native.Baseline.Artifact.URI = "s3://user:secret@example.test/reference.json"
			}
			raw, err := json.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}
			assertInvalidPublication(t, raw, ErrPublication)
			if encoded, err := EncodePublication(message); err == nil || encoded != nil {
				t.Fatal("invalid outbound claims encoded")
			}
		})
	}
}

func TestPublicationRejectsAmbiguousAndUnboundedJSON(t *testing.T) {
	base := publicationFixture(t)
	for _, raw := range [][]byte{
		bytes.Replace(base, []byte(`"data": {`), []byte(`"data": {"pass":true,`), 1),
		append(bytes.Clone(base), []byte(` {}`)...),
		bytes.Replace(base, []byte(`"specversion": "1.0",`), []byte(`"specversion": "1.0", "specversion":"1.0",`), 1),
		bytes.Replace(base, []byte(`"analysis-publication-1"`), []byte(`"\ud800"`), 1),
		bytes.Replace(base, []byte(`"sizeBytes": 2362`), []byte(`"sizeBytes": 1e10000000`), 1),
		bytes.Replace(base, []byte(`"sizeBytes": 2362`), []byte(`"sizeBytes": 1.`+strings.Repeat("0", 130)), 1),
		[]byte(strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34)),
		bytes.Repeat([]byte(" "), PublicationMaximumBytes+1),
	} {
		assertInvalidPublication(t, raw, ErrPublication)
	}
}

func assertInvalidPublication(t *testing.T, raw []byte, expected error) {
	t.Helper()
	got, err := DecodePublication(raw)
	if !errors.Is(err, expected) || !reflect.DeepEqual(got, AnalysisPublication{}) || strings.Contains(err.Error(), "secret") {
		t.Fatal("unsafe/partial publication failure", err)
	}
}

func TestPublicationVersionAndClosedIntegrationFamily(t *testing.T) {
	base := publicationFixture(t)
	for _, pair := range [][2]string{{`"specversion": "1.0"`, `"specversion":"2.0"`}, {PublicationSchemaURI, strings.Replace(PublicationSchemaURI, "/v1/", "/v2/", 1)}, {"analysis-published.v1", "analysis-published.v2"}} {
		assertInvalidPublication(t, bytes.Replace(base, []byte(pair[0]), []byte(pair[1]), 1), ErrPublicationVersion)
	}
	if _, err := DecodeIntegration(base); !errors.Is(err, ErrIntegration) {
		t.Fatal("old union widened to accept publication", err)
	}
	raw, err := os.ReadFile("snapshot/integration/completed.json")
	if err != nil {
		t.Fatal(err)
	}
	var exchange []json.RawMessage
	if err := json.Unmarshal(raw, &exchange); err != nil {
		t.Fatal(err)
	}
	for _, message := range exchange {
		assertInvalidPublication(t, message, ErrPublication)
	}
}

func TestPublicationConcurrentSchemaReuse(t *testing.T) {
	raw := publicationFixture(t)
	var wait sync.WaitGroup
	for range 4 {
		wait.Go(func() {
			for range 3 {
				if _, err := DecodePublication(raw); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wait.Wait()
}
