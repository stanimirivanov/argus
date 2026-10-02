package adaptation

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

const (
	// ValidationRequestAPIVersion identifies one phase request sent to a
	// repository-owned validation adapter.
	ValidationRequestAPIVersion = "argus.dev/functional-api-repair-validation-request/v1"
	// ValidationResultAPIVersion identifies untrusted output for one phase.
	ValidationResultAPIVersion = "argus.dev/functional-api-repair-validation-result/v1"
	// ValidationEvidenceAPIVersion identifies a successful three-phase proof.
	ValidationEvidenceAPIVersion = "argus.dev/validation-evidence/v1"
	// ValidationRejectionAPIVersion identifies a trustworthy policy rejection.
	ValidationRejectionAPIVersion = "argus.dev/validation-rejection/v1"
	// ValidationPolicyVersion identifies the original/candidate/negative-control policy.
	ValidationPolicyVersion = "argus.dev/validation-policy/functional-api-endpoint-rename/v1"
	// ValidationProposalPolicyVersion is the proposal policy admitted by this
	// version of the functional API validation evidence contract.
	ValidationProposalPolicyVersion = "argus.dev/adaptation-policy/functional-api-endpoint-rename/v1"
)

// ValidationRejectionReason explains which discriminating gate rejected a
// candidate. Infrastructure and evidence-integrity errors are deliberately not
// represented as policy rejections.
type ValidationRejectionReason string

const (
	// ValidationOriginalPassed means the unchanged test did not reproduce the diagnosed failure.
	ValidationOriginalPassed ValidationRejectionReason = "original-passed"
	// ValidationCandidateFailed means the exact proposed edit did not repair the test.
	ValidationCandidateFailed ValidationRejectionReason = "candidate-failed"
	// ValidationNegativeControlPassed means the test did not discriminate an invalid target.
	ValidationNegativeControlPassed ValidationRejectionReason = "negative-control-passed"
)

// ValidationPhase identifies one isolated execution state.
type ValidationPhase string

const (
	// ValidationOriginal executes the untouched test and must reproduce failure.
	ValidationOriginal ValidationPhase = "original"
	// ValidationCandidate executes the exact proposed replacement and must pass.
	ValidationCandidate ValidationPhase = "candidate"
	// ValidationNegativeControl replaces the request target with a deterministic
	// invalid endpoint and must fail.
	ValidationNegativeControl ValidationPhase = "negative-control"
)

// ValidationOutcome is the normalized result for the one requested test.
type ValidationOutcome string

const (
	// ValidationPassed means the requested test completed successfully.
	ValidationPassed ValidationOutcome = "passed"
	// ValidationFailed means the requested test completed with an assertion or product failure.
	ValidationFailed ValidationOutcome = "failed"
	// ValidationError means infrastructure or test setup prevented a trustworthy result.
	ValidationError ValidationOutcome = "error"
)

// ValidationFailure carries bounded, adapter-normalized diagnostics.
type ValidationFailure struct {
	Code    string
	Message string
}

// ValidationRequest asks an adapter to execute exactly one catalog test in the
// current disposable workspace state.
type ValidationRequest struct {
	APIVersion   string
	ValidationID string
	ProposalID   string
	Phase        ValidationPhase
	Test         TestReference
	SourceSHA256 string
}

// ValidationAdapterResult is untrusted output for one requested phase.
type ValidationAdapterResult struct {
	APIVersion     string
	ValidationID   string
	ProposalID     string
	Phase          ValidationPhase
	SuiteKey       string
	TestKey        string
	AdapterID      string
	AdapterVersion string
	SourceSHA256   string
	StartedAt      time.Time
	CompletedAt    time.Time
	Outcome        ValidationOutcome
	Failure        *ValidationFailure
}

// ValidationRun is the normalized immutable evidence for one phase.
type ValidationRun struct {
	Phase        ValidationPhase
	SourceSHA256 string
	StartedAt    time.Time
	CompletedAt  time.Time
	Outcome      ValidationOutcome
	Failure      *ValidationFailure
}

