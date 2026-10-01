// Package contracts exposes the generated JSON Schema artifacts embedded in
// Argus binaries. Go transport DTOs are internal to the product; the portable
// schemas and Effect sources in this workspace are the published contracts.
package contracts

import (
	"embed"
	"io/fs"
)

//go:embed generated
var generatedSchemas embed.FS

// ReadGeneratedSchema returns a copy of a generated schema by its path below
// contracts/generated (for example, repository-descriptor/v1/repository-descriptor.schema.json).
// It does not read the working tree, so deployed binaries validate against the
// same checked-in artifact used at build time.
func ReadGeneratedSchema(name string) ([]byte, error) {
	return fs.ReadFile(generatedSchemas, "generated/"+name)
}
