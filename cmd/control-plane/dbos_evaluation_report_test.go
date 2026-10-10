//go:build dbose2e

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type evaluationHost struct {
	RecordedAtUTC  string `json:"recorded_at_utc"`
	Go             string `json:"go"`
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	LogicalCPUs    int    `json:"logical_cpus"`
	PostgresImage  string `json:"postgres_image"`
	ImageID        string `json:"postgres_image_id"`
	PostgresServer string `json:"postgres_server"`
	SourceSHA256   string `json:"go_source_sha256"`
	ModuleSHA256   string `json:"module_manifest_sha256"`
	RaceEnabled    bool   `json:"race_enabled"`
	Workload       string `json:"workload"`
}

func evaluationHostInfo(t *testing.T, harness *dbosWebhookHarness) evaluationHost {
	t.Helper()
	root := dbosEvaluationRoot(t)
	var server string
	if err := harness.reader.QueryRow(t.Context(), "SHOW server_version").Scan(&server); err != nil {
		t.Fatalf("read PostgreSQL provenance: %v", err)
	}
	manifest := sha256.Sum256(append(readEvaluationFile(t, filepath.Join(root, "go.mod")), readEvaluationFile(t, filepath.Join(root, "go.sum"))...))

	return evaluationHost{
		RecordedAtUTC: time.Now().UTC().Format(time.RFC3339), Go: runtime.Version(),
		OS: runtime.GOOS, Architecture: runtime.GOARCH, LogicalCPUs: runtime.NumCPU(),
		PostgresImage: "postgres:17.11", PostgresServer: server,
		ImageID:      harness.imageID,
		SourceSHA256: evaluationSourceDigest(t, root), ModuleSHA256: hex.EncodeToString(manifest[:]),
		RaceEnabled: evaluationRaceEnabled(), Workload: "serial loopback HTTP; synthetic GitHub and mapped OpenAPI change; not a production capacity baseline",
	}
}

func evaluationSourceDigest(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	for _, directory := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			// WalkDir is lexical; include paths to distinguish renamed inputs.
			_, _ = hash.Write([]byte(filepath.ToSlash(relative) + "\x00"))
			_, _ = hash.Write(readEvaluationFile(t, path))

			return nil
		})
		if err != nil {
			t.Fatalf("fingerprint evaluation source: %v", err)
		}
	}

	return hex.EncodeToString(hash.Sum(nil))
}

// retainDBOSEvaluationReport writes only synthetic metrics and fingerprints,
// never connection strings, provider credentials, bodies, or host paths.
// Unique ignored artifacts survive test cleanup and are uploaded by CI even
// when another test fails. A missing report is not a successful experiment.
func retainDBOSEvaluationReport(t *testing.T, name string, report any) {
	t.Helper()
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("encode DBOS evaluation report: %v", err)
	}
	directory := filepath.Join(dbosEvaluationRoot(t), ".local", "dbos-evaluation")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("create ignored evaluation artifact directory: %v", err)
	}
	file, err := os.CreateTemp(directory, name+"-*.json")
	if err != nil {
		t.Fatalf("create evaluation report: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close evaluation report: %v", err)
	}
	writeEvaluationFile(t, file.Name(), append(data, '\n'))
	t.Logf("DBOS_EVALUATION_REPORT %s\n%s", filepath.Base(file.Name()), data)
}

// evaluationEvidenceDigest includes every catalog table, not just counts or
// selected fingerprints. Deleting checkpoint history must leave all immutable
// business rows and the Argus migration ledger byte-equivalent as JSONB.
func evaluationEvidenceDigest(t *testing.T, harness *dbosWebhookHarness) string {
	t.Helper()
	rows, err := harness.reader.Query(t.Context(), "SELECT tablename FROM pg_tables WHERE schemaname = 'argus_catalog' ORDER BY tablename")
	if err != nil {
		t.Fatalf("list immutable evidence tables: %v", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("read immutable evidence table names: %v", err)
	}
	hash := sha256.New()
	for _, name := range names {
		var document string
		query := "SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY to_jsonb(e)::text), '[]'::jsonb)::text FROM " + pgx.Identifier{"argus_catalog", name}.Sanitize() + " AS e"
		if err := harness.reader.QueryRow(t.Context(), query).Scan(&document); err != nil {
			t.Fatalf("fingerprint immutable evidence: %v", err)
		}
		_, _ = hash.Write([]byte(name + "\x00" + document))
	}

	return hex.EncodeToString(hash.Sum(nil))
}

type evaluationLatency struct {
	SamplesNS []int64 `json:"samples_ns"`
	MedianNS  int64   `json:"median_ns"`
	P95NS     int64   `json:"p95_ns"`
}

// summarizeEvaluationLatency uses nearest-rank percentiles and retains raw
// samples so another analyst can reanalyze them. No latency budget is inferred.
func summarizeEvaluationLatency(samples []int64) evaluationLatency {
	result := evaluationLatency{SamplesNS: append([]int64(nil), samples...)}
	if len(samples) == 0 {
		return result
	}
	ordered := append([]int64(nil), samples...)
	sort.Slice(ordered, func(a, b int) bool { return ordered[a] < ordered[b] })
	result.MedianNS = ordered[(len(ordered)+1)/2-1]
	result.P95NS = ordered[(95*len(ordered)+99)/100-1]

	return result
}

// TestDBOSEvaluationPercentiles runs without Docker even though its owning
// source is tagged. It guards the units and small-sample percentile arithmetic.
func TestDBOSEvaluationPercentiles(t *testing.T) {
	for _, test := range []struct {
		samples []int64
		median  int64
		p95     int64
	}{
		{nil, 0, 0}, {[]int64{8}, 8, 8}, {[]int64{9, 1, 7, 3}, 3, 9},
	} {
		result := summarizeEvaluationLatency(test.samples)
		if result.MedianNS != test.median || result.P95NS != test.p95 {
			t.Fatalf("percentiles for %v: %+v", test.samples, result)
		}
	}
	// The caller's chronological samples are never sorted in place.
	samples := []int64{9, 1}
	if result := summarizeEvaluationLatency(samples); result.SamplesNS[0] != 9 || samples[0] != 9 {
		t.Fatal("percentile reporting reordered raw samples")
	}
}
