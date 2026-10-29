package perfeng

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// AnalysisSchemaURI identifies the pinned native artifact, never a fetch target.
const AnalysisSchemaURI = "https://perfeng.dev/schemas/analysis/v1/analysis-result.schema.json"

// AnalysisMaximumBytes bounds one structured analysis artifact independently of raw results.
const AnalysisMaximumBytes = 1 << 20

// ErrAnalysis rejects invalid artifact claims without reflecting provider diagnostics.
var ErrAnalysis = errors.New("invalid performance analysis artifact")

// ErrAnalysisVersion additionally classifies an unsupported artifact generation.
var ErrAnalysisVersion = errors.New("unsupported performance analysis version")

// AnalysisResult contains native transport claims, never authenticated decision
// authority. Quality, SLO and regression stay independent; no overall PASS exists.
type AnalysisResult struct {
	SchemaVersion      json.Number          `json:"schemaVersion"`
	Kind               string               `json:"kind"`
	ContractsVersion   string               `json:"contractsVersion"`
	RunID              string               `json:"runId"`
	TestID             string               `json:"testId"`
	CreatedAt          string               `json:"createdAt"`
	Producer           AnalysisProducer     `json:"producer"`
	Policy             AnalysisPolicy       `json:"policy"`
	CandidateArtifact  AnalysisArtifact     `json:"candidateArtifact"`
	ReferenceArtifacts []AnalysisArtifact   `json:"referenceArtifacts"`
	Blocking           bool                 `json:"blocking"`
	Evaluations        []AnalysisEvaluation `json:"evaluations"`
}

// AnalysisProducer preserves producer identity/version/image claims, not authentication.
type AnalysisProducer struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Image   string `json:"image"`
}

// AnalysisPolicy preserves the exact policy pin and observe/inform mode.
type AnalysisPolicy struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Mode    string `json:"mode"`
}

// AnalysisArtifact is an immutable-reference claim; decoding never fetches its URI
// or verifies referenced bytes. Numeric size syntax is retained without rounding.
type AnalysisArtifact struct {
	ID        string      `json:"id"`
	RunID     string      `json:"runId"`
	Kind      string      `json:"kind"`
	URI       string      `json:"uri"`
	SHA256    string      `json:"sha256"`
	SizeBytes json.Number `json:"sizeBytes"`
	MediaType string      `json:"mediaType"`
	Format    string      `json:"format"`
}

// AnalysisMetric preserves the publisher's selector; consumers do not recompute it.
type AnalysisMetric struct {
	Name      string `json:"name"`
	Statistic string `json:"statistic"`
	Unit      string `json:"unit"`
}

// AnalysisQuality distinguishes invalid/unstable/unknown measurements from a pass.
// Missing samples/CV stay missing; they never become fabricated zero measurements.
type AnalysisQuality struct {
	Status  string       `json:"status"`
	Reasons []string     `json:"reasons"`
	Samples *json.Number `json:"samples,omitempty"`
	CV      *json.Number `json:"cv,omitempty"`
}

// AnalysisSLO describes absolute bounds independently of regression comparison.
type AnalysisSLO struct {
	Status  string       `json:"status"`
	Reasons []string     `json:"reasons"`
	Value   *json.Number `json:"value,omitempty"`
}

// AnalysisRegression preserves comparison evidence, method and uncertainty.
type AnalysisRegression struct {
	Status              string          `json:"status"`
	Reasons             []string        `json:"reasons"`
	CandidateValue      *json.Number    `json:"candidateValue,omitempty"`
	ReferenceValue      *json.Number    `json:"referenceValue,omitempty"`
	ReferenceArtifactID *string         `json:"referenceArtifactId,omitempty"`
	Effect              *AnalysisEffect `json:"effect,omitempty"`
	Method              *AnalysisMethod `json:"method,omitempty"`
}

// AnalysisEffect retains the reported effect; Argus performs no statistical calculation.
type AnalysisEffect struct {
	Kind  string      `json:"kind"`
	Value json.Number `json:"value"`
}

