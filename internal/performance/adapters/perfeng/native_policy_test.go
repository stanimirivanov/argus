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

func nativePolicyFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("snapshot/policy/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

func TestNativePolicyPinsPublishedDocuments(t *testing.T) {
	for name, expected := range map[string]string{
		"schema":   "76935f55c9aef751e0b51b9c9bcb8210e297eea35e022b35d96d258d9a7ce328",
		"browser":  "566a9d13e277db0b492c4a7b55294a2e479c9d449768317c072f0f4dda988646",
		"checkout": "849fdf9a06eadb1aa296b2cac93b3444db98b0a2539e3d5e5c7617121413feb0",
	} {
		raw := nativePolicyFixture(t, name)
		digest := sha256.Sum256(raw)
		if hex.EncodeToString(digest[:]) != expected {
			t.Fatal("upstream policy bytes changed", name)
		}
		if name != "schema" {
			policy, err := DecodeNativePolicy(raw)
			if err != nil || policy.Metadata.Name == "" || len(policy.Spec.Rules) != 1 || policy.Spec.Rules[0].Regression.Reference.Version != "1.0.0" {
				t.Fatal("published policy rejected or reference lost", name, err)
			}
		}
	}
}

func TestNativePolicyPreservesExactThresholdsAndOptionalSections(t *testing.T) {
	base, err := DecodeNativePolicy(nativePolicyFixture(t, "browser"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"slo-only", "regression-only", "exact-bounds"} {
		policy := base
		policy.Spec.Rules = append([]NativePolicyRule(nil), base.Spec.Rules...)
		rule := &policy.Spec.Rules[0]
		zero, count, bound := json.Number("0.00"), json.Number("1e0"), json.Number("1.0000000000000000000000000001")
		rule.Quality = &NativePolicyQuality{MinSamples: &count, MaxCV: &zero}
		switch mode {
		case "slo-only":
			rule.Regression, rule.Quality = nil, nil
		case "regression-only":
			rule.SLO = nil
		case "exact-bounds":
			rule.SLO = &NativePolicySLO{Min: &bound, Max: &bound}
		}
		raw, err := json.Marshal(policy)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeNativePolicy(raw)
		if err != nil || !reflect.DeepEqual(got, policy) {
			t.Fatal("numeric tokens or omitted sections changed", mode, err)
		}
		got.Spec.Rules[0].Metric.Name = "mutated"
		if policy.Spec.Rules[0].Metric.Name == "mutated" {
			t.Fatal("policy rules alias caller data")
		}
		if got.Spec.Rules[0].Quality != nil {
			*got.Spec.Rules[0].Quality.MaxCV = "999"
			if rule.Quality.MaxCV.String() != "0.00" {
				t.Fatal("threshold pointer aliases")
			}
		}
		raw[0] = '!'
		if got.Metadata.Name == "" {
			t.Fatal("decoded metadata aliases input")
		}
	}
}

func TestNativePolicyRejectsAmbiguousOrContradictoryRules(t *testing.T) {
	for _, mode := range []string{"id", "selector", "range", "empty-quality", "empty-slo", "no-evaluation", "reference", "difference", "empty-rules"} {
		policy, err := DecodeNativePolicy(nativePolicyFixture(t, "browser"))
		if err != nil {
			t.Fatal(err)
		}
		rule := &policy.Spec.Rules[0]
		switch mode {
		case "id":
			duplicate := *rule
			duplicate.Metric.Name += ".other"
			policy.Spec.Rules = append(policy.Spec.Rules, duplicate)
		case "selector":
			duplicate := *rule
			duplicate.ID += "-other"
			duplicate.Metric.Unit = "other"
			policy.Spec.Rules = append(policy.Spec.Rules, duplicate)
		case "range":
			min, max := json.Number("1.0000000000000000000000000002"), json.Number("1.0000000000000000000000000001")
			rule.SLO = &NativePolicySLO{Min: &min, Max: &max}
		case "empty-quality":
			rule.Quality = &NativePolicyQuality{}
		case "empty-slo":
			rule.SLO = &NativePolicySLO{}
		case "no-evaluation":
			rule.SLO, rule.Regression = nil, nil
		case "reference":
			rule.Regression.Reference.Version = "latest"
		case "difference":
			rule.Regression.PracticalDifference.Value = "0"
		case "empty-rules":
			policy.Spec.Rules = []NativePolicyRule{}
		}
		raw, err := json.Marshal(policy)
		if err != nil {
			t.Fatal(err)
		}
		assertInvalidNativePolicy(t, raw, ErrNativePolicy)
	}
}

func assertInvalidNativePolicy(t *testing.T, raw []byte, expected error) {
	t.Helper()
	got, err := DecodeNativePolicy(raw)
	if !errors.Is(err, expected) || !reflect.DeepEqual(got, NativePolicy{}) || strings.Contains(err.Error(), "private") {
		t.Fatal("unsafe/partial native policy failure", err)
	}
}

func TestNativePolicyRejectsMalformedAndUnboundedJSON(t *testing.T) {
	base := nativePolicyFixture(t, "browser")
	for _, raw := range [][]byte{
		bytes.Replace(base, []byte(`"spec": {`), []byte(`"spec": {"blocking":true,`), 1),
		bytes.Replace(base, []byte(`"kind": "PerformancePolicy",`), []byte(`"kind":"PerformancePolicy","kind":"PerformancePolicy",`), 1),
		bytes.Replace(base, []byte(`"search-browser"`), []byte(`"\ud800"`), 1),
		bytes.Replace(base, []byte(`"max": 200`), []byte(`"max": 1e10000000`), 1),
		bytes.Replace(base, []byte(`"max": 200`), []byte(`"max": 1.`+strings.Repeat("0", 130)), 1),
		append(bytes.Clone(base), []byte(` {"private":true}`)...),
		[]byte(strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34)),
		bytes.Repeat([]byte(" "), NativePolicyMaximumBytes+1),
	} {
		assertInvalidNativePolicy(t, raw, ErrNativePolicy)
	}
	assertInvalidNativePolicy(t, bytes.Replace(base, []byte("performance.perfeng.io/v1"), []byte("performance.perfeng.io/v2"), 1), ErrNativePolicyVersion)
}

func TestNativePolicyConcurrentSchemaReuse(t *testing.T) {
	raw := nativePolicyFixture(t, "browser")
	var wait sync.WaitGroup
	for range 4 {
		wait.Go(func() {
			for range 3 {
				if _, err := DecodeNativePolicy(raw); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wait.Wait()
}
