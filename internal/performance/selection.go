// Package performance owns conservative, explicit performance workload selection.
// Candidates require owner approval and independent provider admission; selection
// never grants execution authority or broadens an incomplete mapping to run-all.
package performance

import (
	"errors"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

// ErrInvalid denotes malformed or ambiguously identified selection input.
var ErrInvalid = errors.New("invalid performance selection input")

// Reference is an exact immutable, issuer-scoped resource version.
type Reference struct{ Issuer, ID, Revision, SHA256 string }

// Identity is an authoritative identity, never a display name.
type Identity struct{ Issuer, ID string }

// Item is the validated inward catalog projection needed by selection.
type Item struct {
	Kind, State, Purpose                               string
	Reference                                          Reference
	OwnerRoute, Workload, Profile, Environment, Policy Reference
	SupportedProfiles                                  []Reference
	Selections                                         []Mapping
}

// Catalog is a complete adapter-validated view, not arbitrary caller JSON.
type Catalog struct {
	Fingerprint             string
	Reference               Reference
	GeneratedAt, ValidUntil time.Time
	Items                   []Item
}

// Deployment binds a particular environment to the exact candidate source/image.
// Nil references and an empty image digest represent explicitly missing evidence.
type Deployment struct {
	Environment    Reference
	Reference      *Reference
	Service        *Identity
	ImageSHA256    string
	SourceRevision catalog.Revision
}

// Mapping is an explicit owner-reviewed target; nil baseline is known absence.
type Mapping struct {
	ID, Capability                         string
	Workload, Profile, Environment, Policy Reference
	Baseline                               *Reference
}

// Policy is trusted reviewed input; its metadata alone is not authentication.
// SHA256 binds the complete original portable policy document.
type Policy struct {
	Repository                                                catalog.RepositoryIdentity
	HeadRevision                                              catalog.Revision
	SHA256, ImpactSHA256, CatalogSHA256, ReviewID, ReviewedBy string
	ReviewedAt, ExpiresAt                                     time.Time
	HeadCapabilities                                          []string
	AllowedProfiles                                           []Reference
	Deployments                                               []Deployment
	Mappings                                                  []Mapping
}

// Input pairs persisted impact with its canonical public-export digest. The
// boundary adapter computes that digest and validates catalog relationship closure.
type Input struct {
	Impact       change.CapabilityImpact
	ImpactSHA256 string
	Catalog      Catalog
	Policy       Policy
}

// Decision accounts for one reviewed mapping or one uncovered affected capability.
type Decision struct {
	MappingID, Capability, Outcome, Reason string
	Target                                 *Mapping
	OwnerRoute                             *Reference
	Deployment                             *Deployment
}

// Result preserves every omission/review; a single unresolved decision makes the
// overall status review-required. Even candidates always need owner approval.
type Result struct {
	EvaluatedAt, ExpiresAt time.Time
	OwnerApprovalRequired  bool
	Status                 string
	Decisions              []Decision
}

var digest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Select deterministically evaluates exact reviewed mappings at the trusted clock.
// Removed API operations remain affected; only the reviewed head inventory can
// establish whether their capability still exists at the candidate revision.
// Inputs and returned evidence pointers are read-only; callers must not mutate
// them concurrently. Authentication and catalog relationship closure belong to adapters.
func Select(input Input, at time.Time) (Result, error) {
	if err := validateInput(input, at); err != nil {
		return Result{}, err
	}
	impact := change.CanonicalCapabilityImpact(input.Impact)
	complete := completeImpact(impact)
	affected := affectedCapabilities(impact)
	if len(affected) > 5000 {
		return Result{}, ErrInvalid
	}
	items := indexItems(input.Catalog.Items)
	result := Result{EvaluatedAt: at.UTC(), ExpiresAt: minTime(input.Catalog.ValidUntil, input.Policy.ExpiresAt), OwnerApprovalRequired: true, Status: "candidates", Decisions: []Decision{}}
	mappings := slices.Clone(input.Policy.Mappings)
	sort.Slice(mappings, func(i, j int) bool { return mappings[i].ID < mappings[j].ID })
	covered := map[string]bool{}
	for _, mapping := range mappings {
		decision := Decision{MappingID: mapping.ID, Capability: mapping.Capability, Outcome: "omitted", Reason: "capability-unaffected", Target: &mapping}
		if slices.Contains(affected, mapping.Capability) {
			covered[mapping.Capability] = true
			decision = evaluate(input, mapping, items, complete, at)
		}
		result.Decisions = append(result.Decisions, decision)
	}
	for _, capability := range affected {
		if !covered[capability] {
			result.Decisions = append(result.Decisions, Decision{Capability: capability, Outcome: "review-required", Reason: "capability-unmapped"})
		}
	}
	if len(affected) == 0 || !complete {
		result.Decisions = append(result.Decisions, Decision{Outcome: "review-required", Reason: "impact-incomplete"})
	}
	for _, decision := range result.Decisions {
		if decision.Outcome == "review-required" {
			result.Status = "review-required"
		}
	}

	return result, nil
}

type itemKey struct {
	kind string
	ref  Reference
}

func indexItems(source []Item) map[itemKey]Item {
	items := make(map[itemKey]Item, len(source))
	for _, item := range source {
		items[itemKey{item.Kind, item.Reference}] = item
	}

	return items
}

func evaluate(input Input, mapping Mapping, items map[itemKey]Item, complete bool, at time.Time) Decision {
	decision := Decision{MappingID: mapping.ID, Capability: mapping.Capability, Target: &mapping, Outcome: "review-required"}
	decision.Reason = bindingReason(input, mapping, complete, at)
	if decision.Reason != "" {
		return decision
	}
	workload, ok := items[itemKey{"workload", mapping.Workload}]
	if !ok || workload.State != "ACTIVE" {
		decision.Reason = "workload-unavailable"
		return decision
	}
	profile, ok := items[itemKey{"profile", mapping.Profile}]
	if !ok || profile.State != "ACTIVE" || profile.Workload != mapping.Workload || !slices.Contains(workload.SupportedProfiles, mapping.Profile) {
		decision.Reason = "profile-unavailable"
		return decision
	}
	if !slices.ContainsFunc(profile.Selections, func(choice Mapping) bool {
		return choice.Environment == mapping.Environment && choice.Policy == mapping.Policy && sameBaseline(choice.Baseline, mapping.Baseline)
	}) {
		decision.Reason = "selection-unsupported"
		return decision
	}
	if !active(items, "environment", mapping.Environment) || !active(items, "policy", mapping.Policy) || !active(items, "owner-route", workload.OwnerRoute) {
		decision.Reason = "reference-unavailable"
		return decision
	}
	if mapping.Baseline == nil {
		decision.Reason = "baseline-unavailable"
		return decision
	}
	baseline, ok := items[itemKey{"baseline", *mapping.Baseline}]
	if !ok || baseline.State != "APPROVED" || baseline.Workload != mapping.Workload || baseline.Profile != mapping.Profile || baseline.Environment != mapping.Environment || baseline.Policy != mapping.Policy {
		decision.Reason = "baseline-unavailable"
		return decision
	}
	for _, deployment := range input.Policy.Deployments {
		if deployment.Environment != mapping.Environment {
			continue
		}
		if deployment.Reference == nil || !validReference(*deployment.Reference) || deployment.Service == nil || strings.TrimSpace(deployment.Service.Issuer) == "" || strings.TrimSpace(deployment.Service.ID) == "" || !digest.MatchString(deployment.ImageSHA256) || deployment.SourceRevision != input.Impact.Change.HeadRevision {
			break
		}
		decision.Outcome, decision.Reason, decision.OwnerRoute, decision.Deployment = "candidate", "owner-approval-required", &workload.OwnerRoute, &deployment

		return decision
	}
	decision.Reason = "deployment-unbound"

	return decision
}

func bindingReason(input Input, mapping Mapping, complete bool, at time.Time) string {
	p := input.Policy
	switch {
	case !complete:
		return "impact-incomplete"
	case p.Repository != input.Impact.Change.SourceRepository.Identity || p.HeadRevision != input.Impact.Change.HeadRevision || p.ImpactSHA256 != input.ImpactSHA256:
		return "source-mismatch"
	case p.CatalogSHA256 != input.Catalog.Fingerprint:
		return "catalog-changed"
	case at.Before(input.Catalog.GeneratedAt) || !at.Before(input.Catalog.ValidUntil):
		return "catalog-stale"
	case at.Before(p.ReviewedAt) || !at.Before(p.ExpiresAt) || p.ReviewedAt.Before(input.Impact.Change.ObservedAt) || p.ReviewedAt.Before(input.Catalog.GeneratedAt):
		return "policy-stale"
	case !slices.Contains(p.HeadCapabilities, mapping.Capability):
		return "capability-not-at-head"
	case !slices.Contains(p.AllowedProfiles, mapping.Profile):
		return "profile-forbidden"
	}
	for _, other := range p.Mappings {
		if other.ID != mapping.ID && mappingKey(other) == mappingKey(mapping) {
			return "mapping-conflict"
		}
	}

	return ""
}

func completeImpact(impact change.CapabilityImpact) bool {
	if impact.Status != change.ImpactComplete || (impact.APIVersion == change.ImpactAPIVersion && len(impact.Documents) == 0) || (impact.APIVersion == change.CompositeImpactAPIVersion && len(impact.Files) == 0) {
		return false
	}
	for _, file := range impact.Files {
		if len(file.UnmappedPaths) != 0 {
			return false
		}
	}
	for _, document := range impact.Documents {
		if len(document.Operations) == 0 {
			return false
		}
		for _, operation := range document.Operations {
			if len(operation.Capabilities) == 0 {
				return false
			}
		}
	}

	return true
}

func mappingKey(m Mapping) [7]string {
	return [7]string{m.Capability, m.Workload.Issuer, m.Workload.ID, m.Profile.Issuer, m.Profile.ID, m.Environment.Issuer, m.Environment.ID}
}
func sameBaseline(a, b *Reference) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a.UTC()
	}
	return b.UTC()
}
func active(items map[itemKey]Item, kind string, ref Reference) bool {
	item, ok := items[itemKey{kind, ref}]
	return ok && item.State == "ACTIVE"
}

