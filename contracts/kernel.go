// Package contracts exposes validated language bindings for Argus contracts.
package contracts

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	generated "github.com/stanimirivanov/argus/contracts/generated/go/kernel/v1"
)

const kernelSchemaID = "https://argus.dev/contracts/kernel/v1/kernel.schema.json"

//go:embed schemas/kernel/v1/kernel.schema.json
var kernelSchemaJSON []byte

var loadKernelSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	var schemaDocument any
	if err := json.Unmarshal(kernelSchemaJSON, &schemaDocument); err != nil {
		return nil, fmt.Errorf("decode embedded kernel schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource(kernelSchemaID, schemaDocument); err != nil {
		return nil, fmt.Errorf("add embedded kernel schema: %w", err)
	}

	schema, err := compiler.Compile(kernelSchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile embedded kernel schema: %w", err)
	}

	return schema, nil
})

// DecodeKernel validates a kernel document structurally and semantically before
// returning the generated Go representation. Callers never receive a partially
// trusted contract value.
func DecodeKernel(data []byte) (generated.KernelFixture, error) {
	var document generated.KernelFixture

	schema, err := loadKernelSchema()
	if err != nil {
		return document, err
	}

	var untyped any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&untyped); err != nil {
		return document, fmt.Errorf("decode kernel JSON: %w", err)
	}
	if err := schema.Validate(untyped); err != nil {
		return document, fmt.Errorf("validate kernel schema: %w", err)
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode generated kernel binding: %w", err)
	}
	if err := validateKernelInvariants(document); err != nil {
		return generated.KernelFixture{}, err
	}

	return document, nil
}

func validateKernelInvariants(document generated.KernelFixture) error {
	repositoryID := document.Repository.ID
	checks := []struct {
		path  string
		value generated.RepositoryID
	}{
		{path: "/revision/repositoryId", value: document.Revision.RepositoryID},
		{path: "/component/repositoryId", value: document.Component.RepositoryID},
		{path: "/evidence/provenance/repositoryId", value: document.Evidence.Provenance.RepositoryID},
	}
	for _, check := range checks {
		if check.value != repositoryID {
			return fmt.Errorf("validate kernel invariant %s: repository identity mismatch", check.path)
		}
	}
	if document.Test.SuiteID != document.Suite.ID {
		return errors.New("validate kernel invariant /test/suiteId: suite identity mismatch")
	}
	if document.Evidence.Provenance.Revision != document.Revision.Value {
		return errors.New("validate kernel invariant /evidence/provenance/revision: revision mismatch")
	}
	if document.Evidence.ExpiresAt != nil && !document.Evidence.ExpiresAt.After(document.Evidence.ObservedAt) {
		return errors.New("validate kernel invariant /evidence/expiresAt: expiry must follow observation")
	}
	if document.Compatibility.BundleVersion != document.ContractsVersion {
		return errors.New("validate kernel invariant /compatibility/bundleVersion: contracts version mismatch")
	}

	return nil
}
