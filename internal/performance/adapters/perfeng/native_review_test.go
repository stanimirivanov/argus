package perfeng

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func nativeReviewFixture(t *testing.T) (NativeReviewBundle, PublicationAdmission) {
	t.Helper()
	f := publicationAdmissionFixture(t)
	admitted, err := bindPublicationFixture(f)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := DecodePublication(f.wire)
	if err != nil {
		t.Fatal(err)
	}
	report, err := DecodeAnalysis(analysisFixture(t, "slo-pass-regression-fail"))
	if err != nil {
		t.Fatal(err)
	}

	return NativeReviewBundle{APIVersion: NativeReviewVersion, ReviewID: "review-42", ReviewedBy: "operator:owner", Provider: f.provider,
		Principal: publication.Data.Context.Principal, RequestSHA256: admitted.Data().Admission.Data().RequestSHA256,
		AdmissionSHA256: f.retained.Fingerprint(), Native: publication.Data.Native, PolicyBytes: nativePolicyFixture(t, "browser"),
		Producer: report.Producer, ContractsVersion: report.ContractsVersion}, admitted
}

func nativeReviewBytes(t *testing.T, bundle NativeReviewBundle) []byte {
	t.Helper()
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

func TestNativeReviewLoadsOwnedHistoricalEvidence(t *testing.T) {
	bundle, admitted := nativeReviewFixture(t)
	raw := append(append([]byte(" \n"), nativeReviewBytes(t, bundle)...), '\n')
	review, err := LoadNativeReview(context.Background(), bytes.NewReader(raw), admitted, bundle.ReviewedBy)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	data := review.Data()
	if !reflect.DeepEqual(data.Bundle, bundle) || data.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("exact review/policy bytes or pins changed")
	}
	data.Bundle.PolicyBytes[0] = '!'
	data.Bundle.Native.Baseline.Identity.ID = "mutated"
	raw[0] = '!'
	if !reflect.DeepEqual(review.Data().Bundle, bundle) {
		t.Fatal("review owns aliased bytes or baseline")
	}
	if got := (NativeReview{}).Data(); got.SHA256 != "" || got.Bundle.PolicyBytes != nil {
		t.Fatal("zero value gained authority")
	}
}

func TestNativeReviewRejectsScopePinsNativeAndPolicySubstitution(t *testing.T) {
	for _, mode := range []string{"request", "admission", "provider", "principal", "namespace", "run", "workload", "baseline", "policy-bytes", "policy-name", "policy-version", "reviewer", "zero-admission"} {
		t.Run(mode, func(t *testing.T) {
			bundle, admitted := nativeReviewFixture(t)
			reviewer := bundle.ReviewedBy
			switch mode {
			case "request":
				bundle.RequestSHA256 = strings.Repeat("f", 64)
			case "admission":
				bundle.AdmissionSHA256 = strings.Repeat("f", 64)
			case "provider":
				bundle.Provider += "/other"
			case "principal":
				bundle.Principal.ID += "-other"
			case "namespace":
				bundle.Principal.Issuer += "/other"
			case "run":
				bundle.Native.RunID += "-other"
			case "workload":
				bundle.Native.Workload.SHA256 = strings.Repeat("f", 64)
			case "baseline":
				bundle.Native.Baseline = nil
			case "policy-bytes":
				bundle.PolicyBytes = append(bundle.PolicyBytes, '\n')
			case "policy-name":
				bundle.PolicyBytes = bytes.Replace(bundle.PolicyBytes, []byte(`"name": "search-browser"`), []byte(`"name": "other-policy"`), 1)
			case "policy-version":
				bundle.PolicyBytes = bytes.Replace(bundle.PolicyBytes, []byte(`"version": "1.0.0"`), []byte(`"version": "2.0.0"`), 1)
			case "reviewer":
				reviewer = "another-reviewer"
			case "zero-admission":
				admitted = PublicationAdmission{}
			}
			assertInvalidNativeReview(t, nativeReviewBytes(t, bundle), admitted, reviewer)
		})
	}
}

func TestNativeReviewRequiresPolicyIdentityEvenWhenDigestMatches(t *testing.T) {
	for _, field := range []string{"name", "version"} {
		bundle, _ := nativeReviewFixture(t)
		old, replacement := `"name": "search-browser"`, `"name": "other-policy"`
		if field == "version" {
			old, replacement = `"version": "1.0.0"`, `"version": "2.0.0"`
		}
		bundle.PolicyBytes = bytes.Replace(bundle.PolicyBytes, []byte(old), []byte(replacement), 1)
		digest := sha256.Sum256(bundle.PolicyBytes)
		bundle.Native.Policy.SHA256 = hex.EncodeToString(digest[:])
		f := publicationAdmissionFixture(t)
		publication, err := DecodePublication(f.wire)
		if err != nil {
			t.Fatal(err)
		}
		publication.Data.Native = bundle.Native
		f.wire, err = EncodePublication(publication)
		if err != nil {
			t.Fatal(err)
		}
		admitted, err := bindPublicationFixture(f)
		if err != nil {
			t.Fatal(err)
		}
		assertInvalidNativeReview(t, nativeReviewBytes(t, bundle), admitted, bundle.ReviewedBy)
	}
}

