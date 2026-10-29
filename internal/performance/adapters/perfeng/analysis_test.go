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

func analysisFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("snapshot/analysis/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

func TestAnalysisArtifactsRemainPinned(t *testing.T) {
	for name, want := range map[string]string{
		"schema":                   "04d281d709497c511d69fb145a8fe1d3a9f10e0ef914d50c9ee5da596548eb93",
		"missing-reference":        "2f2a1cd69b32415eee6d3467d2c6d474e6bca583a41b3439fb278b63e47942b9",
		"slo-pass-regression-fail": "ddea48923c2eba4c9cf1ba90c067702946094c8cc99a04ebccf5640a49993029",
		"unknown-samples":          "0291736f98e7ccc135f5267e6171eb8ff3f6b1c3022cb746b521be6146c7b4f3",
	} {
		raw := analysisFixture(t, name)
		digest := sha256.Sum256(raw)
		if hex.EncodeToString(digest[:]) != want {
			t.Fatal("published bytes changed", name)
		}
	}
}

func TestAnalysisPreservesIndependentOutcomesAndUnknownMeasurements(t *testing.T) {
	for _, test := range []struct{ name, quality, slo, regression string }{
		{"slo-pass-regression-fail", "PASS", "PASS", "FAIL"},
		{"missing-reference", "PASS", "PASS", "INCONCLUSIVE"},
		{"unknown-samples", "INCONCLUSIVE", "INCONCLUSIVE", "INCONCLUSIVE"},
	} {
		raw := analysisFixture(t, test.name)
		result, err := DecodeAnalysis(raw)
		if err != nil {
			t.Fatal(test.name, err)
		}
		e := result.Evaluations[0]
		if e.Quality.Status != test.quality || e.SLO.Status != test.slo || e.Regression.Status != test.regression || result.Blocking {
			t.Fatal("outcomes conflated")
		}
		if test.name == "unknown-samples" && e.Quality.Samples != nil {
			t.Fatal("missing count became zero")
		}
		raw[0] = '!'
		if result.RunID == "" || e.Metric.Name == "" {
			t.Fatal("input aliases result")
		}
	}
	raw := bytes.Replace(analysisFixture(t, "slo-pass-regression-fail"), []byte(`"value": 140`), []byte(`"value": 1.4000000000000000000000000001e2`), 1)
	result, err := DecodeAnalysis(raw)
	if err != nil || result.Evaluations[0].SLO.Value.String() != "1.4000000000000000000000000001e2" {
		t.Fatal("numeric evidence rounded", err)
	}
	raw = bytes.Replace(raw, []byte(`"schemaVersion": 1`), []byte(`"schemaVersion": 1e0`), 1)
	if _, err := DecodeAnalysis(raw); err != nil {
		t.Fatal("integer-valued version syntax rejected", err)
	}
}

func requireAnalysisInvalid(t *testing.T, raw []byte) {
	t.Helper()
	got, err := DecodeAnalysis(raw)
	if !errors.Is(err, ErrAnalysis) || !reflect.DeepEqual(got, AnalysisResult{}) {
		t.Fatal("invalid artifact returned partial claims", err)
	}
}

func TestAnalysisRejectsAmbiguousMalformedAndUnsupportedDocuments(t *testing.T) {
	raw := analysisFixture(t, "slo-pass-regression-fail")
	for _, bad := range [][]byte{
		append([]byte(`{"schemaVersion":1,`), raw[1:]...),
		append([]byte(`{"unknown":"private diagnostic",`), raw[1:]...),
		append(append([]byte(nil), raw...), []byte(`{}`)...),
		bytes.Replace(raw, []byte("search-browser"), []byte(`\ud800`), 1),
		bytes.Replace(raw, []byte(`"blocking": false`), []byte(`"blocking": true`), 1),
		bytes.Repeat([]byte(" "), AnalysisMaximumBytes+1),
		bytes.Replace(raw, []byte(`"value": 140`), []byte(`"value": 1e999999999`), 1),
		bytes.Replace(raw, []byte(`"value": 140`), []byte(`"value": 1e-999999999`), 1),
		bytes.Replace(raw, []byte(`"value": 140`), []byte(`"value": `+strings.Repeat("1", 129)), 1),
		[]byte(strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34)),
	} {
		requireAnalysisInvalid(t, bad)
	}
	unsupported := bytes.Replace(raw, []byte(`"schemaVersion": 1`), []byte(`"schemaVersion": 2`), 1)
	if _, err := DecodeAnalysis(unsupported); !errors.Is(err, ErrAnalysisVersion) {
		t.Fatal("unsupported generation not classified", err)
	}
	requireAnalysisInvalid(t, bytes.Replace(raw, []byte(`"schemaVersion": 1`), []byte(`"schemaVersion": 1.000000000000000000000000001`), 1))
}

