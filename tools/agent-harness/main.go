package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	options, err := parseCLI(args, stderr)
	if err != nil {
		diagnostic(stderr, "agent-harness: %v\n", err)
		return 2
	}
	p, err := loadPolicy(options.repository, options.taskFile, options.hostFile)
	if err != nil {
		diagnostic(stderr, "agent-harness: policy refused: %v\n", err)
		return 2
	}
	if options.command == "inspect" {
		return inspectPolicy(stdout, stderr, p)
	}

	return verifyPolicy(stdout, stderr, p, options.check)
}

type cliOptions struct {
	command    string
	repository string
	taskFile   string
	hostFile   string
	check      string
}

func parseCLI(args []string, stderr io.Writer) (cliOptions, error) {
	if len(args) == 0 || (args[0] != "inspect" && args[0] != "verify") {
		return cliOptions{}, errors.New("usage: agent-harness inspect|verify --repo DIR --task FILE --host-state FILE [--check TARGET]")
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	repository := flags.String("repo", "", "absolute repository root")
	taskFile := flags.String("task", "", "task policy JSON outside repository")
	hostFile := flags.String("host-state", "", "host scope declaration JSON outside repository")
	check := flags.String("check", "", "allowed verification Make target")
	if err := flags.Parse(args[1:]); err != nil {
		return cliOptions{}, err
	}
	if flags.NArg() != 0 || *repository == "" || *taskFile == "" || *hostFile == "" {
		return cliOptions{}, errors.New("--repo, --task, and --host-state are required; positional arguments are forbidden")
	}
	if (command == "verify" && *check == "") || (command == "inspect" && *check != "") {
		return cliOptions{}, errors.New("--check is required only for verify")
	}

	return cliOptions{command, *repository, *taskFile, *hostFile, *check}, nil
}

func inspectPolicy(stdout, stderr io.Writer, p policy) int {
	summary := struct {
		TaskID        string   `json:"taskId"`
		Profile       string   `json:"profile"`
		Repository    string   `json:"repository"`
		WritableRoots []string `json:"writableRoots"`
		Network       string   `json:"network"`
		Credentials   string   `json:"credentials"`
		Checks        []string `json:"checks"`
	}{p.task.TaskID, p.task.Profile, p.repository, p.writableRoots, p.task.Network, p.task.Credentials, p.task.Checks}
	if err := json.NewEncoder(stdout).Encode(summary); err != nil {
		diagnostic(stderr, "agent-harness: output: %v\n", err)
		return 2
	}

	return 0
}

func verifyPolicy(stdout, stderr io.Writer, p policy, check string) int {
	report, err := runVerification(context.Background(), p, check)
	if err != nil {
		diagnostic(stderr, "agent-harness: verification refused: %v\n", err)
		return 2
	}
	path, err := writeReport(p.scratchRoot, report)
	if err != nil {
		diagnostic(stderr, "agent-harness: failed to record verification: %v\n", err)
		return 2
	}
	if _, err := fmt.Fprintf(stdout, "check=%s exit=%d timedOut=%t report=%s\n", report.Check, report.ExitCode, report.TimedOut, path); err != nil {
		return 2
	}
	if report.ExitCode != 0 || report.TimedOut {
		if err := printFailureOutput(stdout, stderr, report); err != nil {
			return 2
		}

		return 1
	}

	return 0
}

const failureOutputLimit = 32 << 10

func printFailureOutput(stdout, stderr io.Writer, report checkReport) error {
	if report.Stdout != "" {
		if _, err := fmt.Fprintf(stdout, "--- %s stdout (last %d bytes) ---\n%s\n", report.Check, failureOutputLimit, tail(report.Stdout, failureOutputLimit)); err != nil {
			return err
		}
	}
	if report.Stderr != "" {
		if _, err := fmt.Fprintf(stderr, "--- %s stderr (last %d bytes) ---\n%s\n", report.Check, failureOutputLimit, tail(report.Stderr, failureOutputLimit)); err != nil {
			return err
		}
	}
	if report.LaunchError != "" {
		if _, err := fmt.Fprintf(stderr, "--- %s launch error ---\n%s\n", report.Check, report.LaunchError); err != nil {
			return err
		}
	}

	return nil
}

func tail(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[len(value)-limit:]
}

func diagnostic(writer io.Writer, format string, arguments ...any) {
	if _, err := fmt.Fprintf(writer, format, arguments...); err != nil {
		// The diagnostic channel itself failed; the caller already returns a
		// nonzero status and there is no reliable secondary channel.
		return
	}
}
