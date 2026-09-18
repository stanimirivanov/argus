// Package impact implements ingestion and query use cases for immutable
// capability-to-test evidence. Its ports are defined beside the capabilities
// that consume them; adapters implement those ports from the outside.
package impact

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

const (
	// EvidenceBundleAPIVersion is the domain-supported evidence bundle
	// major. Transport packages define their own matching boundary constant.
	EvidenceBundleAPIVersion = "argus.dev/impact-evidence-bundle/v1"
	// MaxEvidenceObservations bounds one atomic evidence ingestion.
	MaxEvidenceObservations = 10_000
	// DefaultEdgePageSize bounds ordinary impact-edge queries.
	DefaultEdgePageSize = 50
	// MaxEdgePageSize prevents one query from materializing an unbounded
	// relationship graph.
	MaxEdgePageSize = 200
)

// Assertion describes whether evidence supports or refutes an edge.
type Assertion string

const (
	// AssertionSupports asserts that a test exercises a capability.
	AssertionSupports Assertion = "supports"
	// AssertionRefutes contradicts the asserted relationship.
	AssertionRefutes Assertion = "refutes"
)

// EvidenceType identifies how an observation was produced.
type EvidenceType string

const (
	// EvidenceExplicit comes from an owner-maintained declaration.
	EvidenceExplicit EvidenceType = "explicit"
	// EvidenceStatic comes from source or contract analysis.
	EvidenceStatic EvidenceType = "static"
	// EvidenceDynamic comes from runtime coverage or tracing.
	EvidenceDynamic EvidenceType = "dynamic"
	// EvidenceHistorical comes from prior changes and executions.
	EvidenceHistorical EvidenceType = "historical"
	// EvidenceReviewerConfirmed records an explicit human conclusion.
	EvidenceReviewerConfirmed EvidenceType = "reviewer-confirmed"
)

// EvidenceProducer identifies the immutable producer revision and
// adapter responsible for an evidence bundle.
type EvidenceProducer struct {
	Repository catalog.Repository
	Revision   catalog.Revision
	Adapter    string
}

// Observation is one immutable assertion about a capability-to-test
// relation. ConfidenceBasisPoints remains producer-specific evidence metadata;
// the catalog never treats it as a universal selection threshold.
type Observation struct {
	Key                   string
	CapabilityKey         string
	Test                  catalog.TestIdentity
	Assertion             Assertion
	EvidenceType          EvidenceType
	ConfidenceBasisPoints int
	Rationale             string
}

// EvidenceBundle contains one producer's observations for an immutable
// catalog snapshot. ExpiresAt is nil when the producer declares no expiry.
type EvidenceBundle struct {
	APIVersion   string
	Snapshot     catalog.SnapshotReference
	Producer     EvidenceProducer
	ObservedAt   time.Time
	ExpiresAt    *time.Time
	Observations []Observation
}

// EvidenceBundleKey is the immutable retry identity for a bundle.
type EvidenceBundleKey struct {
	Snapshot           catalog.SnapshotKey
	ProducerRepository catalog.RepositoryIdentity
	ProducerRevision   catalog.Revision
	ProducerAdapter    string
	APIVersion         string
}

// Key returns the identity used to distinguish exact retries from conflicts.
func (bundle EvidenceBundle) Key() EvidenceBundleKey {
	return EvidenceBundleKey{
		Snapshot:           bundle.Snapshot.Key(),
		ProducerRepository: bundle.Producer.Repository.Identity,
		ProducerRevision:   bundle.Producer.Revision,
		ProducerAdapter:    bundle.Producer.Adapter,
		APIVersion:         bundle.APIVersion,
	}
}

// CanonicalEvidenceBundle returns a deep copy with observations ordered
// by their stable key. Input order does not affect immutable retry identity.
func CanonicalEvidenceBundle(bundle EvidenceBundle) EvidenceBundle {
	canonical := bundle
	canonical.ObservedAt = bundle.ObservedAt.UTC()
	if bundle.ExpiresAt != nil {
		expiresAt := bundle.ExpiresAt.UTC()
		canonical.ExpiresAt = &expiresAt
	}
	canonical.Observations = append([]Observation(nil), bundle.Observations...)
	slices.SortFunc(canonical.Observations, func(left, right Observation) int {
		return cmp.Compare(left.Key, right.Key)
	})

	return canonical
}