// ValidationSourceEvidence proves which bytes were executed and restored.
type ValidationSourceEvidence struct {
	Path                string
	OriginalSHA256      string
	CandidateSHA256     string
	NegativeSHA256      string
	RestoredSHA256      string
	NegativeControlPath string
}

// ValidationEvidence proves the narrow sequence required before review can be
// considered. It is not merge authorization.
type ValidationEvidence struct {
	APIVersion            string
	PolicyVersion         string
	ValidationID          string
	ProposalID            string
	ProposalPolicyVersion string
	Test                  TestReference
	AdapterID             string
	AdapterVersion        string
	Edit                  TextEdit
	Source                ValidationSourceEvidence
	Runs                  []ValidationRun
}

// ValidationRejectionEvidence preserves a completed, trustworthy prefix of
// the validation protocol when an observed outcome disproves the candidate.
// It is negative learning evidence and never authorizes review publication.
type ValidationRejectionEvidence struct {
	APIVersion            string
	PolicyVersion         string
	ValidationID          string
	ProposalID            string
	ProposalPolicyVersion string
	Test                  TestReference
	AdapterID             string
	AdapterVersion        string
	Edit                  TextEdit
	Source                ValidationSourceEvidence
	RejectedPhase         ValidationPhase
	Reason                ValidationRejectionReason
	ExpectedOutcome       ValidationOutcome
	ActualOutcome         ValidationOutcome
	Runs                  []ValidationRun
}

// ValidationRejectedError returns portable rejection evidence while retaining
// errors.Is compatibility with ErrValidationRejected.
type ValidationRejectedError struct {
	Evidence ValidationRejectionEvidence
}

func (rejection *ValidationRejectedError) Error() string {
	return fmt.Sprintf("%s: %s outcome is %s, expected %s",
		ErrValidationRejected,
		rejection.Evidence.RejectedPhase,
		rejection.Evidence.ActualOutcome,
		rejection.Evidence.ExpectedOutcome,
	)
}

func (rejection *ValidationRejectedError) Unwrap() error { return ErrValidationRejected }

// CanonicalValidationEvidence deep-copies runs and orders them by required phase.
func CanonicalValidationEvidence(evidence ValidationEvidence) ValidationEvidence {
	canonical := evidence
	canonical.Test = CanonicalTestReference(evidence.Test)
	canonical.Runs = make([]ValidationRun, len(evidence.Runs))
	for index, run := range evidence.Runs {
		canonical.Runs[index] = run
		canonical.Runs[index].StartedAt = run.StartedAt.UTC()
		canonical.Runs[index].CompletedAt = run.CompletedAt.UTC()
		canonical.Runs[index].Failure = cloneValidationFailure(run.Failure)
	}
	slices.SortFunc(canonical.Runs, func(left, right ValidationRun) int {
		return cmp.Compare(validationPhaseOrder(left.Phase), validationPhaseOrder(right.Phase))
	})

	return canonical
}

// CanonicalValidationRejectionEvidence deep-copies and orders completed runs.
func CanonicalValidationRejectionEvidence(evidence ValidationRejectionEvidence) ValidationRejectionEvidence {
	canonical := evidence
	canonical.Test = CanonicalTestReference(evidence.Test)
	canonical.Runs = canonicalValidationRuns(evidence.Runs)

	return canonical
}

// ValidateValidationRequest enforces the phase protocol independently of JSON.
func ValidateValidationRequest(request ValidationRequest) error {
	if request.APIVersion != ValidationRequestAPIVersion ||
		!sha256Pattern.MatchString(request.ValidationID) || !sha256Pattern.MatchString(request.ProposalID) ||
		!validValidationPhase(request.Phase) || !sha256Pattern.MatchString(request.SourceSHA256) {
		return fmt.Errorf("%w: validation request envelope", ErrInvalid)
	}

	return ValidateTestReference(request.Test)
}

