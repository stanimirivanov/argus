package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/change"
)

func capabilityImpactFingerprint(impact change.CapabilityImpact) (string, error) {
	canonical := change.CanonicalCapabilityImpact(impact)
	payload := struct {
		APIVersion      string
		AnalyzerVersion string
		Change          change.Reference
		ObservedAt      string
		Status          change.ImpactStatus
		Documents       []change.DocumentImpact
		Warnings        []string
	}{
		APIVersion:      canonical.APIVersion,
		AnalyzerVersion: canonical.AnalyzerVersion,
		Change:          canonical.Change,
		ObservedAt:      canonical.Change.ObservedAt.Format(time.RFC3339Nano),
		Status:          canonical.Status,
		Documents:       canonical.Documents,
		Warnings:        canonical.Warnings,
	}
	payload.Change.ObservedAt = time.Time{}
	var data []byte
	var err error
	if canonical.APIVersion == change.CompositeImpactAPIVersion {
		data, err = json.Marshal(struct {
			Payload          any
			SemanticStatus   change.ImpactStatus
			FileStatus       change.ImpactStatus
			Files            []change.FileImpact
			SemanticWarnings []string
			FileWarnings     []string
		}{Payload: payload, SemanticStatus: canonical.SemanticStatus,
			FileStatus: canonical.FileStatus, Files: canonical.Files,
			SemanticWarnings: canonical.SemanticWarnings, FileWarnings: canonical.FileWarnings})
	} else {
		// Keep the original v1 fingerprint byte-for-byte stable for retries.
		data, err = json.Marshal(payload)
	}
	if err != nil {
		return "", fmt.Errorf("encode canonical capability impact: %w", err)
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}
