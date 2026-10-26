package selectionio_test

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/selectionio"
)

func TestReviewInputPreservesCanonicalApprovalWithoutGrantingAuthority(t *testing.T) {
	t.Parallel()
	evidence, policy, review, authority := requestReview(t, false)
	raw := encode(t, review)
	decoded, err := selectionio.DecodeRequestReview(append([]byte(" \n"), raw...))
	if err != nil || !reflect.DeepEqual(decoded, review) {
		t.Fatalf("changed canonical review: %v", err)
	}
	if _, err := selectionio.Compile(evidence, policy, decoded, authority, observed); err != nil {
		t.Fatal(err)
	}
	if _, err := selectionio.Compile(evidence, policy, decoded, selectionio.RequestAuthority{}, observed); !errors.Is(err, performance.ErrInvalid) {
		t.Fatalf("decoded claims granted authority: %v", err)
	}
}

func TestReviewInputRejectsAmbiguousAndUnboundedArtifacts(t *testing.T) {
	t.Parallel()
	_, _, review, _ := requestReview(t, false)
	raw := encode(t, review)
	for _, invalid := range [][]byte{
		nil, []byte("null"), append(raw, []byte(`{}`)...),
		append([]byte(`{"Unexpected":true,`), raw[1:]...),
		append([]byte(`{"Reviewer":null,`), raw[1:]...),
		bytes.Replace(raw, []byte(`"Selection"`), []byte(`"selection"`), 1),
		bytes.Replace(raw, []byte(`"checkout-owner"`), []byte{'"', 0xff, '"'}, 1),
		bytes.Repeat([]byte(" "), (16<<20)+1),
	} {
		decoded, err := selectionio.DecodeRequestReview(invalid)
		if !errors.Is(err, performance.ErrInvalid) || !reflect.DeepEqual(decoded, selectionio.RequestReview{}) {
			t.Fatalf("accepted ambiguous artifact: bytes=%d err=%v", len(invalid), err)
		}
	}
}
