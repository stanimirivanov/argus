package postgres

import (
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
)

func TestExecutionAttemptFingerprintUsesStablePrivateEncoding(t *testing.T) {
	t.Parallel()

	fingerprint, err := executionAttemptFingerprint(fingerprintTestExecutionAttempt())
	if err != nil {
		t.Fatalf("fingerprint execution attempt: %v", err)
	}

	// Changing this value would turn an exact retry of existing evidence into a
	// conflict and therefore requires an explicit data-compatibility decision.
	const want = "727b106fac21aeaef017713176bd2f4eb95dd442d6380d4a6ca38c2e3fdf91bf"
	if fingerprint != want {
		t.Fatalf("fingerprint = %q, want %q", fingerprint, want)
	}
}

func fingerprintTestExecutionAttempt() execution.Attempt {
	return execution.Attempt{
		APIVersion: execution.AttemptAPIVersion, AttemptID: "attempt-42",
		Manifest: execution.ManifestReference{
			APIVersion: "argus.dev/execution-manifest/v1", SHA256: strings.Repeat("a", 64),
		},
		Stage: execution.StageSelected,
		TestRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-42",
			},
			Owner: "example", Name: "orders-tests",
		},
		TestRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("b", 40),
		},
		AdapterID: "generic-http", AdapterVersion: "1.0.0",
		StartedAt:   time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 9, 26, 8, 0, 1, 0, time.UTC),
		Outcome:     execution.AttemptPassed,
		Results: []execution.TestResult{{
			SuiteKey: "orders", TestKey: "create", Outcome: execution.TestPassed,
			Duration: time.Second,
		}},
		Artifacts: []execution.ArtifactReference{{
			Key: "junit", Kind: "junit-xml", URI: "s3://argus/attempts/42/junit.xml",
			SHA256: strings.Repeat("c", 64),
		}},
	}
}
