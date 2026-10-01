package contracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compileEmbeddedSchema(data []byte, schemaID, documentName string) (*jsonschema.Schema, error) {
	var schemaDocument any
	if err := json.Unmarshal(data, &schemaDocument); err != nil {
		return nil, fmt.Errorf("decode embedded %s schema: %w", documentName, err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource(schemaID, schemaDocument); err != nil {
		return nil, fmt.Errorf("add embedded %s schema: %w", documentName, err)
	}

	schema, err := compiler.Compile(schemaID)
	if err != nil {
		return nil, fmt.Errorf("compile embedded %s schema: %w", documentName, err)
	}

	return schema, nil
}

func decodeSingleJSONValue(data []byte, documentName string) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode %s JSON: %w", documentName, err)
	}

	var trailing any
	err := decoder.Decode(&trailing)
	if !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("decode %s JSON: multiple JSON values", documentName)
		}

		return nil, fmt.Errorf("decode %s JSON trailer: %w", documentName, err)
	}

	return document, nil
}
