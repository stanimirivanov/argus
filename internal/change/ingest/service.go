// Package ingest coordinates trusted change ingestion without depending on a
// source-control provider or persistence implementation.
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

var (
	// ErrInvalidDelivery means verified transport input cannot identify one
	// immutable pull-request comparison.
	ErrInvalidDelivery = errors.New("invalid change delivery")
	// ErrUnauthenticated means the delivery signature is absent or invalid.
	ErrUnauthenticated = errors.New("change delivery authentication failed")
)

// Delivery is the verified, normalized portion of a provider event needed to
// resolve a change. PayloadSHA256 binds a delivery ID to the exact signed body
// so a repeated ID with different content is an immutable conflict.
type Delivery struct {
	Provider          catalog.Provider
	ID                string
	Event             string
	Action            string
	PayloadSHA256     string
	Repository        catalog.Repository
	PullRequestNumber int
	BaseRevision      catalog.Revision
	HeadRevision      catalog.Revision
	ObservedAt        time.Time
}

// StoredDelivery is the durable result associated with a provider delivery.
type StoredDelivery struct {
	PayloadSHA256 string
	ChangeSet     change.Set
}

// Store owns atomic delivery deduplication and change-set persistence.
type Store interface {
	FindDelivery(context.Context, catalog.Provider, string) (StoredDelivery, error)
	SaveDelivery(context.Context, Delivery, change.Set) (bool, error)
}

// Resolver obtains bounded provider evidence for the immutable revisions named
// by a verified delivery. Implementations must not silently follow a newer
// pull-request head.
type Resolver interface {
	Resolve(context.Context, Delivery) (change.Set, error)
}

// Service makes retries cheap before provider I/O and relies on Store for the
// final atomic race between concurrent first attempts.
type Service struct {
	store    Store
	resolver Resolver
}

// Result reports whether this call durably created the delivery and returns
// the canonical persisted change set for both first attempts and exact retries.
type Result struct {
	Created   bool
	ChangeSet change.Set
}

// NewService constructs the trusted ingestion use case.
func NewService(store Store, resolver Resolver) *Service {
	return &Service{store: store, resolver: resolver}
}

// Ingest resolves and stores one verified delivery. Exact retries return the
// original result without another provider request; reused IDs with a different
// signed body return change.ErrConflict.
func (service *Service) Ingest(ctx context.Context, delivery Delivery) (Result, error) {
	if service == nil || service.store == nil || service.resolver == nil {
		return Result{}, change.ErrUnavailable
	}
	if err := ValidateDelivery(delivery); err != nil {
		return Result{}, err
	}

	existing, err := service.store.FindDelivery(ctx, delivery.Provider, delivery.ID)
	if err == nil {
		if existing.PayloadSHA256 != delivery.PayloadSHA256 {
			return Result{}, change.ErrConflict
		}

		return Result{ChangeSet: change.CanonicalSet(existing.ChangeSet)}, nil
	}
	if !errors.Is(err, change.ErrNotFound) {
		return Result{}, err
	}

	resolved, err := service.resolver.Resolve(ctx, delivery)
	if err != nil {
		return Result{}, err
	}
	resolved = change.CanonicalSet(resolved)
	if err := ValidateResolvedDelivery(delivery, resolved); err != nil {
		return Result{}, err
	}

	created, err := service.store.SaveDelivery(ctx, delivery, resolved)
	if err != nil {
		return Result{}, err
	}
	if !created {
		existing, err = service.store.FindDelivery(ctx, delivery.Provider, delivery.ID)
		if err != nil {
			return Result{}, err
		}
		if existing.PayloadSHA256 != delivery.PayloadSHA256 {
			return Result{}, change.ErrConflict
		}
		resolved = change.CanonicalSet(existing.ChangeSet)
	}

	return Result{Created: created, ChangeSet: resolved}, nil
}

// ValidateDelivery enforces provider-independent invariants after a driving
// adapter has authenticated and normalized the external event.
func ValidateDelivery(delivery Delivery) error {
	if delivery.Provider != catalog.ProviderGitHub || delivery.Event != "pull_request" ||
		strings.TrimSpace(delivery.ID) == "" || len(delivery.ID) > 255 ||
		len(delivery.PayloadSHA256) != sha256.Size*2 {
		return fmt.Errorf("%w: identity", ErrInvalidDelivery)
	}
	if _, err := hex.DecodeString(delivery.PayloadSHA256); err != nil || delivery.PayloadSHA256 != strings.ToLower(delivery.PayloadSHA256) {
		return fmt.Errorf("%w: payload digest", ErrInvalidDelivery)
	}
	if delivery.PullRequestNumber < 1 || delivery.ObservedAt.IsZero() || !delivery.ObservedAt.Equal(delivery.ObservedAt.UTC()) {
		return fmt.Errorf("%w: pull request metadata", ErrInvalidDelivery)
	}
	if err := change.ValidateSet(change.Set{
		APIVersion:        change.SetAPIVersion,
		SourceRepository:  delivery.Repository,
		PullRequestNumber: delivery.PullRequestNumber,
		BaseRevision:      delivery.BaseRevision,
		HeadRevision:      delivery.HeadRevision,
		ObservedAt:        delivery.ObservedAt,
		Trigger: change.Trigger{
			Provider:   delivery.Provider,
			DeliveryID: delivery.ID,
			Event:      delivery.Event,
			Action:     delivery.Action,
		},
	}); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDelivery, err)
	}

	return nil
}

// ValidateResolvedDelivery prevents a provider adapter from returning evidence
// for a different repository, pull request, revision pair, or trigger.
func ValidateResolvedDelivery(delivery Delivery, set change.Set) error {
	if err := change.ValidateSet(set); err != nil {
		return err
	}
	wantTrigger := change.Trigger{
		Provider:   delivery.Provider,
		DeliveryID: delivery.ID,
		Event:      delivery.Event,
		Action:     delivery.Action,
	}
	if set.SourceRepository != delivery.Repository ||
		set.PullRequestNumber != delivery.PullRequestNumber ||
		set.BaseRevision != delivery.BaseRevision ||
		set.HeadRevision != delivery.HeadRevision ||
		set.ObservedAt != delivery.ObservedAt ||
		set.Trigger != wantTrigger {
		return fmt.Errorf("%w: resolved evidence does not match delivery", change.ErrStale)
	}

	return nil
}
