package impact

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

const (
	impactEdgeCursorVersion   = 1
	impactEdgeCursorNamespace = "argus/impact-edge-query/v1"
	maxImpactEdgeCursorLength = 4096
)

type impactEdgeCursor struct {
	Version   int                      `json:"v"`
	QueryHash string                   `json:"q"`
	After     impactEdgeCursorIdentity `json:"after"`
}

type impactEdgeCursorIdentity struct {
	CapabilityKey        string           `json:"capabilityKey"`
	Provider             catalog.Provider `json:"provider"`
	Host                 string           `json:"host"`
	ProviderRepositoryID string           `json:"providerRepositoryId"`
	SuiteKey             string           `json:"suiteKey"`
	TestKey              string           `json:"testKey"`
}

type impactEdgeCursorBinding struct {
	Namespace            string                    `json:"namespace"`
	Provider             catalog.Provider          `json:"provider"`
	Host                 string                    `json:"host"`
	ProviderRepositoryID string                    `json:"providerRepositoryId"`
	RevisionAlgorithm    catalog.RevisionAlgorithm `json:"revisionAlgorithm"`
	RevisionDigest       string                    `json:"revisionDigest"`
	DescriptorAPIVersion string                    `json:"descriptorApiVersion"`
	EvaluatedAt          string                    `json:"evaluatedAt"`
	CapabilityKey        string                    `json:"capabilityKey"`
}

func normalizeImpactEdgeQuery(query EdgeQuery) (EdgeQuery, *EdgeIdentity, error) {
	if err := catalog.ValidateSnapshotKey(query.Snapshot); err != nil {
		return EdgeQuery{}, nil, err
	}
	if query.EvaluatedAt.IsZero() || !isUTC(query.EvaluatedAt) {
		return EdgeQuery{}, nil, fmt.Errorf("%w: evaluatedAt must be a UTC instant", catalog.ErrInvalidQuery)
	}
	query.EvaluatedAt = query.EvaluatedAt.UTC()
	if query.CapabilityKey != "" && !catalog.IsLocalKey(query.CapabilityKey) {
		return EdgeQuery{}, nil, fmt.Errorf("%w: capability key", catalog.ErrInvalidQuery)
	}
	if query.PageSize == 0 {
		query.PageSize = DefaultEdgePageSize
	}
	if query.PageSize < 1 || query.PageSize > MaxEdgePageSize {
		return EdgeQuery{}, nil, fmt.Errorf(
			"%w: page size must be between 1 and %d",
			catalog.ErrInvalidQuery,
			MaxEdgePageSize,
		)
	}
	if query.Cursor == "" {
		return query, nil, nil
	}

	after, err := decodeImpactEdgeCursor(query, query.Cursor)
	if err != nil {
		return EdgeQuery{}, nil, err
	}

	return query, &after, nil
}

func encodeImpactEdgeCursor(query EdgeQuery, after EdgeIdentity) (string, error) {
	if err := validateImpactEdgeIdentity(after); err != nil {
		return "", catalog.ErrUnavailable
	}
	queryHash, err := impactEdgeQueryHash(query)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(impactEdgeCursor{
		Version:   impactEdgeCursorVersion,
		QueryHash: queryHash,
		After: impactEdgeCursorIdentity{
			CapabilityKey:        after.CapabilityKey,
			Provider:             after.Test.TestRepository.Provider,
			Host:                 after.Test.TestRepository.Host,
			ProviderRepositoryID: after.Test.TestRepository.ProviderRepositoryID,
			SuiteKey:             after.Test.SuiteKey,
			TestKey:              after.Test.TestKey,
		},
	})
	if err != nil {
		return "", fmt.Errorf("encode impact edge cursor: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeImpactEdgeCursor(query EdgeQuery, encoded string) (EdgeIdentity, error) {
	if len(encoded) > maxImpactEdgeCursorLength {
		return EdgeIdentity{}, fmt.Errorf("%w: token length", catalog.ErrInvalidCursor)
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return EdgeIdentity{}, fmt.Errorf("%w: token encoding", catalog.ErrInvalidCursor)
	}

	var cursor impactEdgeCursor
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return EdgeIdentity{}, fmt.Errorf("%w: token document", catalog.ErrInvalidCursor)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return EdgeIdentity{}, err
	}
	if cursor.Version != impactEdgeCursorVersion {
		return EdgeIdentity{}, fmt.Errorf("%w: token version", catalog.ErrInvalidCursor)
	}

	wantHash, err := impactEdgeQueryHash(query)
	if err != nil {
		return EdgeIdentity{}, err
	}
	if subtle.ConstantTimeCompare([]byte(cursor.QueryHash), []byte(wantHash)) != 1 {
		return EdgeIdentity{}, fmt.Errorf("%w: token query", catalog.ErrInvalidCursor)
	}
	identity := EdgeIdentity{
		CapabilityKey: cursor.After.CapabilityKey,
		Test: catalog.TestIdentity{
			TestRepository: catalog.RepositoryIdentity{
				Provider:             cursor.After.Provider,
				Host:                 cursor.After.Host,
				ProviderRepositoryID: cursor.After.ProviderRepositoryID,
			},
			SuiteKey: cursor.After.SuiteKey,
			TestKey:  cursor.After.TestKey,
		},
	}
	if err := validateImpactEdgeIdentity(identity); err != nil {
		return EdgeIdentity{}, fmt.Errorf("%w: token position", catalog.ErrInvalidCursor)
	}

	return identity, nil
}

func impactEdgeQueryHash(query EdgeQuery) (string, error) {
	payload, err := json.Marshal(impactEdgeCursorBinding{
		Namespace:            impactEdgeCursorNamespace,
		Provider:             query.Snapshot.Repository.Provider,
		Host:                 query.Snapshot.Repository.Host,
		ProviderRepositoryID: query.Snapshot.Repository.ProviderRepositoryID,
		RevisionAlgorithm:    query.Snapshot.Revision.Algorithm,
		RevisionDigest:       query.Snapshot.Revision.Digest,
		DescriptorAPIVersion: query.Snapshot.APIVersion,
		EvaluatedAt:          query.EvaluatedAt.UTC().Format(time.RFC3339Nano),
		CapabilityKey:        query.CapabilityKey,
	})
	if err != nil {
		return "", fmt.Errorf("bind impact edge cursor: %w", err)
	}
	digest := sha256.Sum256(payload)

	return hex.EncodeToString(digest[:]), nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}

	return fmt.Errorf("%w: token document", catalog.ErrInvalidCursor)
}
