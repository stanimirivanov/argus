package perfeng

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// IntegrationSchemaURI identifies the pinned shared generation, not a fetch URL.
const IntegrationSchemaURI = "https://perfeng.dev/schemas/performance-integration/v1/performance-integration.schema.json"

// IntegrationMaximumBytes bounds one structured integration message.
const IntegrationMaximumBytes = 64 << 10

// ErrIntegration rejects invalid interchange without reflecting provider content.
var ErrIntegration = errors.New("invalid performance integration message")

// ErrIntegrationVersion additionally classifies an unsupported wire generation.
var ErrIntegrationVersion = errors.New("unsupported performance integration version")

// IntegrationIdentity is an issuer-scoped opaque identity, never a display name.
type IntegrationIdentity struct {
	Issuer string `json:"issuer"`
	ID     string `json:"id"`
}

// IntegrationReference pins the exact producer artifact bytes and revision.
type IntegrationReference struct {
	Issuer   string `json:"issuer"`
	ID       string `json:"id"`
	Revision string `json:"revision"`
	SHA256   string `json:"sha256"`
}

// IntegrationCandidate distinguishes source algorithm/digest from image identity.
type IntegrationCandidate struct {
	Source struct {
		Algorithm string `json:"algorithm"`
		Digest    string `json:"digest"`
	} `json:"source"`
	ImageDigest string `json:"imageDigest"`
}

// IntegrationContext retains immutable correlation and resource claims. Claims
// do not authenticate principals, prove provenance or grant execution authority.
type IntegrationContext struct {
	Request     IntegrationIdentity    `json:"request"`
	Attempt     IntegrationIdentity    `json:"attempt"`
	Principal   IntegrationIdentity    `json:"principal"`
	RequestedAt string                 `json:"requestedAt"`
	Deadline    string                 `json:"deadline"`
	Service     IntegrationReference   `json:"service"`
	Capability  IntegrationReference   `json:"capability"`
	Deployment  IntegrationReference   `json:"deployment"`
	Environment IntegrationReference   `json:"environment"`
	Workload    IntegrationReference   `json:"workload"`
	Profile     IntegrationReference   `json:"profile"`
	Policy      IntegrationReference   `json:"policy"`
	Baseline    *IntegrationReference  `json:"baseline"`
	Evidence    []IntegrationReference `json:"evidence"`
	Candidate   IntegrationCandidate   `json:"candidate"`
}

// IntegrationFailure carries the shared bounded error fields, not raw diagnostics.
type IntegrationFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// IntegrationData is the schema-selected closed union. Revision preserves exact
// integer-valued JSON number syntax within the published safe-integer range.
type IntegrationData struct {
	Context  IntegrationContext   `json:"context"`
	Run      *IntegrationIdentity `json:"run,omitempty"`
	Error    *IntegrationFailure  `json:"error,omitempty"`
	Revision *json.Number         `json:"revision,omitempty"`
	State    string               `json:"state,omitempty"`
	Reason   string               `json:"reason,omitempty"`
}

// IntegrationMessage is shared transport evidence, not Argus request state.
// Use DecodeIntegration/EncodeIntegration to validate its closed wire shape.
type IntegrationMessage struct {
	SpecVersion     string          `json:"specversion"`
	ID              string          `json:"id"`
	Source          string          `json:"source"`
	Type            string          `json:"type"`
	Time            string          `json:"time"`
	DataContentType string          `json:"datacontenttype"`
	DataSchema      string          `json:"dataschema"`
	Data            IntegrationData `json:"data"`
}

//go:embed snapshot/integration/schema.json
var integrationSchemaBytes []byte

var integrationSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileIntegrationSchema(integrationSchemaBytes)
})

func compileIntegrationSchema(raw []byte) (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(jsonschema.SchemeURLLoader{})
	compiler.AssertFormat()
	if err := compiler.AddResource(IntegrationSchemaURI, document); err != nil {
		return nil, err
	}

	return compiler.Compile(IntegrationSchemaURI)
}

// DecodeIntegration returns owned values or a zero message on failure. It checks
// bounded unique-key Unicode JSON, the pinned schema and per-message identity/time
// semantics. Calls are concurrent-safe and perform no I/O or wall-clock lookup.
// Authentication, freshness and cross-message lifecycle checks belong to callers.
func DecodeIntegration(raw []byte) (IntegrationMessage, error) {
	if len(raw) > IntegrationMaximumBytes || !validUnicode(raw) {
		return IntegrationMessage{}, ErrIntegration
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	document, err := decodeValue(decoder, 0)
	if err != nil {
		return IntegrationMessage{}, ErrIntegration
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return IntegrationMessage{}, ErrIntegration
	}
	if header, ok := document.(map[string]any); ok {
		for key, expected := range map[string]string{"specversion": "1.0", "dataschema": IntegrationSchemaURI} {
			if value, ok := header[key].(string); ok && value != "" && value != expected {
				return IntegrationMessage{}, errors.Join(ErrIntegration, ErrIntegrationVersion)
			}
		}
		if kind, ok := header["type"].(string); ok && strings.HasPrefix(kind, "dev.perfeng.integration.") && !strings.HasSuffix(kind, ".v1") {
			return IntegrationMessage{}, errors.Join(ErrIntegration, ErrIntegrationVersion)
		}
	}
	schema, err := integrationSchema()
	if err != nil || schema.Validate(document) != nil {
		return IntegrationMessage{}, ErrIntegration
	}
	var message IntegrationMessage
	if json.Unmarshal(raw, &message) != nil || !validIntegrationContext(message) {
		return IntegrationMessage{}, ErrIntegration
	}

	return message, nil
}

// EncodeIntegration validates outbound messages before returning bytes. Callers
// own slices/pointers and must not mutate them concurrently. No partial output is
// returned on failure; timestamp offsets and numeric revision syntax are retained.
func EncodeIntegration(message IntegrationMessage) ([]byte, error) {
	raw, err := json.Marshal(message)
	if err != nil {
		return nil, ErrIntegration
	}
	decoded, err := DecodeIntegration(raw)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(decoded, message) {
		return nil, ErrIntegration
	}

	return raw, nil
}

func validIntegrationContext(message IntegrationMessage) bool {
	claims := message.Data.Context
	requested, requestErr := time.Parse(time.RFC3339Nano, claims.RequestedAt)
	deadline, deadlineErr := time.Parse(time.RFC3339Nano, claims.Deadline)
	event, eventErr := time.Parse(time.RFC3339Nano, message.Time)
	if requestErr != nil || deadlineErr != nil || eventErr != nil || !deadline.After(requested) || event.Before(requested) || claims.Request == claims.Attempt {
		return false
	}
	if (message.Type == "dev.perfeng.integration.requested.v1" || message.Type == "dev.perfeng.integration.cancel-requested.v1") && message.Source != claims.Request.Issuer {
		return false
	}

	return message.Type != "dev.perfeng.integration.accepted.v1" || event.Before(deadline)
}
