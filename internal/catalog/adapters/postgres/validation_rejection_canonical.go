package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/stanimirivanov/argus/internal/adaptation"
)

func validationRejectionFingerprint(evidence adaptation.ValidationRejectionEvidence) (string, error) {
	evidence = adaptation.CanonicalValidationRejectionEvidence(evidence)
	data, err := json.Marshal(evidence)
	if err != nil {
		return "", adaptation.ErrUnavailable
	}
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:]), nil
}
