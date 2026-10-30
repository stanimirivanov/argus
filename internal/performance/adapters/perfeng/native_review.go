package perfeng

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"strings"
)

// NativeReviewVersion identifies internal protected operator/CI configuration.
const NativeReviewVersion = "argus.dev/performance-native-review/v1"

// NativeReviewMaximumBytes accommodates a base64 policy plus bounded metadata.
const NativeReviewMaximumBytes = 2 << 20

// ErrNativeReview rejects invalid/unbound review material without diagnostics.
var ErrNativeReview = errors.New("invalid frozen native analysis review")

// NativeReviewBundle is canonical host-issued historical review configuration.
// PolicyBytes encodes exact original bytes as base64. The trusted host authenticates
// issuance and protects origin/access independently of these self-declared fields.
type NativeReviewBundle struct {
	APIVersion       string                   `json:"apiVersion"`
	ReviewID         string                   `json:"reviewId"`
	ReviewedBy       string                   `json:"reviewedBy"`
	Provider         string                   `json:"provider"`
	Principal        IntegrationIdentity      `json:"principal"`
	RequestSHA256    string                   `json:"requestSha256"`
	AdmissionSHA256  string                   `json:"admissionSha256"`
	Native           PublicationNativeBinding `json:"native"`
	PolicyBytes      []byte                   `json:"policyBytes"`
	Producer         AnalysisProducer         `json:"producer"`
	ContractsVersion string                   `json:"contractsVersion"`
}

// NativeReviewData contains owned frozen configuration, original file bytes and digest.
// It grants no artifact access, new execution or baseline promotion authority.
type NativeReviewData struct {
	Bundle  NativeReviewBundle
	SHA256  string
	Payload []byte
}

// NativeReview is loaded only through the protected-source boundary below.
// Its zero value has no digest and must not be consumed as reviewed evidence.
type NativeReview struct{ data NativeReviewData }

// Data copies original file/policy bytes and baseline metadata for concurrent reads.
func (review NativeReview) Data() NativeReviewData {
	data := review.data
	data.Payload = bytes.Clone(data.Payload)
	data.Bundle.PolicyBytes = bytes.Clone(data.Bundle.PolicyBytes)
	if baseline := data.Bundle.Native.Baseline; baseline != nil {
		copy := *baseline
		data.Bundle.Native.Baseline = &copy
	}

	return data
}

var (
	nativeReviewID      = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	nativeReviewVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	nativeReviewImage   = regexp.MustCompile(`^[a-z0-9][a-z0-9._:/-]*@sha256:[a-f0-9]{64}$`)
)

// LoadNativeReview reads bounded canonical json.Marshal(NativeReviewBundle) bytes,
// allowing outer whitespace. The host supplies a protected regular-file reader
// and independently authenticated expectedReviewer, not agent/provider-controlled
// origin or metadata. This validates correlation/integrity, never OS identities or
// ACLs. Reader ownership stays with the host; reads must be bounded regular-file
// operations or honor ctx. Failures return zero review and safe classified errors.
func LoadNativeReview(ctx context.Context, reader io.Reader, admitted PublicationAdmission, expectedReviewer string) (NativeReview, error) {
	if err := ctx.Err(); err != nil {
		return NativeReview{}, err
	}
	if reader == nil || !nativeReviewText(expectedReviewer) {
		return NativeReview{}, ErrNativeReview
	}
	raw, err := io.ReadAll(io.LimitReader(reader, NativeReviewMaximumBytes+1))
	if ctx.Err() != nil {
		return NativeReview{}, ctx.Err()
	}
	if err != nil || len(raw) > NativeReviewMaximumBytes || !validUnicode(raw) {
		return NativeReview{}, ErrNativeReview
	}
	var bundle NativeReviewBundle
	if json.Unmarshal(raw, &bundle) != nil {
		return NativeReview{}, ErrNativeReview
	}
	canonical, err := json.Marshal(bundle)
	valid := err == nil && bytes.Equal(bytes.TrimSpace(raw), canonical) && validNativeReview(bundle, admitted, expectedReviewer)
	if ctx.Err() != nil {
		return NativeReview{}, ctx.Err()
	}
	if !valid {
		return NativeReview{}, ErrNativeReview
	}
	digest := sha256.Sum256(raw)

	return NativeReview{data: NativeReviewData{Bundle: bundle, SHA256: hex.EncodeToString(digest[:]), Payload: bytes.Clone(raw)}}, nil
}

func nativeReviewText(value string) bool {
	return len(value) <= 256 && strings.TrimSpace(value) != "" && !strings.ContainsRune(value, 0)
}

func validNativeReview(bundle NativeReviewBundle, admitted PublicationAdmission, reviewer string) bool {
	bound := admitted.Data()
	if bound.SHA256 == "" || bundle.APIVersion != NativeReviewVersion || !nativeReviewText(bundle.ReviewID) || bundle.ReviewedBy != reviewer ||
		bundle.RequestSHA256 != bound.Admission.Data().RequestSHA256 || bundle.AdmissionSHA256 != bound.Admission.Data().Evidence.Fingerprint() ||
		len(bundle.PolicyBytes) > NativePolicyMaximumBytes || !nativeReviewID.MatchString(bundle.Producer.Name) ||
		!nativeReviewVersion.MatchString(bundle.Producer.Version) || !nativeReviewVersion.MatchString(bundle.ContractsVersion) || !nativeReviewImage.MatchString(bundle.Producer.Image) {
		return false
	}
	publication, err := DecodePublication(bound.Payload)
	if err != nil || bundle.Provider != publication.Source || bundle.Principal != publication.Data.Context.Principal || !reflect.DeepEqual(bundle.Native, publication.Data.Native) {
		return false
	}
	policy, err := DecodeNativePolicy(bundle.PolicyBytes)
	digest := sha256.Sum256(bundle.PolicyBytes)

	return err == nil && bundle.Native.Policy.ID == policy.Metadata.Name && bundle.Native.Policy.Version == policy.Metadata.Version && bundle.Native.Policy.SHA256 == hex.EncodeToString(digest[:])
}
