package catalog

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// NewRevision validates and normalizes an immutable Git revision supplied by
// a trusted ingestion context.
func NewRevision(algorithm RevisionAlgorithm, digest string) (Revision, error) {
	digest = strings.ToLower(digest)
	expectedBytes, ok := map[RevisionAlgorithm]int{
		RevisionGitSHA1:   20,
		RevisionGitSHA256: 32,
	}[algorithm]
	if !ok {
		return Revision{}, fmt.Errorf("unsupported revision algorithm %q", algorithm)
	}

	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != expectedBytes {
		return Revision{}, fmt.Errorf("invalid %s revision digest", algorithm)
	}

	return Revision{Algorithm: algorithm, Digest: digest}, nil
}
