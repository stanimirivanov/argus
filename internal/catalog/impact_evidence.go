package catalog

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	// ImpactEvidenceBundleAPIVersion is the domain-supported evidence bundle
	// major. Transport packages define their own matching boundary constant.
	ImpactEvidenceBundleAPIVersion = "argus.dev/impact-evidence-bundle/v1"
	// MaxImpactEvidenceObservations bounds one atomic evidence ingestion.
	MaxImpactEvidenceObservations = 10_000
	// DefaultImpactEdgePageSize bounds ordinary impact-edge queries.
	DefaultImpactEdgePageSize = 50
	// MaxImpactEdgePageSize prevents one query from materializing an unbounded
	// relationship graph.
	MaxImpactEdgePageSize = 200
)

// ImpactAssertion describes whether evidence supports or refutes an edge.
type ImpactAssertion string

const (
	// ImpactAssertionSupports asserts that a test exercises a capability.
	ImpactAssertionSupports ImpactAssertion = "supports"
	// ImpactAssertionRefutes contradicts the asserted relationship.
	ImpactAssertionRefutes ImpactAssertion = "refutes"
)

// ImpactEvidenceType identifies how an observation was produced.
type ImpactEvidenceType string

const (
	// ImpactEvidenceExplicit comes from an owner-maintained declaration.
	ImpactEvidenceExplicit ImpactEvidenceType = "explicit"
	// ImpactEvidenceStatic comes from source or contract analysis.
	ImpactEvidenceStatic ImpactEvidenceType = "static"
	// ImpactEvidenceDynamic comes from runtime coverage or tracing.
	ImpactEvidenceDynamic ImpactEvidenceType = "dynamic"
	// ImpactEvidenceHistorical comes from prior changes and executions.
	ImpactEvidenceHistorical ImpactEvidenceType = "historical"
	// ImpactEvidenceReviewerConfirmed records an explicit human conclusion.
	ImpactEvidenceReviewerConfirmed ImpactEvidenceType = "reviewer-confirmed"
)

// ImpactEvidenceProducer identifies the immutable producer revision and
// adapter responsible for an evidence bundle.
type ImpactEvidenceProducer struct {
	Repository Repository
	Revision   Revision
	Adapter    string
}

// ImpactObservation is one immutable assertion about a capability-to-test
// relation. ConfidenceBasisPoints remains producer-specific evidence metadata;
// the catalog never treats it as a universal selection threshold.
type ImpactObservation struct {
	Key                   string
	CapabilityKey         string
	Test                  TestCatalogIdentity
	Assertion             ImpactAssertion
	EvidenceType          ImpactEvidenceType
	ConfidenceBasisPoints int
	Rationale             string
}

// ImpactEvidenceBundle contains one producer's observations for an immutable
// catalog snapshot. ExpiresAt is nil when the producer declares no expiry.
type ImpactEvidenceBundle struct {
	APIVersion   string
	Snapshot     SnapshotReference
	Producer     ImpactEvidenceProducer
	ObservedAt   time.Time
	ExpiresAt    *time.Time
	Observations []ImpactObservation
}

// ImpactEvidenceBundleKey is the immutable retry identity for a bundle.
type ImpactEvidenceBundleKey struct {
	Snapshot           SnapshotKey
	ProducerRepository RepositoryIdentity
	ProducerRevision   Revision
	ProducerAdapter    string
	APIVersion         string
}

// Key returns the identity used to distinguish exact retries from conflicts.
func (bundle ImpactEvidenceBundle) Key() ImpactEvidenceBundleKey {
	return ImpactEvidenceBundleKey{
		Snapshot:           bundle.Snapshot.Key(),
		ProducerRepository: bundle.Producer.Repository.Identity,
		ProducerRevision:   bundle.Producer.Revision,
		ProducerAdapter:    bundle.Producer.Adapter,
		APIVersion:         bundle.APIVersion,
	}
}

// CanonicalImpactEvidenceBundle returns a deep copy with observations ordered
// by their stable key. Input order does not affect immutable retry identity.
func CanonicalImpactEvidenceBundle(bundle ImpactEvidenceBundle) ImpactEvidenceBundle {
	canonical := bundle
	canonical.ObservedAt = bundle.ObservedAt.UTC()
	if bundle.ExpiresAt != nil {
		expiresAt := bundle.ExpiresAt.UTC()
		canonical.ExpiresAt = &expiresAt
	}
	canonical.Observations = append([]ImpactObservation(nil), bundle.Observations...)
	slices.SortFunc(canonical.Observations, func(left, right ImpactObservation) int {
		return cmp.Compare(left.Key, right.Key)
	})

	return canonical
}