func validReference(ref Reference) bool {
	return strings.TrimSpace(ref.Issuer) != "" && strings.TrimSpace(ref.ID) != "" && strings.TrimSpace(ref.Revision) != "" && digest.MatchString(ref.SHA256)
}

func affectedCapabilities(impact change.CapabilityImpact) []string {
	var affected []string
	for _, document := range impact.Documents {
		for _, operation := range document.Operations {
			affected = append(affected, operation.Capabilities...)
		}
	}
	for _, file := range impact.Files {
		affected = append(affected, file.Capabilities...)
	}
	slices.Sort(affected)

	return slices.Compact(affected)
}

func validateInput(input Input, at time.Time) error {
	p := input.Policy
	if at.IsZero() || change.ValidateCapabilityImpact(input.Impact) != nil || input.Impact.Change.ObservedAt.After(at) ||
		!digest.MatchString(input.ImpactSHA256) || !digest.MatchString(p.SHA256) || !digest.MatchString(p.ImpactSHA256) || !digest.MatchString(p.CatalogSHA256) ||
		strings.TrimSpace(p.ReviewID) == "" || strings.TrimSpace(p.ReviewedBy) == "" || p.ReviewedAt.IsZero() || !p.ReviewedAt.Before(p.ExpiresAt) ||
		!p.Repository.Valid() || !p.HeadRevision.Valid() || !validReference(input.Catalog.Reference) || input.Catalog.GeneratedAt.IsZero() || !input.Catalog.GeneratedAt.Before(input.Catalog.ValidUntil) || !digest.MatchString(input.Catalog.Fingerprint) || len(input.Catalog.Items) > 100000 || len(p.Mappings) > 1000 || len(p.Deployments) > 1000 || len(p.HeadCapabilities) > 5000 || len(p.AllowedProfiles) > 1000 {
		return ErrInvalid
	}
	resources := map[[3]string]bool{}
	for _, item := range input.Catalog.Items {
		key := [3]string{item.Kind, item.Reference.Issuer, item.Reference.ID}
		if resources[key] || !validReference(item.Reference) {
			return ErrInvalid
		}
		resources[key] = true
	}
	ids := map[string]bool{}
	for _, mapping := range p.Mappings {
		if strings.TrimSpace(mapping.ID) == "" || ids[mapping.ID] || !catalog.IsLocalKey(mapping.Capability) {
			return ErrInvalid
		}
		ids[mapping.ID] = true
	}
	environments := map[Identity]bool{}
	for _, deployment := range p.Deployments {
		key := Identity{deployment.Environment.Issuer, deployment.Environment.ID}
		if environments[key] {
			return ErrInvalid
		}
		environments[key] = true
	}

	return nil
}