// AnalysisMethod pins the producer's comparison implementation claims.
type AnalysisMethod struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AnalysisEvaluation accounts for one unique policy rule and its independent outcomes.
type AnalysisEvaluation struct {
	RuleID     string             `json:"ruleId"`
	Metric     AnalysisMetric     `json:"metric"`
	Quality    AnalysisQuality    `json:"quality"`
	SLO        AnalysisSLO        `json:"slo"`
	Regression AnalysisRegression `json:"regression"`
}

//go:embed snapshot/analysis/schema.json
var analysisSchemaBytes []byte

var analysisSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(analysisSchemaBytes))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(jsonschema.SchemeURLLoader{})
	compiler.AssertFormat()
	if err := compiler.AddResource(AnalysisSchemaURI, document); err != nil {
		return nil, err
	}

	return compiler.Compile(AnalysisSchemaURI)
})

// DecodeAnalysis validates bounded unique-key Unicode JSON, the pinned schema and
// published outcome consistency. Returns owned claims or zero output. Calls are
// concurrent-safe, perform no I/O/clock lookup and never follow evidence URIs.
// Run/workload/policy/baseline association, artifact bytes and authenticated scope
// remain caller obligations before any request-linked decision consumption.
func DecodeAnalysis(raw []byte) (AnalysisResult, error) {
	if len(raw) > AnalysisMaximumBytes || !validUnicode(raw) {
		return AnalysisResult{}, ErrAnalysis
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	document, err := decodeValue(decoder, 0)
	if err != nil || !boundedAnalysisNumbers(document) {
		return AnalysisResult{}, ErrAnalysis
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return AnalysisResult{}, ErrAnalysis
	}
	if header, ok := document.(map[string]any); ok {
		if version, ok := header["schemaVersion"].(json.Number); ok {
			number, err := version.Float64()
			if err != nil || number != 1 {
				return AnalysisResult{}, errors.Join(ErrAnalysis, ErrAnalysisVersion)
			}
		}
	}
	schema, err := analysisSchema()
	if err != nil || schema.Validate(document) != nil {
		return AnalysisResult{}, ErrAnalysis
	}
	var result AnalysisResult
	if json.Unmarshal(raw, &result) != nil || !consistentAnalysis(result) {
		return AnalysisResult{}, ErrAnalysis
	}

	return result, nil
}

// Bound rational-number expansion before the schema library performs arithmetic.
func boundedAnalysisNumbers(value any) bool {
	switch value := value.(type) {
	case json.Number:
		if len(value) > 128 {
			return false
		}
		if index := strings.IndexAny(string(value), "eE"); index >= 0 {
			exponent, err := strconv.Atoi(string(value[index+1:]))
			return err == nil && exponent >= -999 && exponent <= 999
		}
	case map[string]any:
		for _, child := range value {
			if !boundedAnalysisNumbers(child) {
				return false
			}
		}
	case []any:
		for _, child := range value {
			if !boundedAnalysisNumbers(child) {
				return false
			}
		}
	}

	return true
}

func consistentAnalysis(result AnalysisResult) bool {
	if result.CandidateArtifact.RunID != result.RunID {
		return false
	}
	artifacts := map[string]bool{result.CandidateArtifact.ID: true}
	for _, ref := range result.ReferenceArtifacts {
		if artifacts[ref.ID] {
			return false
		}
		artifacts[ref.ID] = true
	}
	seen := map[string]bool{}
	for _, e := range result.Evaluations {
		if seen[e.RuleID] || !analysisReasons(e.Quality.Status, e.Quality.Reasons) || !analysisReasons(e.SLO.Status, e.SLO.Reasons) || !analysisReasons(e.Regression.Status, e.Regression.Reasons) {
			return false
		}
		seen[e.RuleID] = true
		if e.Quality.Status != "PASS" && (analysisDecisive(e.SLO.Status) || analysisDecisive(e.Regression.Status)) {
			return false
		}
		if id := e.Regression.ReferenceArtifactID; id != nil && (*id == result.CandidateArtifact.ID || !artifacts[*id]) {
			return false
		}
	}

	return true
}

func analysisReasons(status string, reasons []string) bool {
	return status == "PASS" || len(reasons) > 0
}
func analysisDecisive(status string) bool { return status == "PASS" || status == "FAIL" }