// ValidateImpactEvidenceBundle enforces semantic invariants that are not
// expressible in the generated JSON Schema.
func ValidateImpactEvidenceBundle(bundle ImpactEvidenceBundle) error {
	if err := validateImpactEvidenceBundleHeader(bundle); err != nil {
		return err
	}

	return validateImpactObservations(bundle.Observations)
}

func validateImpactEvidenceBundleHeader(bundle ImpactEvidenceBundle) error {
	if bundle.APIVersion != ImpactEvidenceBundleAPIVersion {
		return fmt.Errorf("%w: unsupported API version", ErrInvalidEvidence)
	}
	if err := validateSnapshotKey(bundle.Snapshot.Key()); err != nil {
		return fmt.Errorf("%w: snapshot: %w", ErrInvalidEvidence, err)
	}
	if err := validateRepository(bundle.Snapshot.SourceRepository); err != nil {
		return fmt.Errorf("%w: snapshot repository: %w", ErrInvalidEvidence, err)
	}
	if err := validateRepository(bundle.Producer.Repository); err != nil {
		return fmt.Errorf("%w: producer repository: %w", ErrInvalidEvidence, err)
	}
	if _, err := NewRevision(bundle.Producer.Revision.Algorithm, bundle.Producer.Revision.Digest); err != nil {
		return fmt.Errorf("%w: producer revision", ErrInvalidEvidence)
	}
	if strings.TrimSpace(bundle.Producer.Adapter) == "" || len(bundle.Producer.Adapter) > 127 {
		return fmt.Errorf("%w: producer adapter", ErrInvalidEvidence)
	}
	if bundle.ObservedAt.IsZero() || !isUTC(bundle.ObservedAt) {
		return fmt.Errorf("%w: observedAt must be a UTC instant", ErrInvalidEvidence)
	}
	if bundle.ExpiresAt != nil {
		if !isUTC(*bundle.ExpiresAt) || !bundle.ExpiresAt.After(bundle.ObservedAt) {
			return fmt.Errorf("%w: expiresAt must be UTC and after observedAt", ErrInvalidEvidence)
		}
	}
	if len(bundle.Observations) == 0 || len(bundle.Observations) > MaxImpactEvidenceObservations {
		return fmt.Errorf("%w: observation count", ErrInvalidEvidence)
	}

	return nil
}

func validateImpactObservations(observations []ImpactObservation) error {
	keys := make(map[string]struct{}, len(observations))
	edges := make(map[ImpactEdgeIdentity]struct{}, len(observations))
	for _, observation := range observations {
		if err := validateImpactObservation(observation); err != nil {
			return err
		}
		if _, exists := keys[observation.Key]; exists {
			return fmt.Errorf("%w: duplicate observation key %q", ErrInvalidEvidence, observation.Key)
		}
		keys[observation.Key] = struct{}{}
		edge := ImpactEdgeIdentity{CapabilityKey: observation.CapabilityKey, Test: observation.Test}
		if _, exists := edges[edge]; exists {
			return fmt.Errorf("%w: duplicate edge in one bundle", ErrInvalidEvidence)
		}
		edges[edge] = struct{}{}
	}

	return nil
}

func validateImpactObservation(observation ImpactObservation) error {
	if !localKeyPattern.MatchString(observation.Key) ||
		!localKeyPattern.MatchString(observation.CapabilityKey) {
		return fmt.Errorf("%w: observation or capability key", ErrInvalidEvidence)
	}
	if err := validateTestCatalogIdentity(observation.Test); err != nil {
		return fmt.Errorf("%w: test identity", ErrInvalidEvidence)
	}
	switch observation.Assertion {
	case ImpactAssertionSupports, ImpactAssertionRefutes:
	default:
		return fmt.Errorf("%w: assertion", ErrInvalidEvidence)
	}
	switch observation.EvidenceType {
	case ImpactEvidenceExplicit,
		ImpactEvidenceStatic,
		ImpactEvidenceDynamic,
		ImpactEvidenceHistorical,
		ImpactEvidenceReviewerConfirmed:
	default:
		return fmt.Errorf("%w: evidence type", ErrInvalidEvidence)
	}
	if observation.ConfidenceBasisPoints < 0 || observation.ConfidenceBasisPoints > 10_000 {
		return fmt.Errorf("%w: confidence basis points", ErrInvalidEvidence)
	}
	if strings.TrimSpace(observation.Rationale) == "" || len(observation.Rationale) > 1024 {
		return fmt.Errorf("%w: rationale", ErrInvalidEvidence)
	}

	return nil
}

