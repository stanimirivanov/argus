package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type policyFixture struct {
	repository string
	scratch    string
	taskFile   string
	hostFile   string
	task       taskPolicy
	host       hostState
}

func newPolicyFixture(t *testing.T, profile string, roots, checks []string) policyFixture {
	t.Helper()
	base := t.TempDir()
	repository := filepath.Join(base, "repository")
	scratch := filepath.Join(base, "scratch")
	for _, path := range []string{repository, scratch} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(repository, "Makefile"), []byte("verify:\n\t@echo ok\n"), 0o600); err != nil {
		t.Fatalf("write fixture Makefile: %v", err)
	}
	canonical, err := existingDirectory(repository)
	if err != nil {
		t.Fatalf("resolve fixture repository: %v", err)
	}
	canonicalScratch, err := existingDirectory(scratch)
	if err != nil {
		t.Fatalf("resolve fixture scratch: %v", err)
	}
	hostRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		hostRoots = append(hostRoots, filepath.Join(canonical, filepath.FromSlash(root)))
	}
	fixture := policyFixture{
		repository: canonical,
		scratch:    canonicalScratch,
		taskFile:   filepath.Join(base, "task.json"),
		hostFile:   filepath.Join(base, "host.json"),
		task: taskPolicy{
			Version:       1,
			TaskID:        "M03-catalog",
			Profile:       profile,
			Repository:    canonical,
			WritableRoots: roots,
			Network:       "deny",
			Credentials:   "none",
			Checks:        checks,
		},
		host: hostState{
			Version:       1,
			Repository:    canonical,
			WritableRoots: hostRoots,
			ScratchRoot:   canonicalScratch,
			Network:       "deny",
			Credentials:   "none",
		},
	}
	fixture.write(t)

	return fixture
}