func TestAnalysisDecodedValuesDoNotShareMutableEvidence(t *testing.T) {
	raw := analysisFixture(t, "unknown-samples")
	one, err := DecodeAnalysis(raw)
	if err != nil {
		t.Fatal(err)
	}
	two, err := DecodeAnalysis(raw)
	if err != nil {
		t.Fatal(err)
	}
	one.Evaluations[0].Quality.Reasons[0] = "changed"
	*one.Evaluations[0].SLO.Value = json.Number("999")
	if two.Evaluations[0].Quality.Reasons[0] == "changed" || two.Evaluations[0].SLO.Value.String() == "999" {
		t.Fatal("decoded documents share evidence")
	}
}

func TestAnalysisEnforcesPublishedCrossFieldEvidenceRules(t *testing.T) {
	for _, mutate := range []func(*AnalysisResult){
		func(r *AnalysisResult) { r.CandidateArtifact.RunID = "perf-20260901-130000-a1b2c3d5" },
		func(r *AnalysisResult) { r.ReferenceArtifacts = append(r.ReferenceArtifacts, r.CandidateArtifact) },
		func(r *AnalysisResult) {
			e := r.Evaluations[0]
			e.Metric.Statistic = "median"
			r.Evaluations = append(r.Evaluations, e)
		},
		func(r *AnalysisResult) {
			r.Evaluations[0].Quality.Status = "INVALID"
			r.Evaluations[0].Quality.Reasons = []string{"bad measurements"}
		},
		func(r *AnalysisResult) {
			r.Evaluations[0].SLO.Status = "FAIL"
			r.Evaluations[0].SLO.Reasons = []string{}
		},
		func(r *AnalysisResult) { r.Evaluations[0].Regression.Reasons = []string{} },
		func(r *AnalysisResult) {
			id := "77777777-7777-4777-8777-777777777777"
			r.Evaluations[0].Regression.ReferenceArtifactID = &id
		},
		func(r *AnalysisResult) { r.Evaluations[0].Regression.ReferenceArtifactID = &r.CandidateArtifact.ID },
	} {
		result, err := DecodeAnalysis(analysisFixture(t, "slo-pass-regression-fail"))
		if err != nil {
			t.Fatal(err)
		}
		mutate(&result)
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		requireAnalysisInvalid(t, raw)
	}
}

func TestAnalysisNonPassQualityStaysNonDecisiveAndConcurrent(t *testing.T) {
	for _, quality := range []string{"INVALID", "UNSTABLE", "INCONCLUSIVE", "NOT_EVALUATED"} {
		result, err := DecodeAnalysis(analysisFixture(t, "unknown-samples"))
		if err != nil {
			t.Fatal(err)
		}
		result.Evaluations[0].Quality.Status = quality
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var wait sync.WaitGroup
		for range 4 {
			wait.Go(func() {
				got, err := DecodeAnalysis(raw)
				if err != nil || got.Evaluations[0].Quality.Status != quality {
					t.Error("quality or concurrent schema changed", err)
				}
			})
		}
		wait.Wait()
	}
}
