package contracts_test

import (
	"bytes"
	"testing"

	"github.com/stanimirivanov/argus/contracts"
)

func TestEmbeddedSchemaReader(t *testing.T) {
	t.Parallel()

	data, err := contracts.ReadGeneratedSchema("repository-descriptor/v1/repository-descriptor.schema.json")
	if err != nil {
		t.Fatalf("read embedded schema: %v", err)
	}
	if !bytes.Contains(data, []byte(`"$id": "https://argus.dev/contracts/repository-descriptor/v1/schema.json"`)) {
		t.Fatal("embedded schema has unexpected identifier")
	}
	if _, err := contracts.ReadGeneratedSchema("../source/repository-descriptor-v1.ts"); err == nil {
		t.Fatal("schema accessor unexpectedly read outside generated artifacts")
	}
}
