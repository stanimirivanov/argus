package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/stanimirivanov/argus/internal/selection"
)

// ManifestSHA256V1 returns the digest of canonical execution-manifest v1 JSON.
// It is the manifest identity used by requests, attempts, and execution plans.
func ManifestSHA256V1(manifest selection.Manifest) (string, error) {
	document, err := ExportV1(manifest)
	if err != nil {
		return "", fmt.Errorf("canonicalize execution manifest: %w", err)
	}
	data, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode canonical execution manifest: %w", err)
	}
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:]), nil
}
