package commandline

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestHandleMetaDoesNotInvokeCapabilityWork(t *testing.T) {
	t.Parallel()

	spec := Spec{Name: "migrate", Synopsis: "migrate", Role: "administrator"}
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "short help", args: []string{"-h"}, want: "usage: migrate\nrole: administrator\n"},
		{name: "long help", args: []string{"--help"}, want: "usage: migrate\nrole: administrator\n"},
		{name: "short version", args: []string{"-version"}, want: "migrate "},
		{name: "long version", args: []string{"--version"}, want: "migrate "},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var output, diagnostics bytes.Buffer
			code, handled := HandleMeta(test.args, &output, &diagnostics, spec)
			if !handled || code != ExitSuccess || !strings.HasPrefix(output.String(), test.want) || diagnostics.Len() != 0 {
				t.Fatalf("HandleMeta() = (%d, %t), stdout=%q, stderr=%q", code, handled, output.String(), diagnostics.String())
			}
		})
	}
	var output, diagnostics bytes.Buffer
	if _, handled := HandleMeta([]string{"--", "--help"}, &output, &diagnostics, spec); handled {
		t.Fatal("adapter arguments after -- were intercepted")
	}
	if code, handled := HandleMeta([]string{"--help", "extra"}, &output, &diagnostics, spec); !handled || code != ExitUsage {
		t.Fatalf("metadata with extra arguments = (%d, %t), want usage failure", code, handled)
	}
}

func TestReportSeparatesUsageFromOperationalFailure(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		err  error
		want int
	}{
		{err: Usage(fmt.Errorf("parse flags: %w", errors.New("unknown option"))), want: ExitUsage},
		{err: errors.New("database unavailable"), want: ExitFailure},
		{err: nil, want: ExitSuccess},
	} {
		var diagnostics bytes.Buffer
		if code := Report(&diagnostics, test.err); code != test.want {
			t.Fatalf("Report(%v) = %d, want %d", test.err, code, test.want)
		}
		if test.err != nil && diagnostics.String() != test.err.Error()+"\n" {
			t.Fatalf("diagnostic = %q, want original error text", diagnostics.String())
		}
	}
}