// ValidateValidationAdapterResult validates untrusted result semantics before
// the application correlates them to a request.
func ValidateValidationAdapterResult(result ValidationAdapterResult) error {
	if result.APIVersion != ValidationResultAPIVersion ||
		!sha256Pattern.MatchString(result.ValidationID) || !sha256Pattern.MatchString(result.ProposalID) ||
		!validValidationPhase(result.Phase) || !catalogKey(result.SuiteKey) || !catalogKey(result.TestKey) ||
		!catalogKey(result.AdapterID) || strings.TrimSpace(result.AdapterVersion) == "" ||
		len(result.AdapterVersion) > 127 || !sha256Pattern.MatchString(result.SourceSHA256) ||
		result.StartedAt.IsZero() || result.CompletedAt.Before(result.StartedAt) {
		return fmt.Errorf("%w: validation result envelope", ErrInvalid)
	}
	switch result.Outcome {
	case ValidationPassed:
		if result.Failure != nil {
			return fmt.Errorf("%w: passing validation result has failure", ErrInvalid)
		}
	case ValidationFailed, ValidationError:
		if err := validateValidationFailure(result.Failure); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: validation outcome", ErrInvalid)
	}

	return nil
}

// ValidateValidationEvidence enforces the successful three-phase proof.
func ValidateValidationEvidence(evidence ValidationEvidence) error {
	if evidence.APIVersion != ValidationEvidenceAPIVersion ||
		evidence.PolicyVersion != ValidationPolicyVersion ||
		!sha256Pattern.MatchString(evidence.ValidationID) || !sha256Pattern.MatchString(evidence.ProposalID) ||
		evidence.ProposalPolicyVersion != ValidationProposalPolicyVersion || !catalogKey(evidence.AdapterID) ||
		strings.TrimSpace(evidence.AdapterVersion) == "" || len(evidence.AdapterVersion) > 127 {
		return fmt.Errorf("%w: validation evidence envelope", ErrInvalid)
	}
	if err := ValidateTestReference(evidence.Test); err != nil {
		return err
	}
	if evidence.AdapterID != evidence.Test.Adapter || !validRepositoryPath(evidence.Source.Path) ||
		validateValidationTextEdit(evidence.Edit) != nil || evidence.Edit.Path != evidence.Source.Path ||
		evidence.Edit.BeforeSHA256 != evidence.Source.OriginalSHA256 ||
		!sha256Pattern.MatchString(evidence.Source.OriginalSHA256) ||
		!sha256Pattern.MatchString(evidence.Source.CandidateSHA256) ||
		!sha256Pattern.MatchString(evidence.Source.NegativeSHA256) ||
		evidence.Source.RestoredSHA256 != evidence.Source.OriginalSHA256 ||
		!strings.HasPrefix(evidence.Source.NegativeControlPath, "/__argus_negative_control__/") {
		return fmt.Errorf("%w: validation source evidence", ErrInvalid)
	}
	canonical := CanonicalValidationEvidence(evidence)
	if !equalValidationRuns(evidence.Runs, canonical.Runs) || len(evidence.Runs) != 3 {
		return fmt.Errorf("%w: validation runs", ErrInvalid)
	}
	expectedPhases := []ValidationPhase{ValidationOriginal, ValidationCandidate, ValidationNegativeControl}
	expectedOutcomes := []ValidationOutcome{ValidationFailed, ValidationPassed, ValidationFailed}
	expectedHashes := []string{
		evidence.Source.OriginalSHA256, evidence.Source.CandidateSHA256, evidence.Source.NegativeSHA256,
	}
	for index, run := range evidence.Runs {
		if run.Phase != expectedPhases[index] || run.Outcome != expectedOutcomes[index] ||
			run.SourceSHA256 != expectedHashes[index] || run.StartedAt.IsZero() ||
			run.CompletedAt.Before(run.StartedAt) {
			return fmt.Errorf("%w: validation run policy", ErrInvalid)
		}
		if run.Outcome == ValidationPassed {
			if run.Failure != nil {
				return fmt.Errorf("%w: passing evidence has failure", ErrInvalid)
			}
		} else if err := validateValidationFailure(run.Failure); err != nil {
			return err
		}
		if index > 0 && run.StartedAt.Before(evidence.Runs[index-1].CompletedAt) {
			return fmt.Errorf("%w: overlapping validation runs", ErrInvalid)
		}
	}

	return nil
}

