package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckedInWorkflowMatchesPolicy(t *testing.T) {
	policy := testPolicy(t)
	path := filepath.Join("..", "..", ".github", "workflows", "validate.yml")
	workflow := readTestFile(t, path)
	drift, err := checkWorkflow(policy, workflow)
	if err != nil {
		t.Fatal(err)
	}
	if len(drift) != 0 {
		t.Fatalf("checked-in validation workflow drifts from policy: %v", drift)
	}
}

func TestWorkflowDrift(t *testing.T) {
	base := strings.ReplaceAll(string(readTestFile(t, filepath.Join("testdata", "valid.yml"))), "\r\n", "\n")
	cases := []struct {
		name    string
		change  func(string) string
		finding string
	}{
		{"renamed status", replaceOnce("name: CI policy", "name: Skipped policy"), "display name"},
		{"removed job", replaceOnce("  ci-policy:\n    name: CI policy\n    runs-on: ubuntu-24.04\n    steps:\n      - run: make ci-policy-check\n", ""), "missing job \"ci-policy\""},
		{"removed target", replaceOnce("run: make verify", "run: echo skipped"), "must run \"make verify\""},
		{"job conditional", replaceOnce("  ci-policy:\n    name:", "  ci-policy:\n    if: false\n    name:"), "must not use if, needs, or continue-on-error"},
		{"job depends on skipped job", replaceOnce("  ci-policy:\n    name:", "  ci-policy:\n    needs: race\n    name:"), "must not use if, needs, or continue-on-error"},
		{"job ignores errors", replaceOnce("  ci-policy:\n    name:", "  ci-policy:\n    continue-on-error: true\n    name:"), "must not use if, needs, or continue-on-error"},
		{"target conditional", replaceOnce("      - run: make verify", "      - if: false\n        run: make verify"), "without if or continue-on-error"},
		{"target ignores errors", replaceOnce("      - run: make verify", "      - continue-on-error: true\n        run: make verify"), "without if or continue-on-error"},
		{"workflow Make flags", replaceOnce("jobs:\n", "env:\n  MAKEFLAGS: -n\njobs:\n"), "workflow must not set MAKEFLAGS"},
		{"job Make flags", replaceOnce("  ci-policy:\n    name:", "  ci-policy:\n    env:\n      MFLAGS: -n\n    name:"), "job \"ci-policy\" must not set MFLAGS"},
		{"step Make flags", replaceOnce("      - run: make verify", "      - run: make verify\n        env:\n          GNUMAKEFLAGS: -n"), "required Make step must not set GNUMAKEFLAGS"},
		{"step Makefiles", replaceOnce("      - run: make verify", "      - run: make verify\n        env:\n          MAKEFILES: injected.mk"), "required Make step must not set MAKEFILES"},
		{"workflow run defaults", replaceOnce("jobs:\n", "defaults:\n  run:\n    shell: bash\njobs:\n"), "workflow must not override defaults.run"},
		{"job run defaults", replaceOnce("  ci-policy:\n    name:", "  ci-policy:\n    defaults:\n      run:\n        working-directory: tools\n    name:"), "job \"ci-policy\" must not override defaults.run"},
		{"target shell override", replaceOnce("      - run: make verify", "      - run: make verify\n        shell: bash"), "without shell or working-directory override"},
		{"target directory override", replaceOnce("      - run: make verify", "      - run: make verify\n        working-directory: tools"), "without shell or working-directory override"},
		{"pull request removed", replaceOnce("  pull_request:\n", ""), "unfiltered pull_request trigger"},
		{"pull request filtered", replaceOnce("  pull_request:\n", "  pull_request:\n    paths: [src/**]\n"), "unfiltered pull_request trigger"},
		{"wrong runner", replaceOnce("runs-on: ${{ matrix.os }}", "runs-on: ubuntu-24.04"), "runs-on"},
		{"OS matrix changed", replaceOnce("os: [ubuntu-24.04, windows-2025]", "os: [ubuntu-24.04]"), "matrix.os"},
		{"matrix include", replaceOnce("        os: [ubuntu-24.04, windows-2025]", "        os: [ubuntu-24.04, windows-2025]\n        include: []"), "unexpected matrix axis \"include\""},
		{"matrix exclude", replaceOnce("        os: [ubuntu-24.04, windows-2025]", "        os: [ubuntu-24.04, windows-2025]\n        exclude: []"), "unexpected matrix axis \"exclude\""},
		{"extra matrix axis", replaceOnce("        os: [ubuntu-24.04, windows-2025]", "        os: [ubuntu-24.04, windows-2025]\n        arch: [amd64]"), "unexpected matrix axis \"arch\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutated := tc.change(base)
			if mutated == base {
				t.Fatal("fixture mutation did not change input")
			}
			drift, err := checkWorkflow(testPolicy(t), []byte(mutated))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(drift, "\n"), tc.finding) {
				t.Fatalf("want finding %q, got %v", tc.finding, drift)
			}
		})
	}
}

func TestInvalidPolicyAndWorkflowFailClosed(t *testing.T) {
	if _, err := loadPolicy([]byte(`{"jobs":{}}`)); err == nil {
		t.Fatal("empty policy must fail")
	}
	if _, err := loadPolicy(append(append([]byte(nil), checkedInPolicy...), []byte(` {}`)...)); err == nil {
		t.Fatal("trailing JSON must fail")
	}
	if _, err := checkWorkflow(testPolicy(t), []byte("jobs: [")); err == nil {
		t.Fatal("invalid workflow YAML must fail")
	}
}

func TestRunExitCodes(t *testing.T) {
	var stdout, stderr bytes.Buffer
	validPath := filepath.Join("testdata", "valid.yml")
	if exitCode := run(validPath, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("valid fixture exit = %d, stderr = %s", exitCode, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := run(filepath.Join("testdata", "missing.yml"), &stdout, &stderr); exitCode != 2 {
		t.Fatalf("missing workflow exit = %d, stderr = %s", exitCode, stderr.String())
	}
}

func testPolicy(t *testing.T) ciPolicy {
	t.Helper()
	policy, err := loadPolicy(checkedInPolicy)
	if err != nil {
		t.Fatal(err)
	}

	return policy
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func replaceOnce(old, replacement string) func(string) string {
	return func(input string) string {
		return strings.Replace(input, old, replacement, 1)
	}
}
