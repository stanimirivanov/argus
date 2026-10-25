package selectionio

import (
	"encoding/json"
	"reflect"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
)

// RequestReview is internal compilation input, not a public review-file format.
// It approves exact provider references and frozen identities, never local labels.
type RequestReview struct {
	Selection             contracts.PerformanceSelectionV1
	Reviewer              perfeng.IntegrationIdentity
	ApprovedAt, ExpiresAt string
	Bindings              []RequestBinding
}

// RequestBinding associates one candidate mapping with an explicitly reviewed
// shared request. Identical requests may account for several distinct mappings.
type RequestBinding struct {
	MappingID string
	Message   perfeng.IntegrationMessage
}

// RequestAuthority MUST come from independently authenticated owner approval.
// ReviewSHA256 pins json.Marshal(RequestReview); computing a hash from untrusted
// review claims does not grant authority. OwnerRoutes are exact authorized pins.
type RequestAuthority struct {
	ReviewSHA256, RequestIssuer string
	Reviewer, Principal         perfeng.IntegrationIdentity
	OwnerRoutes                 []contracts.PerformanceReference
}

// RequestReason preserves each local mapping, capability and selection rationale.
type RequestReason struct{ MappingID, Capability, Reason string }

// CompiledChild owns validated frozen wire bytes and their SHA-256, plus reasons.
// Consumers must persist these bytes verbatim rather than regenerating identities.
type CompiledChild struct {
	Payload []byte
	SHA256  string
	Reasons []RequestReason
}

// RequestSnapshot retains exact policy bytes and canonical selection/review bytes.
// It is an owned projection for persistence, not an interchange contract.
type RequestSnapshot struct {
	Policy, Selection, Review []byte
	Children                  []CompiledChild
}

// Compilation is immutable, concurrent-readable request evidence. Only a successful
// Compile constructs it; its zero value has no children and grants no authority.
type Compilation struct{ snapshot RequestSnapshot }

// Snapshot returns independent copies; callers may mutate them after return.
func (c Compilation) Snapshot() RequestSnapshot {
	s := c.snapshot
	s.Policy, s.Selection, s.Review = slices.Clone(s.Policy), slices.Clone(s.Selection), slices.Clone(s.Review)
	s.Children = slices.Clone(s.Children)
	for i := range s.Children {
		s.Children[i].Payload = slices.Clone(s.Children[i].Payload)
		s.Children[i].Reasons = slices.Clone(s.Children[i].Reasons)
	}

	return s
}

// Compile reselects inspected evidence and original policy at the trusted clock,
// then translates independently approved bindings into validated request bytes.
// It performs no I/O, clock lookup or identity generation. All failure paths return
// zero evidence. Inputs must not be mutated concurrently; results own all bytes.
func Compile(e Evidence, policy []byte, review RequestReview, authority RequestAuthority, at time.Time) (Compilation, error) {
	if len(review.Bindings) == 0 || len(review.Bindings) > 5000 {
		return Compilation{}, performance.ErrInvalid
	}
	current, err := Evaluate(e, policy, at)
	if err != nil {
		return Compilation{}, err
	}
	raw, err := json.Marshal(review)
	if err != nil || len(raw) > 16<<20 || !approvedReview(review, authority, current, raw, at) {
		return Compilation{}, performance.ErrInvalid
	}
	// A live selection cannot legitimize a fabricated evaluation before its inputs existed.
	evaluated, err := time.Parse(time.RFC3339Nano, review.Selection.EvaluatedAt)
	if err != nil {
		return Compilation{}, performance.ErrInvalid
	}
	historical, err := Evaluate(e, policy, evaluated)
	if err != nil || !reflect.DeepEqual(historical, review.Selection) {
		return Compilation{}, performance.ErrInvalid
	}
	selection, err := json.Marshal(review.Selection)
	if err != nil {
		return Compilation{}, performance.ErrInvalid
	}
	snapshot := RequestSnapshot{Policy: slices.Clone(policy), Selection: selection, Review: raw}
	seen, identities, targets := map[string]bool{}, map[string]string{}, map[string]int{}
	for _, binding := range review.Bindings {
		decision, ok := reviewedDecision(current, binding.MappingID, authority.OwnerRoutes)
		if !ok || seen[binding.MappingID] {
			return Compilation{}, performance.ErrInvalid
		}
		seen[binding.MappingID] = true
		payload, err := compileBinding(binding.Message, decision, review, authority, at)
		if err != nil {
			return Compilation{}, err
		}
		reason := RequestReason{MappingID: binding.MappingID, Capability: *decision.Capability, Reason: decision.Reason}
		if !appendChild(&snapshot, targets, identities, binding.Message, payload, reason) {
			return Compilation{}, performance.ErrInvalid
		}
	}
	if !candidatesCovered(current, seen) {
		return Compilation{}, performance.ErrInvalid
	}

	return Compilation{snapshot: snapshot}, nil
}

func contextKey(id perfeng.IntegrationIdentity) string { return id.Issuer + "\x00" + id.ID }