// ValidateValidationRejectionEvidence accepts only the three expected policy
// mismatches and requires the complete ordered run prefix through rejection.
func ValidateValidationRejectionEvidence(evidence ValidationRejectionEvidence) error {
	if evidence.APIVersion != ValidationRejectionAPIVersion ||
		evidence.PolicyVersion != ValidationPolicyVersion ||
		!sha256Pattern.MatchString(evidence.ValidationID) || !sha256Pattern.MatchString(evidence.ProposalID) ||
		evidence.ProposalPolicyVersion != ValidationProposalPolicyVersion || !catalogKey(evidence.AdapterID) ||
		strings.TrimSpace(evidence.AdapterVersion) == "" || len(evidence.AdapterVersion) > 127 {
		return fmt.Errorf("%w: validation rejection envelope", ErrInvalid)
	}
	if err := ValidateTestReference(evidence.Test); err != nil {
		return err
	}
	if err := validateValidationSource(evidence.AdapterID, evidence.Test, evidence.Edit, evidence.Source); err != nil {
		return err
	}
	expectedPhase, expectedOutcome, runCount, valid := rejectionPolicy(evidence.Reason)
	if !valid || evidence.RejectedPhase != expectedPhase || evidence.ExpectedOutcome != expectedOutcome ||
		evidence.ActualOutcome == expectedOutcome || evidence.ActualOutcome == ValidationError ||
		len(evidence.Runs) != runCount {
		return fmt.Errorf("%w: validation rejection policy", ErrInvalid)
	}
	canonical := CanonicalValidationRejectionEvidence(evidence)
	if !equalValidationRuns(evidence.Runs, canonical.Runs) {
		return fmt.Errorf("%w: validation rejection runs", ErrInvalid)
	}
	if err := validateValidationRejectionRuns(evidence); err != nil {
		return err
	}
	last := evidence.Runs[len(evidence.Runs)-1]
	if last.Phase != evidence.RejectedPhase || last.Outcome != evidence.ActualOutcome {
		return fmt.Errorf("%w: validation rejection correlation", ErrInvalid)
	}

	return nil
}

func validateValidationRejectionRuns(evidence ValidationRejectionEvidence) error {
	expectedPhases := []ValidationPhase{ValidationOriginal, ValidationCandidate, ValidationNegativeControl}
	expectedHashes := []string{
		evidence.Source.OriginalSHA256, evidence.Source.CandidateSHA256, evidence.Source.NegativeSHA256,
	}
	for index, run := range evidence.Runs {
		if run.Phase != expectedPhases[index] || run.SourceSHA256 != expectedHashes[index] ||
			run.StartedAt.IsZero() || run.CompletedAt.Before(run.StartedAt) {
			return fmt.Errorf("%w: validation rejection run policy", ErrInvalid)
		}
		if index < len(evidence.Runs)-1 {
			required := []ValidationOutcome{ValidationFailed, ValidationPassed}[index]
			if run.Outcome != required {
				return fmt.Errorf("%w: validation rejection prefix", ErrInvalid)
			}
		}
		if err := validateValidationRejectionRun(run); err != nil {
			return err
		}
		if index > 0 && run.StartedAt.Before(evidence.Runs[index-1].CompletedAt) {
			return fmt.Errorf("%w: overlapping validation rejection runs", ErrInvalid)
		}
	}

	return nil
}

func validateValidationRejectionRun(run ValidationRun) error {
	switch run.Outcome {
	case ValidationPassed:
		if run.Failure != nil {
			return fmt.Errorf("%w: passing rejection run has failure", ErrInvalid)
		}
	case ValidationFailed:
		return validateValidationFailure(run.Failure)
	default:
		return fmt.Errorf("%w: validation rejection run outcome", ErrInvalid)
	}

	return nil
}

