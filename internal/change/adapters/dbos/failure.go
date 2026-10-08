package dbos

import (
	"errors"
	"fmt"

	dbosgo "github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

// DBOS checkpoints ordinary Go sentinel errors as plain text, which loses
// errors.Is classification after recovery. Its registered portable envelope
// gives these known failure classes a small, stable representation instead.
const failureName = "argus-change-failure-v1"

type failureCode string

const (
	failureInvalidDelivery failureCode = "invalid-delivery"
	failureInvalid         failureCode = "invalid"
	failureConflict        failureCode = "conflict"
	failureStale           failureCode = "stale"
	failureNotFound        failureCode = "not-found"
	failureUnavailable     failureCode = "unavailable"
)

func preserveFailureClass(err error) error {
	if err == nil {
		return nil
	}

	var code failureCode
	switch {
	case errors.Is(err, ingest.ErrInvalidDelivery):
		code = failureInvalidDelivery
	case errors.Is(err, change.ErrInvalid):
		code = failureInvalid
	case errors.Is(err, change.ErrConflict):
		code = failureConflict
	case errors.Is(err, change.ErrStale):
		code = failureStale
	case errors.Is(err, change.ErrNotFound):
		code = failureNotFound
	case errors.Is(err, change.ErrUnavailable):
		code = failureUnavailable
	default:
		// Unclassified infrastructure errors remain generic. Never infer a
		// domain category from their text, even if it resembles a sentinel.
		return err
	}

	return &dbosgo.PortableWorkflowError{
		Name: failureName, Code: string(code), Message: "change workflow failed",
	}
}

func restoreFailureClass(err error) error {
	var portable *dbosgo.PortableWorkflowError
	if !errors.As(err, &portable) || portable.Name != failureName {
		return err
	}

	code, ok := portable.Code.(string)
	if !ok {
		return err
	}

	var category error
	switch failureCode(code) {
	case failureInvalidDelivery:
		category = ingest.ErrInvalidDelivery
	case failureInvalid:
		category = change.ErrInvalid
	case failureConflict:
		category = change.ErrConflict
	case failureStale:
		category = change.ErrStale
	case failureNotFound:
		category = change.ErrNotFound
	case failureUnavailable:
		category = change.ErrUnavailable
	default:
		return err
	}

	return fmt.Errorf("%w: %w", category, err)
}
