package perfeng

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// PublicationSchemaURI selects the separate pinned family, never a fetch URL.
const PublicationSchemaURI = "https://perfeng.dev/schemas/performance-analysis/v1/performance-analysis.schema.json"

// PublicationMaximumBytes bounds metadata independently of analysis artifact bytes.
const PublicationMaximumBytes = 64 << 10

// ErrPublication rejects malformed claims without exposing provider content.
var ErrPublication = errors.New("invalid performance analysis publication")

// ErrPublicationVersion additionally classifies unsupported wire generations.
var ErrPublicationVersion = errors.New("unsupported performance analysis publication version")

// PublicationNativeIdentity preserves a native immutable pin, never catalog aliases.
type PublicationNativeIdentity struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// PublicationBaseline preserves claimed native identity and reference association.
type PublicationBaseline struct {
	Identity PublicationNativeIdentity `json:"identity"`
	Artifact AnalysisArtifact          `json:"artifact"`
}

// PublicationNativeBinding preserves explicit native Run/test/workload/policy
// associations. Baseline nil means explicit absence, never permission to infer one.
type PublicationNativeBinding struct {
	RunID    string                    `json:"runId"`
	TestID   string                    `json:"testId"`
	Workload PublicationNativeIdentity `json:"workload"`
	Policy   PublicationNativeIdentity `json:"policy"`
	Baseline *PublicationBaseline      `json:"baseline"`
}

// PublicationData separates original immutable integration context from native
// claims and the analysis reference. Neither URI nor native identity grants authority.
type PublicationData struct {
	Context  IntegrationContext       `json:"context"`
	Run      IntegrationIdentity      `json:"run"`
	Native   PublicationNativeBinding `json:"native"`
	Artifact AnalysisArtifact         `json:"artifact"`
}

// AnalysisPublication is an owned transport claim, not an IntegrationMessage or
// authenticated decision. Public scoped Run and native Run identities stay distinct.
type AnalysisPublication struct {
	SpecVersion     string          `json:"specversion"`
	ID              string          `json:"id"`
	Source          string          `json:"source"`
	Type            string          `json:"type"`
	Time            string          `json:"time"`
	DataContentType string          `json:"datacontenttype"`
	DataSchema      string          `json:"dataschema"`
	Data            PublicationData `json:"data"`
}

//go:embed snapshot/publication/schema.json
var publicationSchemaBytes []byte

var publicationSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(publicationSchemaBytes))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(jsonschema.SchemeURLLoader{})
	compiler.AssertFormat()
	if err := compiler.AddResource(PublicationSchemaURI, document); err != nil {
		return nil, err
	}

	return compiler.Compile(PublicationSchemaURI)
})

// DecodePublication validates owned bounded claims, with no I/O, clock or URI
// resolution. Authentication, byte verification and reviewed frozen associations
// remain caller obligations. Calls are concurrent-safe; errors return zero claims.
func DecodePublication(raw []byte) (AnalysisPublication, error) {
	if len(raw) > PublicationMaximumBytes || !validUnicode(raw) {
		return AnalysisPublication{}, ErrPublication
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	document, err := decodeValue(decoder, 0)
	if err != nil || !boundedAnalysisNumbers(document) {
		return AnalysisPublication{}, ErrPublication
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return AnalysisPublication{}, ErrPublication
	}
	if header, ok := document.(map[string]any); ok {
		for key, expected := range map[string]string{"specversion": "1.0", "dataschema": PublicationSchemaURI} {
			if value, ok := header[key].(string); ok && value != "" && value != expected {
				return AnalysisPublication{}, errors.Join(ErrPublication, ErrPublicationVersion)
			}
		}
		if kind, ok := header["type"].(string); ok && strings.HasPrefix(kind, "dev.perfeng.integration.analysis-published.") && kind != "dev.perfeng.integration.analysis-published.v1" {
			return AnalysisPublication{}, errors.Join(ErrPublication, ErrPublicationVersion)
		}
	}
	schema, err := publicationSchema()
	if err != nil || schema.Validate(document) != nil {
		return AnalysisPublication{}, ErrPublication
	}
	var message AnalysisPublication
	if json.Unmarshal(raw, &message) != nil || !validPublicationContext(message) {
		return AnalysisPublication{}, ErrPublication
	}

	return message, nil
}

func validPublicationContext(message AnalysisPublication) bool {
	validURI := func(address string) bool {
		uri, err := url.Parse(address)
		return err == nil && uri.Hostname() != "" && uri.User == nil && uri.RawQuery == "" && uri.Fragment == ""
	}

	return validIntegrationContext(IntegrationMessage{Time: message.Time, Data: IntegrationData{Context: message.Data.Context}}) &&
		message.Source == message.Data.Run.Issuer && message.Data.Artifact.RunID == message.Data.Native.RunID &&
		(message.Data.Context.Baseline == nil) == (message.Data.Native.Baseline == nil) && validURI(message.Data.Artifact.URI) &&
		(message.Data.Native.Baseline == nil || validURI(message.Data.Native.Baseline.Artifact.URI))
}

// EncodePublication validates owned outbound values before returning bytes.
// Callers must not mutate input concurrently. Failure returns nil bytes; timestamp
// offsets and accepted exact numeric syntax survive round trips unchanged.
func EncodePublication(message AnalysisPublication) ([]byte, error) {
	raw, err := json.Marshal(message)
	if err != nil {
		return nil, ErrPublication
	}
	decoded, err := DecodePublication(raw)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(decoded, message) {
		return nil, ErrPublication
	}

	return raw, nil
}