func validateRepository(repository Repository) error {
	if strings.TrimSpace(repository.Owner) == "" || len(repository.Owner) > 255 ||
		strings.TrimSpace(repository.Name) == "" || len(repository.Name) > 255 {
		return fmt.Errorf("invalid repository coordinates")
	}
	identity := TestCatalogIdentity{TestRepository: repository.Identity, SuiteKey: "valid", TestKey: "valid"}
	if err := validateTestCatalogIdentity(identity); err != nil {
		return fmt.Errorf("invalid repository identity")
	}

	return nil
}

func isUTC(value time.Time) bool {
	_, offsetSeconds := value.Zone()

	return offsetSeconds == 0
}

// ImpactEvidenceStore is the write capability consumed by evidence ingestion.
// Implementations must save a complete bundle atomically and distinguish exact
// retries from immutable-content conflicts.
type ImpactEvidenceStore interface {
	SaveImpactEvidence(context.Context, ImpactEvidenceBundle) (bool, error)
}

// ImpactEvidenceService validates and persists immutable evidence bundles.
type ImpactEvidenceService struct {
	store ImpactEvidenceStore
}

// NewImpactEvidenceService constructs the evidence-ingestion use case. store
// must be non-nil.
func NewImpactEvidenceService(store ImpactEvidenceStore) *ImpactEvidenceService {
	return &ImpactEvidenceService{store: store}
}

// Ingest validates, canonicalizes, and atomically persists one bundle. It
// returns false for an exact retry.
func (service *ImpactEvidenceService) Ingest(
	ctx context.Context,
	bundle ImpactEvidenceBundle,
) (bool, error) {
	if err := ValidateImpactEvidenceBundle(bundle); err != nil {
		return false, err
	}

	return service.store.SaveImpactEvidence(ctx, CanonicalImpactEvidenceBundle(bundle))
}

// ImpactEdgeIdentity is the stable ordering and pagination identity of one
// capability-to-test relation.
type ImpactEdgeIdentity struct {
	CapabilityKey string
	Test          TestCatalogIdentity
}

// ImpactEvidence is one visible observation returned by a storage adapter.
type ImpactEvidence struct {
	Producer              ImpactEvidenceProducer
	ObservationKey        string
	Assertion             ImpactAssertion
	EvidenceType          ImpactEvidenceType
	ConfidenceBasisPoints int
	Rationale             string
	ObservedAt            time.Time
	ExpiresAt             *time.Time
}

// ImpactEvidenceState describes whether evidence is active at the query time.
type ImpactEvidenceState string

const (
	// ImpactEvidenceActive is not expired at the evaluation time.
	ImpactEvidenceActive ImpactEvidenceState = "active"
	// ImpactEvidenceExpired reached its producer-declared expiry.
	ImpactEvidenceExpired ImpactEvidenceState = "expired"
)

// EvaluatedImpactEvidence pairs an observation with its derived temporal state.
type EvaluatedImpactEvidence struct {
	ImpactEvidence
	State ImpactEvidenceState
}

// ImpactEdgeStatus is the catalog's deterministic state for an edge at one
// explicit evaluation instant.
type ImpactEdgeStatus string

const (
	// ImpactEdgeSupported has active support and no active refutation.
	ImpactEdgeSupported ImpactEdgeStatus = "supported"
	// ImpactEdgeRefuted has active refutation and no active support.
	ImpactEdgeRefuted ImpactEdgeStatus = "refuted"
	// ImpactEdgeStale has visible evidence but no active observation.
	ImpactEdgeStale ImpactEdgeStatus = "stale"
	// ImpactEdgeConflicting has active support and active refutation.
	ImpactEdgeConflicting ImpactEdgeStatus = "conflicting"
)

