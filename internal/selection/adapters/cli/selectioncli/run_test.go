package selectioncli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/commandline"
)

func TestRunValidatesArgumentsBeforeInfrastructure(t *testing.T) {
	t.Parallel()

	if err := Run(t.Context(), nil, "", &bytes.Buffer{}, nil); !errors.Is(err, commandline.ErrUsage) ||
		!strings.Contains(err.Error(), "usage: select") {
		t.Fatalf("missing arguments error = %v", err)
	}
	if err := Run(t.Context(), []string{"--unknown"}, "", &bytes.Buffer{}, nil); !errors.Is(err, commandline.ErrUsage) {
		t.Fatalf("unknown flag error = %v, want usage classification", err)
	}
	if err := Run(
		t.Context(),
		[]string{"-delivery-id", "delivery-42"},
		"",
		&bytes.Buffer{},
		nil,
	); err == nil || !strings.Contains(err.Error(), "ARGUS_DATABASE_URL") {
		t.Fatalf("missing database error = %v", err)
	}
}
