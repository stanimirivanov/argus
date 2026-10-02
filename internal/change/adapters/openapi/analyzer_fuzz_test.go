package openapi

import "testing"

// FuzzOpenAPIDocumentParsing exercises both YAML/JSON and semantic parsing.
func FuzzOpenAPIDocumentParsing(f *testing.F) {
	f.Add([]byte(openAPISpec("string")))
	f.Add([]byte(`{"openapi":"3.1.0","paths":{}}`))
	f.Add([]byte("openapi: 3.1.0\npaths: [invalid]\n"))
	f.Add([]byte("not an OpenAPI document"))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 16<<10 {
			t.Skip()
		}
		document, isOpenAPI, err := parseDocument(data)
		if err != nil {
			if document != nil {
				t.Fatal("invalid document returned a parsed model")
			}
			return
		}
		if (document != nil) != isOpenAPI {
			t.Fatalf("document presence = %t, OpenAPI discovery = %t", document != nil, isOpenAPI)
		}
		if document != nil && document.operations == nil {
			t.Fatal("parsed document has no operation map")
		}
	})
}