func validateValidationSource(
	adapterID string,
	test TestReference,
	edit TextEdit,
	source ValidationSourceEvidence,
) error {
	if adapterID != test.Adapter || !validRepositoryPath(source.Path) ||
		validateValidationTextEdit(edit) != nil || edit.Path != source.Path ||
		edit.BeforeSHA256 != source.OriginalSHA256 ||
		!sha256Pattern.MatchString(source.OriginalSHA256) ||
		!sha256Pattern.MatchString(source.CandidateSHA256) ||
		!sha256Pattern.MatchString(source.NegativeSHA256) ||
		source.RestoredSHA256 != source.OriginalSHA256 ||
		!strings.HasPrefix(source.NegativeControlPath, "/__argus_negative_control__/") {
		return fmt.Errorf("%w: validation source evidence", ErrInvalid)
	}

	return nil
}

func rejectionPolicy(reason ValidationRejectionReason) (ValidationPhase, ValidationOutcome, int, bool) {
	switch reason {
	case ValidationOriginalPassed:
		return ValidationOriginal, ValidationFailed, 1, true
	case ValidationCandidateFailed:
		return ValidationCandidate, ValidationPassed, 2, true
	case ValidationNegativeControlPassed:
		return ValidationNegativeControl, ValidationFailed, 3, true
	default:
		return "", "", 0, false
	}
}

func canonicalValidationRuns(runs []ValidationRun) []ValidationRun {
	canonical := make([]ValidationRun, len(runs))
	for index, run := range runs {
		canonical[index] = run
		canonical[index].StartedAt = run.StartedAt.UTC()
		canonical[index].CompletedAt = run.CompletedAt.UTC()
		canonical[index].Failure = cloneValidationFailure(run.Failure)
	}
	slices.SortFunc(canonical, func(left, right ValidationRun) int {
		return cmp.Compare(validationPhaseOrder(left.Phase), validationPhaseOrder(right.Phase))
	})

	return canonical
}

func validateValidationFailure(failure *ValidationFailure) error {
	if failure == nil || !catalogKey(failure.Code) || strings.TrimSpace(failure.Message) == "" ||
		len(failure.Message) > 2000 {
		return fmt.Errorf("%w: validation failure", ErrInvalid)
	}

	return nil
}

func validateValidationTextEdit(edit TextEdit) error {
	if err := ValidateTextEdit(edit); err != nil {
		return err
	}
	if edit.SemanticRole != "request-target" {
		return fmt.Errorf("%w: validation request-target edit", ErrInvalid)
	}

	return nil
}

func validValidationPhase(phase ValidationPhase) bool {
	return phase == ValidationOriginal || phase == ValidationCandidate || phase == ValidationNegativeControl
}

func validationPhaseOrder(phase ValidationPhase) int {
	switch phase {
	case ValidationOriginal:
		return 0
	case ValidationCandidate:
		return 1
	case ValidationNegativeControl:
		return 2
	default:
		return 3
	}
}

func catalogKey(value string) bool {
	return catalog.IsLocalKey(value)
}

func cloneValidationFailure(failure *ValidationFailure) *ValidationFailure {
	if failure == nil {
		return nil
	}
	cloned := *failure

	return &cloned
}

func equalValidationRuns(left, right []ValidationRun) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Phase != right[index].Phase || left[index].SourceSHA256 != right[index].SourceSHA256 ||
			!left[index].StartedAt.Equal(right[index].StartedAt) ||
			!left[index].CompletedAt.Equal(right[index].CompletedAt) ||
			left[index].Outcome != right[index].Outcome ||
			!equalValidationFailure(left[index].Failure, right[index].Failure) {
			return false
		}
	}

	return true
}

func equalValidationFailure(left, right *ValidationFailure) bool {
	if left == nil || right == nil {
		return left == right
	}

	return *left == *right
}
