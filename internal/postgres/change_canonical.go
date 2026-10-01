package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/change"
)

func changeSetFingerprint(set change.Set) (string, error) {
	canonical := change.CanonicalSet(set)
	payload := struct {
		APIVersion        string
		SourceRepository  any
		PullRequestNumber int
		BaseRevision      any
		HeadRevision      any
		ObservedAt        string
		Trigger           any
		Files             []change.File
		FilesTruncated    bool
	}{
		APIVersion:        canonical.APIVersion,
		SourceRepository:  canonical.SourceRepository,
		PullRequestNumber: canonical.PullRequestNumber,
		BaseRevision:      canonical.BaseRevision,
		HeadRevision:      canonical.HeadRevision,
		ObservedAt:        canonical.ObservedAt.Format(time.RFC3339Nano),
		Trigger:           canonical.Trigger,
		Files:             canonical.Files,
		FilesTruncated:    canonical.FilesTruncated,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode canonical change set: %w", err)
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}
