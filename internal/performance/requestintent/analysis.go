package requestintent

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
)

// AnalysisData binds adapter-validated original evidence to retained admission.
// Publication, Review and Report are opaque JSON, not inward vendor contracts.
// PublishedAt is provider occurrence time with full precision, never arrival time.
// Principal and full wire associations must be independently checked by adapters.
type AnalysisData struct {
	Admission                   LifecycleAdmission
	Principal                   performance.Identity
	EventID                     string
	PublishedAt                 time.Time
	Publication, Review, Report []byte
}

// AnalysisEvidence owns immutable reviewed evidence, not authentication or an
// aggregate verdict. Only NewAnalysisEvidence constructs valid inward values.
// Storage must compare admission/request and authorized scope with retained rows.
type AnalysisEvidence struct {
	data                                                       AnalysisData
	publicationSHA256, reviewSHA256, reportSHA256, fingerprint string
}

// NewAnalysisEvidence checks inward bounds, copies bytes and fingerprints all pins.
// It performs no vendor parsing, I/O, clocks, outcome aggregation or authorization.
// Inputs must not be mutated concurrently. Failures return zero evidence.
func NewAnalysisEvidence(data AnalysisData) (AnalysisEvidence, error) {
	admission := data.Admission.Data()
	if admission.Evidence.Fingerprint() == "" || !digest(admission.RequestSHA256) || !text(data.Principal.Issuer, 512) || !text(data.Principal.ID, 256) || !text(data.EventID, 256) || data.PublishedAt.IsZero() || data.PublishedAt.Year() < 1 || data.PublishedAt.Year() > 9999 || !analysisJSON(data.Publication, 64<<10) || !analysisJSON(data.Review, 2<<20) || !analysisJSON(data.Report, 1<<20) {
		return AnalysisEvidence{}, ErrInvalid
	}
	data.PublishedAt = data.PublishedAt.UTC()
	data.Publication, data.Review, data.Report = bytes.Clone(data.Publication), bytes.Clone(data.Review), bytes.Clone(data.Report)
	evidence := AnalysisEvidence{data: data, publicationSHA256: sum(data.Publication), reviewSHA256: sum(data.Review), reportSHA256: sum(data.Report)}
	raw, err := json.Marshal(struct {
		Version, AdmissionSHA256, RequestSHA256       string
		Run, Principal                                performance.Identity
		EventID                                       string
		PublishedAt                                   time.Time
		PublicationSHA256, ReviewSHA256, ReportSHA256 string
	}{"argus.performance-analysis-evidence/v1", admission.Evidence.Fingerprint(), admission.RequestSHA256, admission.Run, data.Principal, data.EventID, data.PublishedAt, evidence.publicationSHA256, evidence.reviewSHA256, evidence.reportSHA256})
	if err != nil {
		return AnalysisEvidence{}, ErrInvalid
	}
	evidence.fingerprint = sum(raw)

	return evidence, nil
}

func analysisJSON(raw []byte, maximum int) bool { return len(raw) <= maximum && json.Valid(raw) }

// Data returns owned original bytes and immutable admission for concurrent reads.
func (evidence AnalysisEvidence) Data() AnalysisData {
	data := evidence.data
	data.Publication, data.Review, data.Report = bytes.Clone(data.Publication), bytes.Clone(data.Review), bytes.Clone(data.Report)

	return data
}

// Fingerprint binds versioned metadata and all original byte digests; zero is invalid.
func (evidence AnalysisEvidence) Fingerprint() string { return evidence.fingerprint }

// PublicationSHA256 returns the original publication-byte digest, including whitespace.
func (evidence AnalysisEvidence) PublicationSHA256() string { return evidence.publicationSHA256 }

// ReviewSHA256 returns the protected original review-file digest, including whitespace.
func (evidence AnalysisEvidence) ReviewSHA256() string { return evidence.reviewSHA256 }

// ReportSHA256 returns the exact reviewed report-byte digest.
func (evidence AnalysisEvidence) ReportSHA256() string { return evidence.reportSHA256 }
