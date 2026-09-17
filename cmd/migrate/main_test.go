package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRequiresDatabaseURL(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := run(context.Background(), "", &output)
	if err == nil || !strings.Contains(err.Error(), "ARGUS_DATABASE_URL") {
		t.Fatalf("run error = %v, want missing database configuration", err)
	}
	if output.Len() != 0 {
		t.Fatalf("failed command wrote output: %q", output.String())
	}
}