// ValidateEvidenceBundle enforces semantic invariants that are not
// expressible in the generated JSON Schema.
func ValidateEvidenceBundle(bundle EvidenceBundle) error {
	if err := validateImpactEvidenceBundleHeader(bundle); err != nil {
		return err
	}

	return validateImpactObservations(bundle.Observations)
}

func validateImpactEvidenceBundleHeader(bundle EvidenceBundle) error {
	if bundle.APIVersion != EvidenceBundleAPIVersion {
		return fmt.Errorf("%w: unsupported API version", catalog.ErrInvalidEvidence)
	}
	if err := catalog.ValidateSnapshotKey(bundle.Snapshot.Key()); err != nil {
		return fmt.Errorf("%w: snapshot: %w", catalog.ErrInvalidEvidence, err)
	}
	if err := validateRepository(bundle.Snapshot.SourceRepository); err != nil {
		return fmt.Errorf("%w: snapshot repository: %w", catalog.ErrInvalidEvidence, err)
	}
	if err := validateRepository(bundle.Producer.Repository); err != nil {
		return fmt.Errorf("%w: producer repository: %w", catalog.ErrInvalidEvidence, err)
	}
	if _, err := catalog.NewRevision(bundle.Producer.Revision.Algorithm, bundle.Producer.Revision.Digest); err != nil {
		return fmt.Errorf("%w: producer revision", catalog.ErrInvalidEvidence)
	}
	if strings.TrimSpace(bundle.Producer.Adapter) == "" || len(bundle.Producer.Adapter) > 127 {
		return fmt.Errorf("%w: producer adapter", catalog.ErrInvalidEvidence)
	}
	if bundle.ObservedAt.IsZero() || !isUTC(bundle.ObservedAt) {
		return fmt.Errorf("%w: observedAt must be a UTC instant", catalog.ErrInvalidEvidence)
	}
	if bundle.ExpiresAt != nil {
		if !isUTC(*bundle.ExpiresAt) || !bundle.ExpiresAt.After(bundle.ObservedAt) {
			return fmt.Errorf("%w: expiresAt must be UTC and after observedAt", catalog.ErrInvalidEvidence)
		}
	}
	if len(bundle.Observations) == 0 || len(bundle.Observations) > MaxEvidenceObservations {
		return fmt.Errorf("%w: observation count", catalog.ErrInvalidEvidence)
	}

	return nil
}

func validateImpactObservations(observations []Observation) error {
	keys := make(map[string]struct{}, len(observations))
	edges := make(map[EdgeIdentity]struct{}, len(observations))
	for _, observation := range observations {
		if err := validateImpactObservation(observation); err != nil {
			return err
		}
		if _, exists := keys[observation.Key]; exists {
			return fmt.Errorf("%w: duplicate observation key %q", catalog.ErrInvalidEvidence, observation.Key)
		}
		keys[observation.Key] = struct{}{}
		edge := EdgeIdentity{CapabilityKey: observation.CapabilityKey, Test: observation.Test}
		if _, exists := edges[edge]; exists {
			return fmt.Errorf("%w: duplicate edge in one bundle", catalog.ErrInvalidEvidence)
		}
		edges[edge] = struct{}{}
	}

	return nil
}

func validateImpactObservation(observation Observation) error {
	if !catalog.IsLocalKey(observation.Key) ||
		!catalog.IsLocalKey(observation.CapabilityKey) {
		return fmt.Errorf("%w: observation or capability key", catalog.ErrInvalidEvidence)
	}
	if !observation.Test.Valid() {
		return fmt.Errorf("%w: test identity", catalog.ErrInvalidEvidence)
	}
	switch observation.Assertion {
	case AssertionSupports, AssertionRefutes:
	default:
		return fmt.Errorf("%w: assertion", catalog.ErrInvalidEvidence)
	}
	switch observation.EvidenceType {
	case EvidenceExplicit,
		EvidenceStatic,
		EvidenceDynamic,
		EvidenceHistorical,
		EvidenceReviewerConfirmed:
	default:
		return fmt.Errorf("%w: evidence type", catalog.ErrInvalidEvidence)
	}
	if observation.ConfidenceBasisPoints < 0 || observation.ConfidenceBasisPoints > 10_000 {
		return fmt.Errorf("%w: confidence basis points", catalog.ErrInvalidEvidence)
	}
	if strings.TrimSpace(observation.Rationale) == "" || len(observation.Rationale) > 1024 {
		return fmt.Errorf("%w: rationale", catalog.ErrInvalidEvidence)
	}

	return nil
}

