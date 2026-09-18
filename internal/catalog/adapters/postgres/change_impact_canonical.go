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
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode canonical capability impact: %w", err)
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}
