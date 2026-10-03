package commandline_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

var commandNames = []string{
	"adaptation-evidence",
	"capture-functional-api-review-outcome",
	"catalog",
	"control-plane",
	"descriptor",
	"execution-evidence",
	"migrate",
	"open-functional-api-repair-pr",
	"plan-functional-api",
	"playwright-catalog",
	"propose-functional-api-repair",
	"run-functional-api",
	"select",
	"validate-functional-api-repair",
}

// TestExecutableMetadataAndExitCodes exercises actual command roots, including
// Windows binaries, without a configured database, token, or adapter.
func TestExecutableMetadataAndExitCodes(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	assertCommandCoverage(t, repositoryRoot)
	binDirectory := buildCommandBinaries(t, repositoryRoot)
	testMetadata(t, binDirectory)
	testExitCodes(t, binDirectory)
}

func assertCommandCoverage(t *testing.T, repositoryRoot string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repositoryRoot, "cmd"))
	if err != nil {
		t.Fatalf("list command roots: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() && !slices.Contains(commandNames, entry.Name()) {
			t.Errorf("command %s has no metadata smoke coverage", entry.Name())
		}
	}
}

func buildCommandBinaries(t *testing.T, repositoryRoot string) string {
	t.Helper()
	binDirectory := t.TempDir()
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binDirectory, "./cmd/...")
	build.Dir = repositoryRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build command binaries: %v\n%s", err, output)
	}

	return binDirectory
}

func testMetadata(t *testing.T, binDirectory string) {
	t.Helper()
	for _, name := range commandNames {
		t.Run(name, func(t *testing.T) {
			binary := commandBinary(binDirectory, name)
			for _, option := range []string{"--help", "--version"} {
				output, code := runBinary(t, binary, option)
				if code != 0 {
					t.Fatalf("%s %s exit = %d, output = %q", name, option, code, output)
				}
				if option == "--help" && (!strings.HasPrefix(output, "usage: "+name) || !strings.Contains(output, "role: ")) {
					t.Fatalf("%s help output = %q", name, output)
				}
				if option == "--version" && (!strings.HasPrefix(output, name+" ") || !strings.Contains(output, " revision ")) {
					t.Fatalf("%s version output = %q", name, output)
				}
			}
		})
	}
}

func testExitCodes(t *testing.T, binDirectory string) {
	t.Helper()
	for _, test := range []struct {
		name string
		args []string
		code int
	}{
		{name: "control-plane", args: []string{"unexpected"}, code: 2},
		{name: "migrate", args: []string{"unexpected"}, code: 2},
		{name: "migrate", code: 1},
		{name: "descriptor", code: 2},
		{name: "descriptor", args: []string{"--unknown"}, code: 2},
		{name: "playwright-catalog", code: 2},
		{name: "playwright-catalog", args: []string{"--unknown"}, code: 2},
	} {
		output, code := runBinary(t, commandBinary(binDirectory, test.name), test.args...)
		if code != test.code {
			t.Errorf("%s %v exit = %d, want %d; output = %q", test.name, test.args, code, test.code, output)
		}
	}
}

func commandBinary(binDirectory, name string) string {
	binary := filepath.Join(binDirectory, name)
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	return binary
}

func runBinary(t *testing.T, binary string, args ...string) (string, int) {
	t.Helper()
	command := exec.CommandContext(t.Context(), binary, args...)
	command.Env = withoutArgusEnvironment(os.Environ())
	output, err := command.CombinedOutput()
	if err == nil {
		return string(output), 0
	}

	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("run %s: %v", filepath.Base(binary), err)
	}

	return string(output), exitError.ExitCode()
}

func withoutArgusEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		if !strings.HasPrefix(strings.ToUpper(entry), "ARGUS_") {
			filtered = append(filtered, entry)
		}
	}

	return filtered
}