func validateRepository(repository catalog.Repository) error {
	if strings.TrimSpace(repository.Owner) == "" || len(repository.Owner) > 255 ||
		strings.TrimSpace(repository.Name) == "" || len(repository.Name) > 255 {
		return fmt.Errorf("invalid repository coordinates")
	}
	identity := catalog.TestIdentity{TestRepository: repository.Identity, SuiteKey: "valid", TestKey: "valid"}
	if !identity.Valid() {
		return fmt.Errorf("invalid repository identity")
	}

	return nil
}

func isUTC(value time.Time) bool {
	_, offsetSeconds := value.Zone()

	return offsetSeconds == 0
}

// EvidenceStore is the write capability consumed by evidence ingestion.
// Implementations must save a complete bundle atomically and distinguish exact
// retries from immutable-content conflicts.
type EvidenceStore interface {
	SaveImpactEvidence(context.Context, EvidenceBundle) (bool, error)
}

// EvidenceService validates and persists immutable evidence bundles.
type EvidenceService struct {
	store EvidenceStore
}

// NewEvidenceService constructs the evidence-ingestion use case. store
// must be non-nil.
func NewEvidenceService(store EvidenceStore) *EvidenceService {
	return &EvidenceService{store: store}
}

// Ingest validates, canonicalizes, and atomically persists one bundle. It
// returns false for an exact retry.
func (service *EvidenceService) Ingest(
	ctx context.Context,
	bundle EvidenceBundle,
) (bool, error) {
	if err := ValidateEvidenceBundle(bundle); err != nil {
		return false, err
	}

	return service.store.SaveImpactEvidence(ctx, CanonicalEvidenceBundle(bundle))
}

// EdgeIdentity is the stable ordering and pagination identity of one
// capability-to-test relation.
type EdgeIdentity struct {
	CapabilityKey string
	Test          catalog.TestIdentity
}

// Evidence is one visible observation returned by a storage adapter.
type Evidence struct {
	Producer              EvidenceProducer
	ObservationKey        string
	Assertion             Assertion
	EvidenceType          EvidenceType
	ConfidenceBasisPoints int
	Rationale             string
	ObservedAt            time.Time
	ExpiresAt             *time.Time
}

// EvidenceState describes whether evidence is active at the query time.
type EvidenceState string

const (
	// EvidenceActive is not expired at the evaluation time.
	EvidenceActive EvidenceState = "active"
	// EvidenceExpired reached its producer-declared expiry.
	EvidenceExpired EvidenceState = "expired"
)

// EvaluatedEvidence pairs an observation with its derived temporal state.
type EvaluatedEvidence struct {
	Evidence
	State EvidenceState
}

// EdgeStatus is the catalog's deterministic state for an edge at one
// explicit evaluation instant.
type EdgeStatus string

const (
	// EdgeSupported has active support and no active refutation.
	EdgeSupported EdgeStatus = "supported"
	// EdgeRefuted has active refutation and no active support.
	EdgeRefuted EdgeStatus = "refuted"
	// EdgeStale has visible evidence but no active observation.
	EdgeStale EdgeStatus = "stale"
	// EdgeConflicting has active support and active refutation.
	EdgeConflicting EdgeStatus = "conflicting"
)

// MappingConflict exposes contradictory active evidence without choosing a
// winner.
type MappingConflict struct {
	SupportingEvidenceCount int
	RefutingEvidenceCount   int
}

// Edge contains stable catalog metadata plus evaluated evidence.
type Edge struct {
	Capability     catalog.Capability
	TestRepository catalog.Repository
	SuiteKey       string
	Family         catalog.TestFamily
	Adapter        string
	TestKey        string
	TestName       string
	Status         EdgeStatus
	Evidence       []EvaluatedEvidence
	Conflict       *MappingConflict
}

// Identity returns the edge's stable ordering identity.
func (edge Edge) Identity() EdgeIdentity {
	return EdgeIdentity{
		CapabilityKey: edge.Capability.Key,
		Test: catalog.TestIdentity{
			TestRepository: edge.TestRepository.Identity,
			SuiteKey:       edge.SuiteKey,
			TestKey:        edge.TestKey,
		},
	}
}

