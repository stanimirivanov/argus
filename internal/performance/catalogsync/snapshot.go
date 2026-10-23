// Package catalogsync owns immutable authorized performance catalog evidence.
// Vendor validation and transports belong to adapters; this package owns the
// persistence capability and freshness/identity vocabulary, never launch authority.
package catalogsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

var (
	// ErrInvalid denotes malformed inward evidence.
	ErrInvalid = errors.New("invalid performance catalog evidence")
	// ErrConflict denotes changed evidence under an existing scoped source ID.
	ErrConflict = errors.New("performance catalog identity conflict")
	// ErrOlder denotes a new ID that cannot advance the current generation.
	ErrOlder = errors.New("performance catalog generation is not newer")
	// ErrExpired denotes publication outside the source freshness window.
	ErrExpired = errors.New("performance catalog is stale or future-dated")
	// ErrNotFound denotes absent evidence in the requested authority scope.
	ErrNotFound = errors.New("performance catalog not found")
	// ErrUnavailable hides database details, including constraint failure messages.
	ErrUnavailable = errors.New("performance catalog database unavailable")
)

// Scope binds evidence to its provider and authenticated principal, not a label.
type Scope struct{ Issuer, PrincipalIssuer, PrincipalID string }

// Resource preserves one current issuer-scoped reference and lifecycle state.
type Resource struct{ Kind, Issuer, ID, Revision, SHA256, State string }

// Evidence is the v1 fingerprint encoding: field names and meaning are durable.
// Pages is opaque, independently versioned source evidence validated by an adapter.
// Resources are its relational identity projection, not execution eligibility.
type Evidence struct {
	Scope                                     Scope
	ID, Revision, ContentSHA256, SchemaSHA256 string
	GeneratedAt, ValidUntil                   time.Time
	Pages                                     json.RawMessage
	Resources                                 []Resource
}

// Snapshot owns immutable canonical evidence; its zero value is invalid.
type Snapshot struct {
	data        Evidence
	fingerprint string
}

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// NewSnapshot validates inward invariants and takes an owned canonical copy.
// Generic JSON normalization makes the fingerprint stable across lexical retries;
// the pinned v1 source permits only bounded integer counts (below float64 limits).
func NewSnapshot(data Evidence) (Snapshot, error) {
	data.GeneratedAt, data.ValidUntil = data.GeneratedAt.UTC().Truncate(time.Microsecond), data.ValidUntil.UTC().Truncate(time.Microsecond)
	for _, value := range []string{data.Scope.Issuer, data.Scope.PrincipalIssuer, data.Scope.PrincipalID, data.ID, data.Revision} {
		if strings.TrimSpace(value) == "" || strings.ContainsRune(value, 0) {
			return Snapshot{}, ErrInvalid
		}
	}
	if data.GeneratedAt.IsZero() || !data.GeneratedAt.Before(data.ValidUntil) || !digestPattern.MatchString(data.ContentSHA256) || !digestPattern.MatchString(data.SchemaSHA256) || len(data.Pages) > 64<<20 || len(data.Resources) > 100000 {
		return Snapshot{}, ErrInvalid
	}
	var pages []any
	if err := json.Unmarshal(data.Pages, &pages); err != nil || len(pages) == 0 || len(pages) > 1000 {
		return Snapshot{}, ErrInvalid
	}
	canonical, err := json.Marshal(pages)
	if err != nil || len(canonical) > 64<<20 {
		return Snapshot{}, ErrInvalid
	}
	data.Pages = canonical
	data.GeneratedAt, data.ValidUntil = data.GeneratedAt.UTC(), data.ValidUntil.UTC()
	data.Resources = append([]Resource{}, data.Resources...)
	sort.Slice(data.Resources, func(i, j int) bool {
		a, b := data.Resources[i], data.Resources[j]
		return a.Kind+"\x00"+a.Issuer+"\x00"+a.ID < b.Kind+"\x00"+b.Issuer+"\x00"+b.ID
	})
	seen := map[[3]string]bool{}
	for _, resource := range data.Resources {
		key := [3]string{resource.Kind, resource.Issuer, resource.ID}
		if seen[key] || !validResource(resource) {
			return Snapshot{}, ErrInvalid
		}
		seen[key] = true
	}
	encoded, err := json.Marshal(struct {
		Version  string
		Evidence Evidence
	}{"argus.performance-catalog/v1", data})
	if err != nil {
		return Snapshot{}, ErrInvalid
	}
	fingerprint := sha256.Sum256(encoded)

	return Snapshot{data: data, fingerprint: hex.EncodeToString(fingerprint[:])}, nil
}

func validResource(resource Resource) bool {
	for _, value := range []string{resource.Issuer, resource.ID, resource.Revision} {
		if strings.ContainsRune(value, 0) {
			return false
		}
	}
	if strings.TrimSpace(resource.Issuer) == "" || strings.TrimSpace(resource.ID) == "" || strings.TrimSpace(resource.Revision) == "" || !digestPattern.MatchString(resource.SHA256) {
		return false
	}
	if resource.Kind == "baseline" {
		return slices.Contains([]string{"CANDIDATE", "QUALIFIED", "APPROVED", "RETIRED"}, resource.State)
	}

	return slices.Contains([]string{"workload", "profile", "environment", "policy", "owner-route"}, resource.Kind) && slices.Contains([]string{"ACTIVE", "RETIRED"}, resource.State)
}

// Evidence returns independent slices; callers cannot mutate the snapshot.
func (snapshot Snapshot) Evidence() Evidence {
	data := snapshot.data
	data.Pages = slices.Clone(data.Pages)
	data.Resources = slices.Clone(data.Resources)
	return data
}

// Fingerprint identifies the complete immutable v1 evidence, including schema pin.
func (snapshot Snapshot) Fingerprint() string { return snapshot.fingerprint }

// Fresh reports an inclusive generation and exclusive expiry window. Historical
// reads remain available after expiry; consumers must check freshness before use.
func (snapshot Snapshot) Fresh(at time.Time) bool {
	return snapshot.fingerprint != "" && !at.IsZero() && !at.Before(snapshot.data.GeneratedAt) && at.Before(snapshot.data.ValidUntil)
}

// Store publishes snapshot, resources and current pointer in one transaction.
// Exact replay returns false and never repoints a newer head. Get/Current retain
// historical evidence without claiming freshness. Implementations recheck the
// database clock at commit and serialize publication within Scope.
type Store interface {
	Publish(context.Context, Snapshot) (bool, error)
	Current(context.Context, Scope) (Snapshot, error)
	Get(context.Context, Scope, string) (Snapshot, error)
}

// Publication reports the original durable representation after an exact retry.
type Publication struct {
	Created  bool
	Snapshot Snapshot
}

// Publish coordinates write/readback through the owned store capability. A read
// failure after successful publication does not imply the transaction rolled back.
func Publish(ctx context.Context, store Store, snapshot Snapshot) (Publication, error) {
	created, err := store.Publish(ctx, snapshot)
	if err != nil {
		return Publication{}, err
	}
	data := snapshot.Evidence()
	persisted, err := store.Get(ctx, data.Scope, data.ID)
	if err != nil {
		return Publication{}, err
	}

	return Publication{Created: created, Snapshot: persisted}, nil
}