func candidatesCovered(selection contracts.PerformanceSelectionV1, seen map[string]bool) bool {
	for _, decision := range selection.Decisions {
		if decision.Outcome == "candidate" && !seen[*decision.MappingID] {
			return false
		}
	}

	return true
}

func appendChild(s *RequestSnapshot, targets map[string]int, identities map[string]string, m perfeng.IntegrationMessage, payload []byte, reason RequestReason) bool {
	context := m.Data.Context
	context.Request, context.Attempt = perfeng.IntegrationIdentity{}, perfeng.IntegrationIdentity{}
	// Correlation, timing and evidence variations cannot turn one execution target
	// into several requests; conflicting reviewed metadata must fail instead.
	context.RequestedAt, context.Deadline, context.Evidence = "", "", nil
	target, err := json.Marshal(context)
	if err != nil {
		return false
	}
	digest := hash(payload)
	for _, id := range []string{m.Source + "\x00" + m.ID, contextKey(m.Data.Context.Request), contextKey(m.Data.Context.Attempt)} {
		if previous, exists := identities[id]; exists && previous != digest {
			return false
		}
		identities[id] = digest
	}
	if index, exists := targets[string(target)]; exists {
		if s.Children[index].SHA256 != digest {
			return false
		}
		s.Children[index].Reasons = append(s.Children[index].Reasons, reason)
	} else {
		targets[string(target)] = len(s.Children)
		s.Children = append(s.Children, CompiledChild{Payload: payload, SHA256: digest, Reasons: []RequestReason{reason}})
	}

	return true
}

func approvedReview(r RequestReview, a RequestAuthority, current contracts.PerformanceSelectionV1, raw []byte, at time.Time) bool {
	approved, err := time.Parse(time.RFC3339Nano, r.ApprovedAt)
	expires, expiryErr := time.Parse(time.RFC3339Nano, r.ExpiresAt)
	evaluated, evaluatedErr := time.Parse(time.RFC3339Nano, r.Selection.EvaluatedAt)
	selectionExpiry, selectionErr := time.Parse(time.RFC3339Nano, current.ExpiresAt)
	current.EvaluatedAt = r.Selection.EvaluatedAt

	return err == nil && expiryErr == nil && evaluatedErr == nil && selectionErr == nil &&
		!evaluated.After(approved) && !approved.After(at) && at.Before(expires) && !expires.After(selectionExpiry) &&
		hash(raw) == a.ReviewSHA256 && r.Reviewer == a.Reviewer && a.Reviewer.Issuer != "" && a.Reviewer.ID != "" &&
		current.Catalog.Scope.PrincipalIssuer == a.Principal.Issuer && current.Catalog.Scope.PrincipalID == a.Principal.ID &&
		current.Status == "candidates" && reflect.DeepEqual(current, r.Selection)
}

func reviewedDecision(selection contracts.PerformanceSelectionV1, id string, routes []contracts.PerformanceReference) (contracts.PerformanceDecision, bool) {
	for _, d := range selection.Decisions {
		if d.MappingID != nil && *d.MappingID == id && d.Outcome == "candidate" && d.OwnerRoute != nil && slices.Contains(routes, *d.OwnerRoute) {
			return d, true
		}
	}

	return contracts.PerformanceDecision{}, false
}

func compileBinding(m perfeng.IntegrationMessage, d contracts.PerformanceDecision, r RequestReview, a RequestAuthority, at time.Time) ([]byte, error) {
	raw, err := perfeng.EncodeIntegration(m)
	if err != nil {
		return nil, performance.ErrInvalid
	}
	c, target, deployment := m.Data.Context, d.Target, d.Deployment
	deadline, err := time.Parse(time.RFC3339Nano, c.Deadline)
	expires, expiryErr := time.Parse(time.RFC3339Nano, r.ExpiresAt)
	if err != nil || expiryErr != nil || !at.Before(deadline) || deadline.After(expires) ||
		m.Type != "dev.perfeng.integration.requested.v1" || m.Time != r.ApprovedAt || c.RequestedAt != r.ApprovedAt ||
		c.Request.Issuer != a.RequestIssuer || c.Attempt.Issuer != a.RequestIssuer || c.Principal != a.Principal || len(c.Evidence) == 0 ||
		c.Service.Issuer != deployment.Service.Issuer || c.Service.ID != deployment.Service.ID ||
		contracts.PerformanceReference(c.Deployment) != *deployment.Reference ||
		contracts.PerformanceReference(c.Workload) != target.Workload || contracts.PerformanceReference(c.Profile) != target.Profile ||
		contracts.PerformanceReference(c.Environment) != target.Environment || contracts.PerformanceReference(c.Policy) != target.Policy ||
		c.Baseline == nil || contracts.PerformanceReference(*c.Baseline) != *target.Baseline ||
		c.Candidate.Source.Algorithm != deployment.SourceRevision.Algorithm || c.Candidate.Source.Digest != deployment.SourceRevision.Digest ||
		c.Candidate.ImageDigest != "sha256:"+*deployment.ImageSHA256 {
		return nil, performance.ErrInvalid
	}

	return raw, nil
}