// RawEdge is the adapter result before application-owned temporal state
// and conflict policy are evaluated.
type RawEdge struct {
	Capability     catalog.Capability
	TestRepository catalog.Repository
	SuiteKey       string
	Family         catalog.TestFamily
	Adapter        string
	TestKey        string
	TestName       string
	Evidence       []Evidence
}

// Identity returns the raw edge's stable ordering identity.
func (edge RawEdge) Identity() EdgeIdentity {
	return EdgeIdentity{
		CapabilityKey: edge.Capability.Key,
		Test: catalog.TestIdentity{
			TestRepository: edge.TestRepository.Identity,
			SuiteKey:       edge.SuiteKey,
			TestKey:        edge.TestKey,
		},
	}
}

// EdgeQuery selects one keyset page evaluated at a caller-supplied time.
type EdgeQuery struct {
	Snapshot      catalog.SnapshotKey
	EvaluatedAt   time.Time
	CapabilityKey string
	PageSize      int
	Cursor        string
}

// EdgePage is the application result before transport conversion.
type EdgePage struct {
	Snapshot    catalog.SnapshotReference
	EvaluatedAt time.Time
	Items       []Edge
	NextCursor  string
}

// EdgeReadRequest is the normalized query consumed by storage adapters.
// After is exclusive.
type EdgeReadRequest struct {
	Snapshot      catalog.SnapshotKey
	EvaluatedAt   time.Time
	CapabilityKey string
	Limit         int
	After         *EdgeIdentity
}

// EdgeReadPage is the storage-neutral raw evidence page.
type EdgeReadPage struct {
	Snapshot catalog.SnapshotReference
	Items    []RawEdge
	HasMore  bool
}

// EdgeReader is the query capability consumed by the impact use case.
type EdgeReader interface {
	ListImpactEdges(context.Context, EdgeReadRequest) (EdgeReadPage, error)
}

// EdgeService validates queries, owns cursor semantics, and evaluates
// raw evidence returned by a consumer-owned reader port.
type EdgeService struct {
	reader EdgeReader
}

// NewEdgeService constructs the impact-edge query use case. reader must
// be non-nil.
func NewEdgeService(reader EdgeReader) *EdgeService {
	return &EdgeService{reader: reader}
}

// ValidateEdgeQuery verifies a query without accessing storage.
func ValidateEdgeQuery(query EdgeQuery) error {
	_, _, err := normalizeImpactEdgeQuery(query)

	return err
}

// List returns a deterministic page whose state is evaluated at query time.
func (service *EdgeService) List(ctx context.Context, query EdgeQuery) (EdgePage, error) {
	normalized, after, err := normalizeImpactEdgeQuery(query)
	if err != nil {
		return EdgePage{}, err
	}

	raw, err := service.reader.ListImpactEdges(ctx, EdgeReadRequest{
		Snapshot:      normalized.Snapshot,
		EvaluatedAt:   normalized.EvaluatedAt,
		CapabilityKey: normalized.CapabilityKey,
		Limit:         normalized.PageSize,
		After:         after,
	})
	if err != nil {
		return EdgePage{}, err
	}
	if !validImpactEdgeReadPage(raw, normalized, after) {
		return EdgePage{}, catalog.ErrUnavailable
	}

	items := make([]Edge, len(raw.Items))
	for index, item := range raw.Items {
		items[index] = evaluateImpactEdge(item, normalized.EvaluatedAt)
	}

	nextCursor := ""
	if raw.HasMore {
		nextCursor, err = encodeImpactEdgeCursor(normalized, raw.Items[len(raw.Items)-1].Identity())
		if err != nil {
			return EdgePage{}, err
		}
	}

	return EdgePage{
		Snapshot:    raw.Snapshot,
		EvaluatedAt: normalized.EvaluatedAt,
		Items:       items,
		NextCursor:  nextCursor,
	}, nil
}

