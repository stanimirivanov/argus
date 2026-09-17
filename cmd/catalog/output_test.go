package main

import (
	"encoding/json"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestSnapshotOutputPreservesPublicJSONShape(t *testing.T) {
	t.Parallel()

	output := newSnapshotOutput(catalog.Snapshot{
		APIVersion: "argus.dev/repository-descriptor/v1",
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider:             catalog.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "source-1",
			},
			Owner: "example",
			Name:  "orders",
		},
		Revision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1,
			Digest:    testRevision,
		},
		Capabilities: []catalog.Capability{},
		Components:   []catalog.Component{},
		TestSuites:   []catalog.TestSuite{},
	})

	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal snapshot output: %v", err)
	}

	const want = `{"apiVersion":"argus.dev/repository-descriptor/v1","repository":{"identity":{"provider":"github","host":"github.com","providerRepositoryId":"source-1"},"owner":"example","name":"orders"},"revision":{"algorithm":"git-sha1","digest":"0123456789abcdef0123456789abcdef01234567"},"capabilities":[],"components":[],"testSuites":[]}`
	if string(encoded) != want {
		t.Fatalf("snapshot output = %s, want %s", encoded, want)
	}
}
