package selectionio

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
)

// DecodeRequestReview reads bounded canonical internal operator evidence. It
// grants no approval: Compile still requires independently authenticated authority.
// Canonical json.Marshal(RequestReview) bytes (optionally surrounded by whitespace)
// prevent ignored/duplicate fields, case aliases and lossy Unicode from changing
// what an operator believes was approved. This is not a portable review contract.
func DecodeRequestReview(raw []byte) (RequestReview, error) {
	var review RequestReview
	if !canonicalRequestInput(raw, &review) {
		return RequestReview{}, performance.ErrInvalid
	}

	return review, nil
}

// RequestApproval combines independently authenticated owner authority and trusted
// provider settings. Its file origin and permissions are controlled by the host.
type RequestApproval struct {
	Authority RequestAuthority
	Expected  perfeng.Expectations
}

// DecodeRequestApproval reads canonical host-provided approval configuration.
// The host must authenticate its issuer and protect its origin/access independently
// of review files. Decoding itself provides neither authentication nor approval.
func DecodeRequestApproval(raw []byte) (RequestApproval, error) {
	var approval RequestApproval
	if !canonicalRequestInput(raw, &approval) {
		return RequestApproval{}, performance.ErrInvalid
	}

	return approval, nil
}

func canonicalRequestInput(raw []byte, target any) bool {
	if len(raw) > 16<<20 || !utf8.Valid(raw) || json.Unmarshal(raw, target) != nil {
		return false
	}
	canonical, err := json.Marshal(target)

	return err == nil && bytes.Equal(bytes.TrimSpace(raw), canonical)
}
