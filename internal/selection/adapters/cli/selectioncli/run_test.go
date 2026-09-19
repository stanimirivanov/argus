package selectioncli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunValidatesArgumentsBeforeInfrastructure(t *testing.T) {
	t.Parallel()

	if err := Run(t.Context(), nil, "", &bytes.Buffer{}, nil); err == nil ||
		!strings.Contains(err.Error(), "usage: select") {
		t.Fatalf("missing arguments error = %v", err)
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