func (fixture policyFixture) write(t *testing.T) {
	t.Helper()
	for path, value := range map[string]any{fixture.taskFile: fixture.task, fixture.hostFile: fixture.host} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAuditProfileAcceptsOnlyReadOnlyScope(t *testing.T) {
	fixture := newPolicyFixture(t, "audit", nil, nil)
	p, err := loadPolicy(fixture.repository, fixture.taskFile, fixture.hostFile)
	if err != nil {
		t.Fatal(err)
	}
	if p.task.Profile != "audit" || len(p.writableRoots) != 0 {
		t.Fatalf("unexpected audit scope: %+v", p)
	}
	if _, err := runVerification(t.Context(), p, "verify"); err == nil {
		t.Fatal("audit profile ran a verification command")
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"inspect", "--repo", fixture.repository, "--task", fixture.taskFile, "--host-state", fixture.hostFile}, &stdout, &stderr); code != 0 {
		t.Fatalf("inspect exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"profile":"audit"`) {
		t.Fatalf("missing audit profile: %s", stdout.String())
	}
}

func TestAuditProfileRejectsWritableRoots(t *testing.T) {
	fixture := newPolicyFixture(t, "audit", []string{"internal/catalog"}, nil)
	if _, err := loadPolicy(fixture.repository, fixture.taskFile, fixture.hostFile); err == nil {
		t.Fatal("audit profile accepted a writable root")
	}
}

func TestImplementationRequiresMatchingScopedHostRoots(t *testing.T) {
	fixture := newPolicyFixture(t, "implementation", []string{"internal/catalog", "contracts"}, []string{"verify"})
	p, err := loadPolicy(fixture.repository, fixture.taskFile, fixture.hostFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.writableRoots) != 2 || !slices.Contains(p.task.Checks, "verify") {
		t.Fatalf("unexpected implementation scope: %+v", p)
	}
	fixture.host.WritableRoots = []string{filepath.Join(fixture.repository, "internal")}
	fixture.write(t)
	if _, err := loadPolicy(fixture.repository, fixture.taskFile, fixture.hostFile); err == nil {
		t.Fatal("accepted a broader or incomplete host scope")
	}
}

func TestPolicyRejectsOnlineAcceptanceTarget(t *testing.T) {
	fixture := newPolicyFixture(t, "implementation", []string{"internal/catalog"}, []string{"validate"})
	if _, err := loadPolicy(fixture.repository, fixture.taskFile, fixture.hostFile); err == nil {
		t.Fatal("denied-network profile accepted make validate")
	}
}

func TestPolicyRejectsTraversalAndWholeRepository(t *testing.T) {
	for _, relative := range []string{"..", "internal/../contracts", ".", ".git", "internal\\catalog", "/outside"} {
		t.Run(relative, func(t *testing.T) {
			fixture := newPolicyFixture(t, "implementation", []string{relative}, []string{"verify"})
			if _, err := loadPolicy(fixture.repository, fixture.taskFile, fixture.hostFile); err == nil {
				t.Fatalf("accepted unsafe writable root %q", relative)
			}
		})
	}
}

func TestPolicyRejectsSymlinkEscape(t *testing.T) {
	fixture := newPolicyFixture(t, "implementation", []string{"external/new"}, []string{"verify"})
	outside := filepath.Dir(fixture.repository)
	if err := os.Symlink(outside, filepath.Join(fixture.repository, "external")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := loadPolicy(fixture.repository, fixture.taskFile, fixture.hostFile); err == nil {
		t.Fatal("accepted symlink that escapes the repository")
	}
}

func TestPolicyRejectsNonDenyNetworkOrCredentials(t *testing.T) {
	fixture := newPolicyFixture(t, "implementation", []string{"internal/catalog"}, []string{"verify"})
	for _, mutate := range []func(*hostState){
		func(host *hostState) { host.Network = "unrestricted" },
		func(host *hostState) { host.Credentials = "inherited" },
	} {
		host := fixture.host
		mutate(&host)
		candidate := fixture
		candidate.host = host
		candidate.write(t)
		if _, err := loadPolicy(fixture.repository, fixture.taskFile, fixture.hostFile); err == nil {
			t.Fatalf("accepted unsafe host policy: %+v", host)
		}
	}
}

func TestPolicyFilesCannotBeInsideRepository(t *testing.T) {
	fixture := newPolicyFixture(t, "audit", nil, nil)
	path := filepath.Join(fixture.repository, "task.json")
	data, err := os.ReadFile(fixture.taskFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPolicy(fixture.repository, path, fixture.hostFile); err == nil {
		t.Fatal("accepted repository-controlled task policy")
	}
}

func TestLimitedEnvironmentDropsCredentialVariables(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("ARGUS_GITHUB_WRITE_TOKEN", "secret")
	environment := limitedEnvironment(t.TempDir(), filepath.Join(t.TempDir(), "module-cache"))
	for _, entry := range environment {
		if strings.Contains(entry, "secret") || strings.HasPrefix(entry, "GITHUB_TOKEN=") || strings.HasPrefix(entry, "AWS_SECRET_ACCESS_KEY=") {
			t.Fatalf("inherited credential variable: %q", entry)
		}
	}
}

func TestUnsetModuleCacheUsesCurrentGoToolchainCache(t *testing.T) {
	t.Setenv("GOMODCACHE", "")
	moduleCache, err := resolveModuleCache()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(moduleCache) {
		t.Fatalf("module cache is not absolute: %q", moduleCache)
	}
	if !slices.Contains(limitedEnvironment(t.TempDir(), moduleCache), "GOMODCACHE="+moduleCache) {
		t.Fatal("restricted child environment lost the bootstrapped module cache")
	}
}

func TestExecuteProcessRecordsOutputAndFailure(t *testing.T) {
	for _, example := range []struct {
		mode string
		code int
	}{
		{mode: "pass", code: 0},
		{mode: "fail", code: 7},
	} {
		t.Run(example.mode, func(t *testing.T) {
			environment := append(os.Environ(), "AGENT_HARNESS_TEST_HELPER=1")
			result := executeProcess(t.Context(), os.Args[0], []string{"-test.run=^TestHelperProcess$", "--", example.mode}, t.TempDir(), environment)
			if result.exitCode != example.code || result.launchError != "" {
				t.Fatalf("unexpected exit: %+v", result)
			}
			if result.stdout.String() != "stdout\n" || result.stderr.String() != "stderr\n" {
				t.Fatalf("missing process output: %+v", result)
			}
		})
	}
}

func TestExecuteProcessHonorsTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	environment := append(os.Environ(), "AGENT_HARNESS_TEST_HELPER=1")
	result := executeProcess(ctx, os.Args[0], []string{"-test.run=^TestHelperProcess$", "--", "wait"}, t.TempDir(), environment)
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || result.exitCode == 0 {
		t.Fatalf("timeout was not recorded: %+v", result)
	}
}

func TestRunVerificationExecutesNamedMakeTargetAndRecordsFailure(t *testing.T) {
	fixture := newPolicyFixture(t, "implementation", []string{"internal/catalog"}, []string{"verify"})
	t.Setenv("GOMODCACHE", t.TempDir())
	p, err := loadPolicy(fixture.repository, fixture.taskFile, fixture.hostFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, example := range []struct {
		name        string
		recipe      string
		wantSuccess bool
	}{
		{name: "success", recipe: "@echo verified", wantSuccess: true},
		{name: "failure", recipe: "@go no-such-command", wantSuccess: false},
	} {
		t.Run(example.name, func(t *testing.T) {
			makefile := "verify:\n\t" + example.recipe + "\n"
			if err := os.WriteFile(filepath.Join(fixture.repository, "Makefile"), []byte(makefile), 0o600); err != nil {
				t.Fatal(err)
			}
			report, err := runVerification(t.Context(), p, "verify")
			if err != nil {
				t.Fatal(err)
			}
			if (report.ExitCode == 0) != example.wantSuccess || report.LaunchError != "" {
				t.Fatalf("unexpected verification result: %+v", report)
			}
			if example.wantSuccess && !strings.Contains(report.Stdout, "verified") {
				t.Fatalf("successful output was not recorded: %+v", report)
			}
			if !example.wantSuccess && report.Stderr == "" {
				t.Fatalf("failure diagnostics were not recorded: %+v", report)
			}
		})
	}
}

func TestWriteReportRetainsBoundedEvidenceOutsideRepository(t *testing.T) {
	fixture := newPolicyFixture(t, "implementation", []string{"internal/catalog"}, []string{"verify"})
	capture := &boundedCapture{limit: 4}
	for _, chunk := range []string{"123", "456", "789"} {
		if _, err := capture.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if capture.String() != "6789" || capture.total != 9 {
		t.Fatalf("capture limit failed: %+v", capture)
	}
	path, err := writeReport(fixture.scratch, checkReport{TaskID: "M03-catalog", Check: "verify", ExitCode: 7, Stdout: capture.String(), StdoutBytes: capture.total, StdoutTruncated: true})
	if err != nil {
		t.Fatal(err)
	}
	if !within(fixture.scratch, path) || within(fixture.repository, path) {
		t.Fatalf("report escaped scratch root: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report checkReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.ExitCode != 7 || report.Stdout != "6789" || !report.StdoutTruncated {
		t.Fatalf("report lost evidence: %+v", report)
	}
}

func TestFailureOutputReturnsBoundedEvidenceForSelfCorrection(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := printFailureOutput(&stdout, &stderr, checkReport{
		Check:       "verify",
		ExitCode:    1,
		Stdout:      strings.Repeat("x", failureOutputLimit+10) + "useful stdout",
		Stderr:      "compile failed: missing symbol",
		LaunchError: "",
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "useful stdout") || strings.Contains(stdout.String(), strings.Repeat("x", failureOutputLimit+10)) {
		t.Fatalf("stdout was not bounded to its useful tail: %d bytes", stdout.Len())
	}
	if !strings.Contains(stderr.String(), "compile failed: missing symbol") {
		t.Fatalf("stderr was not returned for correction: %s", stderr.String())
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("AGENT_HARNESS_TEST_HELPER") != "1" {
		return
	}
	if _, err := fmt.Fprintln(os.Stdout, "stdout"); err != nil {
		os.Exit(10)
	}
	if _, err := fmt.Fprintln(os.Stderr, "stderr"); err != nil {
		os.Exit(10)
	}
	switch os.Args[len(os.Args)-1] {
	case "pass":
		os.Exit(0)
	case "fail":
		os.Exit(7)
	case "wait":
		time.Sleep(time.Second)
		os.Exit(0)
	default:
		os.Exit(9)
	}
}
