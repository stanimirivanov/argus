package perfeng

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// NativePolicySchemaURI identifies the pinned native policy, never a fetch URL.
const NativePolicySchemaURI = "https://perfeng.dev/schemas/policy/v1/performance-policy.schema.json"

// NativePolicyMaximumBytes bounds one policy independently of artifact metadata.
const NativePolicyMaximumBytes = 1 << 20

// ErrNativePolicy rejects invalid policy claims without reflecting their content.
var ErrNativePolicy = errors.New("invalid native performance policy")

// ErrNativePolicyVersion additionally classifies unsupported policy generations.
var ErrNativePolicyVersion = errors.New("unsupported native performance policy version")

// NativePolicy contains native policy claims, not approval or request association.
// Hash original bytes before trusting an immutable pin; never hash re-encoded data.
type NativePolicy struct {
	APIVersion string               `json:"apiVersion"`
	Kind       string               `json:"kind"`
	Metadata   NativePolicyMetadata `json:"metadata"`
	Spec       NativePolicySpec     `json:"spec"`
}

// NativePolicyMetadata preserves the native policy identity and owning team.
type NativePolicyMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Owner   string `json:"owner"`
}

// NativePolicySpec preserves nonblocking mode and the complete ordered rule set.
type NativePolicySpec struct {
	Mode        string             `json:"mode"`
	MissingData string             `json:"missingData"`
	Rules       []NativePolicyRule `json:"rules"`
}

// NativePolicyRule preserves one unique ID/selector and explicitly configured sections.
type NativePolicyRule struct {
	ID         string                  `json:"id"`
	Metric     AnalysisMetric          `json:"metric"`
	Quality    *NativePolicyQuality    `json:"quality,omitempty"`
	SLO        *NativePolicySLO        `json:"slo,omitempty"`
	Regression *NativePolicyRegression `json:"regression,omitempty"`
}

// NativePolicyQuality preserves exact optional evidence thresholds, including zero CV.
type NativePolicyQuality struct {
	MinSamples *json.Number `json:"minSamples,omitempty"`
	MaxCV      *json.Number `json:"maxCv,omitempty"`
}

// NativePolicySLO preserves inclusive exact bounds; omitted bounds remain absent.
type NativePolicySLO struct {
	Min *json.Number `json:"min,omitempty"`
	Max *json.Number `json:"max,omitempty"`
}

// NativePolicyRegression preserves direction, practical difference and baseline pin.
type NativePolicyRegression struct {
	Direction           string                 `json:"direction"`
	PracticalDifference NativePolicyDifference `json:"practicalDifference"`
	Reference           NativePolicyReference  `json:"reference"`
}

// NativePolicyDifference retains exact relative/absolute threshold syntax.
type NativePolicyDifference struct {
	Kind  string      `json:"kind"`
	Value json.Number `json:"value"`
}

// NativePolicyReference identifies a requested baseline, never its approval state.
type NativePolicyReference struct {
	BaselineID string `json:"baselineId"`
	Version    string `json:"version"`
}

//go:embed snapshot/policy/schema.json
var nativePolicySchemaBytes []byte

var nativePolicySchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(nativePolicySchemaBytes))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(jsonschema.SchemeURLLoader{})
	compiler.AssertFormat()
	if err := compiler.AddResource(NativePolicySchemaURI, document); err != nil {
		return nil, err
	}

	return compiler.Compile(NativePolicySchemaURI)
})

// DecodeNativePolicy validates bounded owned claims without I/O, clocks, approval
// lookup or outcome recomputation. Schema and ambiguity/bound checks return zero
// output on failure. Calls are concurrent-safe; exact numeric tokens are preserved.
func DecodeNativePolicy(raw []byte) (NativePolicy, error) {
	if len(raw) > NativePolicyMaximumBytes || !validUnicode(raw) {
		return NativePolicy{}, ErrNativePolicy
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	document, err := decodeValue(decoder, 0)
	if err != nil || !boundedAnalysisNumbers(document) {
		return NativePolicy{}, ErrNativePolicy
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return NativePolicy{}, ErrNativePolicy
	}
	if header, ok := document.(map[string]any); ok {
		if version, ok := header["apiVersion"].(string); ok && version != "" && version != "performance.perfeng.io/v1" {
			return NativePolicy{}, errors.Join(ErrNativePolicy, ErrNativePolicyVersion)
		}
	}
	schema, err := nativePolicySchema()
	if err != nil || schema.Validate(document) != nil {
		return NativePolicy{}, ErrNativePolicy
	}
	var policy NativePolicy
	if json.Unmarshal(raw, &policy) != nil || !consistentNativePolicy(policy) {
		return NativePolicy{}, ErrNativePolicy
	}

	return policy, nil
}

func consistentNativePolicy(policy NativePolicy) bool {
	ids, selectors := map[string]bool{}, map[[2]string]bool{}
	for _, rule := range policy.Spec.Rules {
		selector := [2]string{rule.Metric.Name, rule.Metric.Statistic}
		if ids[rule.ID] || selectors[selector] {
			return false
		}
		ids[rule.ID], selectors[selector] = true, true
		if rule.SLO != nil && rule.SLO.Min != nil && rule.SLO.Max != nil {
			min, minOK := new(big.Rat).SetString(rule.SLO.Min.String())
			max, maxOK := new(big.Rat).SetString(rule.SLO.Max.String())
			if !minOK || !maxOK || min.Cmp(max) > 0 {
				return false
			}
		}
	}

	return true
}
