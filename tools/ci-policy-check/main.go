package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed policy.json
var checkedInPolicy []byte

func main() {
	workflowPath := filepath.Join(".github", "workflows", "validate.yml")
	os.Exit(run(workflowPath, os.Stdout, os.Stderr))
}

func run(workflowPath string, stdout, stderr io.Writer) int {
	policy, err := loadPolicy(checkedInPolicy)
	if err != nil {
		return report(stderr, 2, "invalid CI policy: %v\n", err)
	}
	workflowData, err := os.ReadFile(workflowPath)
	if err != nil {
		return report(stderr, 2, "read validation workflow: %v\n", err)
	}
	drift, err := checkWorkflow(policy, workflowData)
	if err != nil {
		return report(stderr, 2, "parse validation workflow: %v\n", err)
	}
	if len(drift) != 0 {
		return report(stderr, 1, "CI policy drift: %s\n", strings.Join(drift, "\nCI policy drift: "))
	}
	if _, err := fmt.Fprintln(stdout, "CI workflow matches policy"); err != nil {
		return report(stderr, 2, "write output: %v\n", err)
	}

	return 0
}

func report(stderr io.Writer, exitCode int, format string, args ...any) int {
	if _, err := fmt.Fprintf(stderr, format, args...); err != nil {
		return 2
	}

	return exitCode
}