func validImpactEdgeReadPage(
	page EdgeReadPage,
	query EdgeQuery,
	after *EdgeIdentity,
) bool {
	if page.Snapshot.Key() != query.Snapshot || len(page.Items) > query.PageSize ||
		(page.HasMore && len(page.Items) == 0) {
		return false
	}

	var previous EdgeIdentity
	hasPrevious := false
	if after != nil {
		previous = *after
		hasPrevious = true
	}
	for _, item := range page.Items {
		identity := item.Identity()
		if validateImpactEdgeIdentity(identity) != nil || len(item.Evidence) == 0 ||
			(hasPrevious && compareImpactEdgeIdentity(previous, identity) >= 0) {
			return false
		}
		for _, evidence := range item.Evidence {
			if evidence.ObservedAt.After(query.EvaluatedAt) || validateReturnedImpactEvidence(evidence) != nil {
				return false
			}
		}
		previous = identity
		hasPrevious = true
	}

	return true
}

func validateReturnedImpactEvidence(evidence Evidence) error {
	observation := Observation{
		Key:                   evidence.ObservationKey,
		CapabilityKey:         "valid",
		Test:                  validTestCatalogIdentityForValidation(),
		Assertion:             evidence.Assertion,
		EvidenceType:          evidence.EvidenceType,
		ConfidenceBasisPoints: evidence.ConfidenceBasisPoints,
		Rationale:             evidence.Rationale,
	}
	if err := validateImpactObservation(observation); err != nil {
		return err
	}
	if err := validateRepository(evidence.Producer.Repository); err != nil {
		return err
	}
	if _, err := catalog.NewRevision(evidence.Producer.Revision.Algorithm, evidence.Producer.Revision.Digest); err != nil {
		return err
	}
	if strings.TrimSpace(evidence.Producer.Adapter) == "" || evidence.ObservedAt.IsZero() ||
		!isUTC(evidence.ObservedAt) {
		return catalog.ErrInvalidEvidence
	}
	if evidence.ExpiresAt != nil &&
		(!isUTC(*evidence.ExpiresAt) || !evidence.ExpiresAt.After(evidence.ObservedAt)) {
		return catalog.ErrInvalidEvidence
	}

	return nil
}

func validTestCatalogIdentityForValidation() catalog.TestIdentity {
	return catalog.TestIdentity{
		TestRepository: catalog.RepositoryIdentity{
			Provider:             catalog.ProviderOther,
			Host:                 "validation.invalid",
			ProviderRepositoryID: "validation",
		},
		SuiteKey: "valid",
		TestKey:  "valid",
	}
}

func evaluateImpactEdge(raw RawEdge, evaluatedAt time.Time) Edge {
	result := Edge{
		Capability:     raw.Capability,
		TestRepository: raw.TestRepository,
		SuiteKey:       raw.SuiteKey,
		Family:         raw.Family,
		Adapter:        raw.Adapter,
		TestKey:        raw.TestKey,
		TestName:       raw.TestName,
		Evidence:       make([]EvaluatedEvidence, len(raw.Evidence)),
	}

	activeSupport := 0
	activeRefutation := 0
	for index, evidence := range raw.Evidence {
		state := EvidenceActive
		if evidence.ExpiresAt != nil && !evidence.ExpiresAt.After(evaluatedAt) {
			state = EvidenceExpired
		} else if evidence.Assertion == AssertionSupports {
			activeSupport++
		} else {
			activeRefutation++
		}
		result.Evidence[index] = EvaluatedEvidence{Evidence: evidence, State: state}
	}

	switch {
	case activeSupport > 0 && activeRefutation > 0:
		result.Status = EdgeConflicting
		result.Conflict = &MappingConflict{
			SupportingEvidenceCount: activeSupport,
			RefutingEvidenceCount:   activeRefutation,
		}
	case activeSupport > 0:
		result.Status = EdgeSupported
	case activeRefutation > 0:
		result.Status = EdgeRefuted
	default:
		result.Status = EdgeStale
	}

	return result
}

func compareImpactEdgeIdentity(left, right EdgeIdentity) int {
	if compared := cmp.Compare(left.CapabilityKey, right.CapabilityKey); compared != 0 {
		return compared
	}

	return catalog.CompareTestIdentities(left.Test, right.Test)
}

func validateImpactEdgeIdentity(identity EdgeIdentity) error {
	if !catalog.IsLocalKey(identity.CapabilityKey) {
		return catalog.ErrInvalidCursor
	}
	if !identity.Test.Valid() {
		return catalog.ErrInvalidCursor
	}

	return nil
}
