// Package perfeng validates pinned public Perfeng interchange offline.
// Its wire values never confer authentication, ownership or execution authority.
package perfeng

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed snapshot/catalog.schema.json
var schemaBytes []byte

var catalogSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	var document any
	if err := json.Unmarshal(schemaBytes, &document); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(jsonschema.SchemeURLLoader{})
	compiler.AssertFormat()
	const id = "https://perfeng.dev/schemas/performance-catalog/v1/performance-catalog.schema.json"
	if err := compiler.AddResource(id, document); err != nil {
		return nil, err
	}

	return compiler.Compile(id)
})

// Expectations must come from reviewed configuration and authenticated scope,
// independently of the supplied pages. ProducerVersion pins the accepted release.
type Expectations struct {
	Issuer, PrincipalIssuer, PrincipalID, ProducerID, ProducerVersion string
}

// Validate checks an entire ordered page set at an explicit observation instant.
// It performs no I/O, mutates no input and publishes nothing on success or failure.
// Callers must own the page bytes and recheck freshness before later publication.
// Local safety limits are 2 MiB/page, 64 MiB/snapshot and 32 JSON nesting levels.
func Validate(pages []json.RawMessage, expected Expectations, observedAt time.Time) error {
	if err := checkInput(pages, expected, observedAt); err != nil {
		return err
	}
	schema, err := catalogSchema()
	if err != nil {
		return fmt.Errorf("compile pinned catalog schema: %w", err)
	}
	var metadata map[string]any
	items := make([]any, 0)
	var cursor any
	seen := map[any]bool{nil: true}
	for index, data := range pages {
		page, err := decodePage(data, schema)
		if err != nil {
			return fmt.Errorf("invalid catalog page %d: %w", index, err)
		}
		snapshot := object(page["snapshot"])
		if index == 0 {
			metadata = snapshot
		}
		position, batch := object(page["page"]), page["items"].([]any)
		if !reflect.DeepEqual(metadata, snapshot) || integer(position["index"]) != index ||
			integer(snapshot["totalPages"]) != len(pages) || position["cursor"] != cursor ||
			(position["nextCursor"] == nil) != (index == len(pages)-1) ||
			(len(batch) == 0 && (len(pages) != 1 || integer(snapshot["totalItems"]) != 0)) {
			return errors.New("incomplete, mixed or reordered catalog pages")
		}
		cursor = position["nextCursor"]
		if cursor != nil && seen[cursor] {
			return errors.New("repeated catalog cursor")
		}
		seen[cursor] = true
		items = append(items, batch...)
	}
	if err := checkScope(metadata, expected, observedAt); err != nil {
		return err
	}
	digest := sha256.Sum256(canonical(items))
	if len(items) != integer(metadata["totalItems"]) ||
		hex.EncodeToString(digest[:]) != object(metadata["reference"])["sha256"] {
		return errors.New("catalog count or content digest mismatch")
	}

	return checkResources(items)
}

func checkInput(pages []json.RawMessage, expected Expectations, observedAt time.Time) error {
	if len(pages) == 0 || len(pages) > 1000 || observedAt.IsZero() ||
		expected.Issuer == "" || expected.PrincipalIssuer == "" || expected.PrincipalID == "" ||
		expected.ProducerID == "" || expected.ProducerVersion == "" {
		return errors.New("catalog pages and independent expectations are required")
	}
	totalBytes := 0
	for _, page := range pages {
		totalBytes += len(page)
		if totalBytes > 64<<20 {
			return errors.New("catalog snapshot exceeds local byte limit")
		}
	}

	return nil
}

func decodePage(data []byte, schema *jsonschema.Schema) (map[string]any, error) {
	if len(data) > 2<<20 || !validUnicode(data) {
		return nil, errors.New("catalog page exceeds limit or contains invalid Unicode")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	document, err := decodeValue(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected catalog JSON trailer")
	}
	if err := schema.Validate(document); err != nil {
		// Do not reflect provider-controlled data into logs or operator output.
		return nil, errors.New("catalog page does not match pinned schema")
	}

	return object(document), nil
}

func checkScope(metadata map[string]any, expected Expectations, at time.Time) error {
	producer := object(metadata["producer"])
	if object(metadata["reference"])["issuer"] != expected.Issuer ||
		!reflect.DeepEqual(metadata["principal"], map[string]any{"issuer": expected.PrincipalIssuer, "id": expected.PrincipalID}) ||
		!reflect.DeepEqual(producer["identity"], map[string]any{"issuer": expected.Issuer, "id": expected.ProducerID}) ||
		producer["version"] != expected.ProducerVersion {
		return errors.New("unexpected catalog authority, principal or producer release")
	}
	generated, err := time.Parse(time.RFC3339Nano, metadata["generatedAt"].(string))
	if err != nil {
		return errors.New("invalid catalog generation instant")
	}
	expires, err := time.Parse(time.RFC3339Nano, metadata["validUntil"].(string))
	if err != nil || !generated.Before(expires) || at.Before(generated) || !at.Before(expires) {
		return errors.New("catalog is stale, future-dated or has invalid freshness window")
	}

	return nil
}

func object(value any) map[string]any { return value.(map[string]any) }

func integer(value any) int {
	// Schema validation guarantees bounded nonnegative integers. Float64 also
	// accepts legal JSON integer spellings such as 1.0 and 1e0.
	number, err := value.(json.Number).Float64()
	if err != nil {
		return -1
	}

	return int(number)
}