// MappingConflict exposes contradictory active evidence without choosing a
// winner.
type MappingConflict struct {
	SupportingEvidenceCount int
	RefutingEvidenceCount   int
}

// ImpactEdge contains stable catalog metadata plus evaluated evidence.
type ImpactEdge struct {
	Capability     Capability
	TestRepository Repository
	SuiteKey       string
	Family         TestFamily
	Adapter        string
	TestKey        string
	TestName       string
	Status         ImpactEdgeStatus
	Evidence       []EvaluatedImpactEvidence
	Conflict       *MappingConflict
}

// Identity returns the edge's stable ordering identity.
func (edge ImpactEdge) Identity() ImpactEdgeIdentity {
	return ImpactEdgeIdentity{
		CapabilityKey: edge.Capability.Key,
		Test: TestCatalogIdentity{
			TestRepository: edge.TestRepository.Identity,
			SuiteKey:       edge.SuiteKey,
			TestKey:        edge.TestKey,
		},
	}
}

// RawImpactEdge is the adapter result before application-owned temporal state
// and conflict policy are evaluated.
type RawImpactEdge struct {
	Capability     Capability
	TestRepository Repository
	SuiteKey       string
	Family         TestFamily
	Adapter        string
	TestKey        string
	TestName       string
	Evidence       []ImpactEvidence
}

// Identity returns the raw edge's stable ordering identity.
func (edge RawImpactEdge) Identity() ImpactEdgeIdentity {
	return ImpactEdgeIdentity{
		CapabilityKey: edge.Capability.Key,
		Test: TestCatalogIdentity{
			TestRepository: edge.TestRepository.Identity,
			SuiteKey:       edge.SuiteKey,
			TestKey:        edge.TestKey,
		},
	}
}

// ImpactEdgeQuery selects one keyset page evaluated at a caller-supplied time.
type ImpactEdgeQuery struct {
	Snapshot      SnapshotKey
	EvaluatedAt   time.Time
	CapabilityKey string
	PageSize      int
	Cursor        string
}

// ImpactEdgePage is the application result before transport conversion.
type ImpactEdgePage struct {
	Snapshot    SnapshotReference
	EvaluatedAt time.Time
	Items       []ImpactEdge
	NextCursor  string
}

// ImpactEdgeReadRequest is the normalized query consumed by storage adapters.
// After is exclusive.
type ImpactEdgeReadRequest struct {
	Snapshot      SnapshotKey
	EvaluatedAt   time.Time
	CapabilityKey string
	Limit         int
	After         *ImpactEdgeIdentity
}

// ImpactEdgeReadPage is the storage-neutral raw evidence page.
type ImpactEdgeReadPage struct {
	Snapshot SnapshotReference
	Items    []RawImpactEdge
	HasMore  bool
}

// ImpactEdgeReader is the query capability consumed by the impact use case.
type ImpactEdgeReader interface {
	ListImpactEdges(context.Context, ImpactEdgeReadRequest) (ImpactEdgeReadPage, error)
}

// ImpactEdgeService validates queries, owns cursor semantics, and evaluates
// raw evidence returned by a consumer-owned reader port.
type ImpactEdgeService struct {
	reader ImpactEdgeReader
}

// NewImpactEdgeService constructs the impact-edge query use case. reader must
// be non-nil.
func NewImpactEdgeService(reader ImpactEdgeReader) *ImpactEdgeService {
	return &ImpactEdgeService{reader: reader}
}

// ValidateImpactEdgeQuery verifies a query without accessing storage.
func ValidateImpactEdgeQuery(query ImpactEdgeQuery) error {
	_, _, err := normalizeImpactEdgeQuery(query)

	return err
}

