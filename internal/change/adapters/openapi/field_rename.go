package openapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/stanimirivanov/argus/internal/change"
)

// ErrNoCompatibleFieldRename means immutable contract documents do not prove
// exactly one explicitly declared, otherwise unchanged JSON field rename.
var ErrNoCompatibleFieldRename = errors.New("no authoritative compatible field rename")

// DetectCompatibleFieldRename compares one operation at two trusted immutable
// revisions. The new property's x-argus-renamed-from annotation declares its
// identity; equality of the complete operation after reversing only that
// rename proves that type, requiredness, values, and flow did not also change.
// Local or external references and multiple rename declarations abstain.
func DetectCompatibleFieldRename(
	baseBytes, headBytes []byte, documentPath, method, path string,
) (change.FieldRename, error) {
	if len(baseBytes) == 0 || len(headBytes) == 0 || len(baseBytes) > 5<<20 || len(headBytes) > 5<<20 {
		return change.FieldRename{}, ErrNoCompatibleFieldRename
	}
	base, baseAPI, baseErr := parseDocument(baseBytes)
	head, headAPI, headErr := parseDocument(headBytes)
	if baseErr != nil || headErr != nil || !baseAPI || !headAPI {
		return change.FieldRename{}, ErrNoCompatibleFieldRename
	}
	baseOperation, ok := operationObject(base.root, method, path)
	if !ok {
		return change.FieldRename{}, ErrNoCompatibleFieldRename
	}
	headOperation, ok := operationObject(head.root, method, path)
	if !ok || len(collectReferences(baseOperation)) != 0 || len(collectReferences(headOperation)) != 0 {
		return change.FieldRename{}, ErrNoCompatibleFieldRename
	}
	baseID, baseIDOK := baseOperation["operationId"].(string)
	headID, headIDOK := headOperation["operationId"].(string)
	baseCaps, baseCapsErr := operationCapabilities(baseOperation)
	headCaps, headCapsErr := operationCapabilities(headOperation)
	if !baseIDOK || !headIDOK || baseID == "" || baseID != headID || baseCapsErr != nil ||
		headCapsErr != nil || len(baseCaps) == 0 || !slices.Equal(baseCaps, headCaps) {
		return change.FieldRename{}, ErrNoCompatibleFieldRename
	}
	candidates := declaredFieldRenames(headOperation)
	if len(candidates) != 1 {
		return change.FieldRename{}, ErrNoCompatibleFieldRename
	}
	candidate := candidates[0]
	baseSchema, ok := schemaAt(baseOperation, candidate)
	if !ok {
		return change.FieldRename{}, ErrNoCompatibleFieldRename
	}
	headSchema, ok := schemaAt(headOperation, candidate)
	if !ok || !validRenameProperties(baseSchema, headSchema, candidate) {
		return change.FieldRename{}, ErrNoCompatibleFieldRename
	}
	// Reparse to avoid mutating the caller's evidence while normalizing the new
	// operation back to its old name for complete semantic comparison.
	var normalized map[string]any
	encoded, err := json.Marshal(headOperation)
	if err != nil || json.Unmarshal(encoded, &normalized) != nil {
		return change.FieldRename{}, ErrNoCompatibleFieldRename
	}
	normalizedSchema, ok := schemaAt(normalized, candidate)
	if !ok || !reverseFieldRename(normalizedSchema, candidate) {
		return change.FieldRename{}, ErrNoCompatibleFieldRename
	}
	baseCanonical, baseEncodeErr := json.Marshal(baseOperation)
	headCanonical, headEncodeErr := json.Marshal(normalized)
	if baseEncodeErr != nil || headEncodeErr != nil || !bytes.Equal(baseCanonical, headCanonical) {
		return change.FieldRename{}, ErrNoCompatibleFieldRename
	}
	baseSum, headSum := sha256.Sum256(baseBytes), sha256.Sum256(headBytes)

	return change.FieldRename{
		DocumentPath: documentPath, BaseSHA256: hex.EncodeToString(baseSum[:]),
		HeadSHA256: hex.EncodeToString(headSum[:]), Method: method, Path: path,
		OperationID: baseID, Capabilities: baseCaps, Direction: candidate.direction,
		StatusCode: candidate.status, PreviousName: candidate.previous, Name: candidate.name,
	}, nil
}

