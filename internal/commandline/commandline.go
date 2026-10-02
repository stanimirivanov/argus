// Package commandline defines the small, machine-facing command protocol used
// by Argus executables. It owns metadata output and exit classification, not
// capability argument parsing or application behavior.
package commandline

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
)

// Exit codes are part of the executable boundary: zero is success or metadata,
// one is an operational failure, and two is invalid command usage.
const (
	ExitSuccess = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// ErrUsage classifies invalid command syntax without relying on error text.
var ErrUsage = errors.New("invalid command usage")

type usageError struct{ cause error }

func (err usageError) Error() string        { return err.cause.Error() }
func (err usageError) Unwrap() error        { return err.cause }
func (err usageError) Is(target error) bool { return target == ErrUsage }

// Usage wraps an argument error while preserving its diagnostic text and cause.
func Usage(err error) error {
	if err == nil {
		return nil
	}

	return usageError{cause: err}
}

// UsageText classifies a missing or invalid positional argument without
// changing the human-readable usage line.
func UsageText(message string) error { return Usage(errors.New(message)) }

// Parse preserves flag.Parse diagnostics and marks syntax failures as usage.
func Parse(flags *flag.FlagSet, args []string) error { return Usage(flags.Parse(args)) }

// Spec describes one executable's top-level invocation and operational role.
type Spec struct {
	Name     string
	Synopsis string
	Role     string
}

// HandleMeta handles a leading help or version option before configuration,
// credentials, database access, or adapter construction. Only a standalone
// leading metadata option is recognized; arguments after -- remain untouched.
func HandleMeta(args []string, stdout, stderr io.Writer, spec Spec) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	if !isHelp(args[0]) && !isVersion(args[0]) {
		return 0, false
	}
	if len(args) != 1 {
		return Report(stderr, UsageText("help and version options must be used alone")), true
	}
	if isHelp(args[0]) {
		if _, err := fmt.Fprintf(stdout, "usage: %s\nrole: %s\noptions: -h, --help; -version, --version\n", spec.Synopsis, spec.Role); err != nil {
			return Report(stderr, err), true
		}

		return ExitSuccess, true
	}
	if _, err := fmt.Fprintln(stdout, versionLine(spec.Name)); err != nil {
		return Report(stderr, err), true
	}

	return ExitSuccess, true
}

// Report writes one error and returns its stable exit classification.
func Report(stderr io.Writer, err error) int {
	if err == nil {
		return ExitSuccess
	}
	if _, writeErr := fmt.Fprintln(stderr, err); writeErr != nil {
		return ExitFailure
	}
	if errors.Is(err, ErrUsage) {
		return ExitUsage
	}

	return ExitFailure
}

func isHelp(value string) bool    { return value == "-h" || value == "--help" }
func isVersion(value string) bool { return value == "-version" || value == "--version" }

func versionLine(name string) string {
	version := "devel"
	revision := "unknown"
	modified := false
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
	}
	if modified {
		revision += "+modified"
	}

	return strings.Join([]string{name, version, "revision", revision}, " ")
}
