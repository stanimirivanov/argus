//go:build dbose2e

package main

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

const (
	evaluationSDKModule = "github.com/dbos-inc/dbos-transact-golang"
	evaluationOldSDK    = "v1.5.0"
	evaluationNewSDK    = "v1.6.0"
	evaluationNewSum    = "h1:lhn03aRlLj8Npx1LUOvx++pYsXzu/Dnbtbty7Wy7YCk="
	evaluationNewModSum = "h1:Eg3T6HFZ5Z38rpu3qNryuuLa2ysB933aVL4e7dpqzo0="
)

type evaluationBuild struct {
	SDK                string `json:"sdk"`
	SchemaVersion      int    `json:"schema_version"`
	BinarySHA256       string `json:"binary_sha256"`
	ApplicationVersion string `json:"application_version"`
	RaceInstrumented   bool   `json:"race_instrumented"`
	ModuleChecksum     string `json:"sdk_module_checksum"`
	executable         string
}

// buildDBOSEvaluation compiles the same Argus sources twice. The candidate
// uses a temporary module graph and a Go source overlay for its exact schema
// guard (123). Neither the root SDK pin nor the production guard (121) changes.
// This is candidate-release evaluation, not permission to bypass schema checks.
func buildDBOSEvaluation(t *testing.T, candidate bool) evaluationBuild {
	t.Helper()
	root := dbosEvaluationRoot(t)
	mod := readEvaluationFile(t, filepath.Join(root, "go.mod"))
	sum := readEvaluationFile(t, filepath.Join(root, "go.sum"))
	oldPin := evaluationSDKModule + " " + evaluationOldSDK
	if strings.Count(string(mod), oldPin) != 1 {
		t.Fatal("DBOS evaluation baseline changed; review the pinned matrix before running it")
	}
	build := evaluationBuild{SDK: evaluationOldSDK, SchemaVersion: 121, RaceInstrumented: evaluationRaceEnabled()}
	directory := t.TempDir()
	build.executable = filepath.Join(directory, "control-plane.test")
	if runtime.GOOS == "windows" {
		build.executable += ".exe"
	}
	args := []string{"test", "-c", "-mod=readonly", "-tags=dbose2e", "-o", build.executable}
	if build.RaceInstrumented {
		args = append(args, "-race")
	}
	if candidate {
		build.SDK, build.SchemaVersion = evaluationNewSDK, 123
		mod = []byte(strings.Replace(string(mod), oldPin, evaluationSDKModule+" "+evaluationNewSDK, 1))
		sum = append(sum, []byte(fmt.Sprintf("\n%s %s %s\n%s %s/go.mod %s\n",
			evaluationSDKModule, evaluationNewSDK, evaluationNewSum, evaluationSDKModule, evaluationNewSDK, evaluationNewModSum))...)
		modFile := filepath.Join(directory, "evaluation.mod")
		writeEvaluationFile(t, modFile, mod)
		writeEvaluationFile(t, filepath.Join(directory, "evaluation.sum"), sum)
		schemaFile := filepath.Join(root, "internal", "change", "adapters", "dbos", "schema.go")
		schema := string(readEvaluationFile(t, schemaFile))
		const oldGuard = "const evaluationSchemaVersion int64 = 121"
		if strings.Count(schema, oldGuard) != 1 {
			t.Fatal("DBOS schema guard changed; review the candidate overlay")
		}
		overlaySource := filepath.Join(directory, "schema.go")
		writeEvaluationFile(t, overlaySource, []byte(strings.Replace(schema, oldGuard, "const evaluationSchemaVersion int64 = 123", 1)))
		overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{schemaFile: overlaySource}})
		if err != nil {
			t.Fatalf("encode candidate build overlay: %v", err)
		}
		overlayFile := filepath.Join(directory, "overlay.json")
		writeEvaluationFile(t, overlayFile, overlay)
		args = append(args, "-modfile="+modFile, "-overlay="+overlayFile)
	}
	args = append(args, "./cmd/control-plane")
	command := exec.CommandContext(t.Context(), "go", args...)
	command.Dir = root
	// Ambient GOFLAGS can override the reviewed build graph or application.
	command.Env = append(dbosWorkerEnvironment(nil), "GOFLAGS=", "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile DBOS %s evaluation worker: %v\n%s", build.SDK, err, output)
	}
	digest := sha256.Sum256(readEvaluationFile(t, build.executable))
	build.BinarySHA256 = hex.EncodeToString(digest[:])
	info, err := buildinfo.ReadFile(build.executable)
	if err != nil {
		t.Fatalf("read evaluation worker build provenance: %v", err)
	}
	for _, dependency := range info.Deps {
		if dependency.Path == evaluationSDKModule && dependency.Version == build.SDK {
			build.ModuleChecksum = dependency.Sum
		}
	}
	if build.ModuleChecksum == "" || (candidate && build.ModuleChecksum != evaluationNewSum) {
		t.Fatal("worker does not contain the reviewed SDK version and checksum")
	}

	return build
}

func assertEvaluationDependencyGraphs(t *testing.T, oldBuild, newBuild evaluationBuild) {
	t.Helper()
	dependencies := func(build evaluationBuild) map[string]string {
		info, err := buildinfo.ReadFile(build.executable)
		if err != nil {
			t.Fatalf("read compatibility dependency graph: %v", err)
		}
		result := make(map[string]string)
		for _, dependency := range info.Deps {
			if dependency.Path != evaluationSDKModule {
				result[dependency.Path] = dependency.Version + " " + dependency.Sum
			}
		}

		return result
	}
	oldGraph, newGraph := dependencies(oldBuild), dependencies(newBuild)
	if len(oldGraph) != len(newGraph) {
		t.Fatal("candidate adds dependencies; review its license and vulnerability graph before evaluation")
	}
	for path, version := range oldGraph {
		if newGraph[path] != version {
			t.Fatalf("candidate changes transitive dependency %s; review its license and vulnerability graph", path)
		}
	}
}

func scanEvaluationCandidate(t *testing.T, build evaluationBuild) {
	t.Helper()
	command := exec.CommandContext(t.Context(), "go", "tool", "-modfile=tools/quality/go.mod", "govulncheck", "-mode=binary", build.executable)
	command.Dir = dbosEvaluationRoot(t)
	command.Env = append(dbosWorkerEnvironment(nil), "GOFLAGS=", "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("candidate binary vulnerability gate failed: %v\n%s", err, output)
	}
}

func evaluationRaceEnabled() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, setting := range info.Settings {
		if setting.Key == "-race" {
			return setting.Value == "true"
		}
	}

	return false
}

func dbosEvaluationRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("locate evaluation source root: %v", err)
	}
	if !strings.Contains(string(readEvaluationFile(t, filepath.Join(root, "go.mod"))), "module github.com/stanimirivanov/argus\n") {
		t.Fatal("evaluation must run from the control-plane Go package")
	}

	return root
}

func readEvaluationFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read evaluation input: %v", err)
	}

	return data
}

func writeEvaluationFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write evaluation artifact: %v", err)
	}
}