type fieldCandidate struct{ direction, status, previous, name string }

func operationObject(root map[string]any, method, path string) (map[string]any, bool) {
	paths, ok := stringMap(root["paths"])
	if !ok {
		return nil, false
	}
	item, ok := stringMap(paths[path])
	if !ok {
		return nil, false
	}
	operation, ok := stringMap(item[strings.ToLower(method)])

	return operation, ok
}

func declaredFieldRenames(operation map[string]any) []fieldCandidate {
	var result []fieldCandidate
	if request, ok := stringMap(operation["requestBody"]); ok {
		result = append(result, candidatesInContent(request, "request", "")...)
	}
	if responses, ok := stringMap(operation["responses"]); ok {
		for status, raw := range responses {
			response, valid := stringMap(raw)
			if valid {
				result = append(result, candidatesInContent(response, "response", status)...)
			}
		}
	}

	return result
}

func candidatesInContent(container map[string]any, direction, status string) []fieldCandidate {
	content, ok := stringMap(container["content"])
	if !ok {
		return nil
	}
	media, ok := stringMap(content["application/json"])
	if !ok {
		return nil
	}
	schema, ok := stringMap(media["schema"])
	if !ok || schema["type"] != "object" {
		return nil
	}
	properties, ok := stringMap(schema["properties"])
	if !ok {
		return nil
	}
	var result []fieldCandidate
	for name, raw := range properties {
		property, valid := stringMap(raw)
		if !valid {
			continue
		}
		if previous, declared := property["x-argus-renamed-from"].(string); declared {
			result = append(result, fieldCandidate{direction, status, previous, name})
		}
	}

	return result
}

func schemaAt(operation map[string]any, candidate fieldCandidate) (map[string]any, bool) {
	var container map[string]any
	if candidate.direction == "request" {
		container, _ = stringMap(operation["requestBody"])
	} else {
		responses, ok := stringMap(operation["responses"])
		if !ok {
			return nil, false
		}
		container, _ = stringMap(responses[candidate.status])
	}
	content, ok := stringMap(container["content"])
	if !ok {
		return nil, false
	}
	media, ok := stringMap(content["application/json"])
	if !ok {
		return nil, false
	}
	schema, ok := stringMap(media["schema"])

	return schema, ok && schema["type"] == "object"
}

func validRenameProperties(base, head map[string]any, candidate fieldCandidate) bool {
	if candidate.previous == "" || candidate.name == "" || candidate.previous == candidate.name ||
		len(candidate.previous) > 255 || len(candidate.name) > 255 {
		return false
	}
	before, ok := stringMap(base["properties"])
	if !ok {
		return false
	}
	after, ok := stringMap(head["properties"])
	if !ok {
		return false
	}
	_, oldExists := before[candidate.previous]
	_, newExists := after[candidate.name]
	_, collisionBefore := before[candidate.name]
	_, collisionAfter := after[candidate.previous]

	return oldExists && newExists && !collisionBefore && !collisionAfter
}

func reverseFieldRename(schema map[string]any, candidate fieldCandidate) bool {
	properties, ok := stringMap(schema["properties"])
	if !ok {
		return false
	}
	property, ok := stringMap(properties[candidate.name])
	if !ok || property["x-argus-renamed-from"] != candidate.previous {
		return false
	}
	delete(property, "x-argus-renamed-from")
	delete(properties, candidate.name)
	properties[candidate.previous] = property
	if required, exists := schema["required"].([]any); exists {
		for index, value := range required {
			if value == candidate.name {
				required[index] = candidate.previous
			}
		}
	}

	return true
}