func TestNativeReviewReadsHostFileWithoutTakingOwnership(t *testing.T) {
	bundle, admitted := nativeReviewFixture(t)
	path := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(path, nativeReviewBytes(t, bundle), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := LoadNativeReview(context.Background(), file, admitted, bundle.ReviewedBy); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal("loader closed host file", err)
	}
}

func assertInvalidNativeReview(t *testing.T, raw []byte, admitted PublicationAdmission, reviewer string) {
	t.Helper()
	got, err := LoadNativeReview(context.Background(), bytes.NewReader(raw), admitted, reviewer)
	if !errors.Is(err, ErrNativeReview) || !reflect.DeepEqual(got, NativeReview{}) || strings.Contains(err.Error(), "private") {
		t.Fatal("unsafe/partial review failure", err)
	}
}

func TestNativeReviewRejectsNoncanonicalAndInvalidProvenance(t *testing.T) {
	bundle, admitted := nativeReviewFixture(t)
	base := nativeReviewBytes(t, bundle)
	for _, raw := range [][]byte{
		bytes.Replace(base, []byte(`"reviewId":`), []byte(`"unknown":true,"reviewId":`), 1),
		bytes.Replace(base, []byte(`"reviewId":`), []byte(`"reviewId":"other","reviewId":`), 1),
		bytes.Replace(base, []byte(`"reviewId":`), []byte(`"ReviewId":`), 1),
		bytes.Replace(base, []byte(`"review-42"`), []byte(`"\ud800"`), 1),
		append(bytes.Clone(base), []byte(` {}`)...),
		bytes.Repeat([]byte(" "), NativeReviewMaximumBytes+1),
	} {
		assertInvalidNativeReview(t, raw, admitted, bundle.ReviewedBy)
	}
	for _, mode := range []string{"generation", "id", "producer", "image", "version", "contracts", "policy"} {
		changed := bundle
		switch mode {
		case "generation":
			changed.APIVersion += "-other"
		case "id":
			changed.ReviewID = " "
		case "producer":
			changed.Producer.Name = "bad name"
		case "image":
			changed.Producer.Image = "registry.example/analysis:latest"
		case "version":
			changed.Producer.Version = "latest"
		case "contracts":
			changed.ContractsVersion = "latest"
		case "policy":
			changed.PolicyBytes = []byte(`{}`)
		}
		assertInvalidNativeReview(t, nativeReviewBytes(t, changed), admitted, bundle.ReviewedBy)
	}
}

type nativeReviewReader struct {
	raw    []byte
	err    error
	reads  int
	cancel context.CancelFunc
}

func (reader *nativeReviewReader) Read(target []byte) (int, error) {
	reader.reads++
	if reader.cancel != nil {
		reader.cancel()
	}
	if reader.err != nil {
		return 0, reader.err
	}

	return copy(target, reader.raw), io.EOF
}

func TestNativeReviewHandlesReadErrorsAndCancellation(t *testing.T) {
	bundle, admitted := nativeReviewFixture(t)
	reader := &nativeReviewReader{err: errors.New("private file details")}
	if got, err := LoadNativeReview(context.Background(), reader, admitted, bundle.ReviewedBy); !errors.Is(err, ErrNativeReview) || got.Data().SHA256 != "" || strings.Contains(err.Error(), "private") {
		t.Fatal("file error leaked review or diagnostics", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader.reads = 0
	if got, err := LoadNativeReview(ctx, reader, admitted, bundle.ReviewedBy); !errors.Is(err, context.Canceled) || got.Data().SHA256 != "" || reader.reads != 0 {
		t.Fatal("cancelled loader read file", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	reader = &nativeReviewReader{raw: nativeReviewBytes(t, bundle), cancel: cancel}
	if got, err := LoadNativeReview(ctx, reader, admitted, bundle.ReviewedBy); !errors.Is(err, context.Canceled) || got.Data().SHA256 != "" {
		t.Fatal("cancellation after read retained review", err)
	}
	if got, err := LoadNativeReview(context.Background(), nil, admitted, bundle.ReviewedBy); !errors.Is(err, ErrNativeReview) || got.Data().SHA256 != "" {
		t.Fatal("nil reader accepted", err)
	}
}