// List returns a deterministic page whose state is evaluated at query time.
func (service *ImpactEdgeService) List(ctx context.Context, query ImpactEdgeQuery) (ImpactEdgePage, error) {
	normalized, after, err := normalizeImpactEdgeQuery(query)
	if err != nil {
		return ImpactEdgePage{}, err
	}

	raw, err := service.reader.ListImpactEdges(ctx, ImpactEdgeReadRequest{
		Snapshot:      normalized.Snapshot,
		EvaluatedAt:   normalized.EvaluatedAt,
		CapabilityKey: normalized.CapabilityKey,
		Limit:         normalized.PageSize,
		After:         after,
	})
	if err != nil {
		return ImpactEdgePage{}, err
	}
	if !validImpactEdgeReadPage(raw, normalized, after) {
		return ImpactEdgePage{}, ErrUnavailable
	}

	items := make([]ImpactEdge, len(raw.Items))
	for index, item := range raw.Items {
		items[index] = evaluateImpactEdge(item, normalized.EvaluatedAt)
	}

	nextCursor := ""
	if raw.HasMore {
		nextCursor, err = encodeImpactEdgeCursor(normalized, raw.Items[len(raw.Items)-1].Identity())
		if err != nil {
			return ImpactEdgePage{}, err
		}
	}

	return ImpactEdgePage{
		Snapshot:    raw.Snapshot,
		EvaluatedAt: normalized.EvaluatedAt,
		Items:       items,
		NextCursor:  nextCursor,
	}, nil
}

func validImpactEdgeReadPage(
	page ImpactEdgeReadPage,
	query ImpactEdgeQuery,
	after *ImpactEdgeIdentity,
) bool {
	if page.Snapshot.Key() != query.Snapshot || len(page.Items) > query.PageSize ||
		(page.HasMore && len(page.Items) == 0) {
		return false
	}

	var previous ImpactEdgeIdentity
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

func validateReturnedImpactEvidence(evidence ImpactEvidence) error {
	observation := ImpactObservation{
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
	if _, err := NewRevision(evidence.Producer.Revision.Algorithm, evidence.Producer.Revision.Digest); err != nil {
		return err
	}
	if strings.TrimSpace(evidence.Producer.Adapter) == "" || evidence.ObservedAt.IsZero() ||
		!isUTC(evidence.ObservedAt) {
		return ErrInvalidEvidence
	}
	if evidence.ExpiresAt != nil &&
		(!isUTC(*evidence.ExpiresAt) || !evidence.ExpiresAt.After(evidence.ObservedAt)) {
		return ErrInvalidEvidence
	}

	return nil
}

func validTestCatalogIdentityForValidation() TestCatalogIdentity {
	return TestCatalogIdentity{
		TestRepository: RepositoryIdentity{
			Provider:             ProviderOther,
			Host:                 "validation.invalid",
			ProviderRepositoryID: "validation",
		},
		SuiteKey: "valid",
		TestKey:  "valid",
	}
}

func evaluateImpactEdge(raw RawImpactEdge, evaluatedAt time.Time) ImpactEdge {
	result := ImpactEdge{
		Capability:     raw.Capability,
		TestRepository: raw.TestRepository,
		SuiteKey:       raw.SuiteKey,
		Family:         raw.Family,
		Adapter:        raw.Adapter,
		TestKey:        raw.TestKey,
		TestName:       raw.TestName,
		Evidence:       make([]EvaluatedImpactEvidence, len(raw.Evidence)),
	}

	activeSupport := 0
	activeRefutation := 0
	for index, evidence := range raw.Evidence {
		state := ImpactEvidenceActive
		if evidence.ExpiresAt != nil && !evidence.ExpiresAt.After(evaluatedAt) {
			state = ImpactEvidenceExpired
		} else if evidence.Assertion == ImpactAssertionSupports {
			activeSupport++
		} else {
			activeRefutation++
		}
		result.Evidence[index] = EvaluatedImpactEvidence{ImpactEvidence: evidence, State: state}
	}

	switch {
	case activeSupport > 0 && activeRefutation > 0:
		result.Status = ImpactEdgeConflicting
		result.Conflict = &MappingConflict{
			SupportingEvidenceCount: activeSupport,
			RefutingEvidenceCount:   activeRefutation,
		}
	case activeSupport > 0:
		result.Status = ImpactEdgeSupported
	case activeRefutation > 0:
		result.Status = ImpactEdgeRefuted
	default:
		result.Status = ImpactEdgeStale
	}

	return result
}

func compareImpactEdgeIdentity(left, right ImpactEdgeIdentity) int {
	if compared := cmp.Compare(left.CapabilityKey, right.CapabilityKey); compared != 0 {
		return compared
	}

	return compareTestCatalogIdentity(left.Test, right.Test)
}

func validateImpactEdgeIdentity(identity ImpactEdgeIdentity) error {
	if !localKeyPattern.MatchString(identity.CapabilityKey) {
		return ErrInvalidCursor
	}

	return validateTestCatalogIdentity(identity.Test)
}
