package testquery

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

	"github.com/stanimirivanov/argus/internal/catalog"
)

const (
	testCatalogCursorVersion   = 1
	testCatalogCursorNamespace = "argus/test-catalog-query/v1"
	maxTestCatalogCursorLength = 4096
)

type testCatalogCursor struct {
	Version   int                       `json:"v"`
	QueryHash string                    `json:"q"`
	After     testCatalogCursorIdentity `json:"after"`
}

type testCatalogCursorIdentity struct {
	Provider             catalog.Provider `json:"provider"`
	Host                 string           `json:"host"`
	ProviderRepositoryID string           `json:"providerRepositoryId"`
	SuiteKey             string           `json:"suiteKey"`
	TestKey              string           `json:"testKey"`
}

type testCatalogCursorBinding struct {
	Namespace            string                    `json:"namespace"`
	Provider             catalog.Provider          `json:"provider"`
	Host                 string                    `json:"host"`
	ProviderRepositoryID string                    `json:"providerRepositoryId"`
	RevisionAlgorithm    catalog.RevisionAlgorithm `json:"revisionAlgorithm"`
	RevisionDigest       string                    `json:"revisionDigest"`
	DescriptorAPIVersion string                    `json:"descriptorApiVersion"`
	CapabilityKey        string                    `json:"capabilityKey"`
}

func normalizeTestCatalogQuery(query TestCatalogQuery) (TestCatalogQuery, *catalog.TestIdentity, error) {
	if err := catalog.ValidateSnapshotKey(query.Snapshot); err != nil {
		return TestCatalogQuery{}, nil, err
	}
	if query.CapabilityKey != "" && !catalog.IsLocalKey(query.CapabilityKey) {
		return TestCatalogQuery{}, nil, fmt.Errorf("%w: capability key", catalog.ErrInvalidQuery)
	}
	if query.PageSize == 0 {
		query.PageSize = DefaultTestCatalogPageSize
	}
	if query.PageSize < 1 || query.PageSize > MaxTestCatalogPageSize {
		return TestCatalogQuery{}, nil, fmt.Errorf("%w: page size must be between 1 and %d", catalog.ErrInvalidQuery, MaxTestCatalogPageSize)
	}
	if query.Cursor == "" {
		return query, nil, nil
	}

	after, err := decodeTestCatalogCursor(query, query.Cursor)
	if err != nil {
		return TestCatalogQuery{}, nil, err
	}

	return query, &after, nil
}

func encodeTestCatalogCursor(query TestCatalogQuery, after catalog.TestIdentity) (string, error) {
	if !after.Valid() {
		return "", catalog.ErrUnavailable
	}
	queryHash, err := testCatalogQueryHash(query)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(testCatalogCursor{
		Version:   testCatalogCursorVersion,
		QueryHash: queryHash,
		After: testCatalogCursorIdentity{
			Provider:             after.TestRepository.Provider,
			Host:                 after.TestRepository.Host,
			ProviderRepositoryID: after.TestRepository.ProviderRepositoryID,
			SuiteKey:             after.SuiteKey,
			TestKey:              after.TestKey,
		},
	})
	if err != nil {
		return "", fmt.Errorf("encode catalog cursor: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeTestCatalogCursor(query TestCatalogQuery, encoded string) (catalog.TestIdentity, error) {
	if len(encoded) > maxTestCatalogCursorLength {
		return catalog.TestIdentity{}, fmt.Errorf("%w: token length", catalog.ErrInvalidCursor)
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return catalog.TestIdentity{}, fmt.Errorf("%w: token encoding", catalog.ErrInvalidCursor)
	}

	var cursor testCatalogCursor
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return catalog.TestIdentity{}, fmt.Errorf("%w: token document", catalog.ErrInvalidCursor)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return catalog.TestIdentity{}, err
	}
	if cursor.Version != testCatalogCursorVersion {
		return catalog.TestIdentity{}, fmt.Errorf("%w: token version", catalog.ErrInvalidCursor)
	}

	wantHash, err := testCatalogQueryHash(query)
	if err != nil {
		return catalog.TestIdentity{}, err
	}
	if subtle.ConstantTimeCompare([]byte(cursor.QueryHash), []byte(wantHash)) != 1 {
		return catalog.TestIdentity{}, fmt.Errorf("%w: token query", catalog.ErrInvalidCursor)
	}
	identity := catalog.TestIdentity{
		TestRepository: catalog.RepositoryIdentity{
			Provider:             cursor.After.Provider,
			Host:                 cursor.After.Host,
			ProviderRepositoryID: cursor.After.ProviderRepositoryID,
		},
		SuiteKey: cursor.After.SuiteKey,
		TestKey:  cursor.After.TestKey,
	}
	if !identity.Valid() {
		return catalog.TestIdentity{}, fmt.Errorf("%w: token position", catalog.ErrInvalidCursor)
	}

	return identity, nil
}

func testCatalogQueryHash(query TestCatalogQuery) (string, error) {
	payload, err := json.Marshal(testCatalogCursorBinding{
		Namespace:            testCatalogCursorNamespace,
		Provider:             query.Snapshot.Repository.Provider,
		Host:                 query.Snapshot.Repository.Host,
		ProviderRepositoryID: query.Snapshot.Repository.ProviderRepositoryID,
		RevisionAlgorithm:    query.Snapshot.Revision.Algorithm,
		RevisionDigest:       query.Snapshot.Revision.Digest,
		DescriptorAPIVersion: query.Snapshot.APIVersion,
		CapabilityKey:        query.CapabilityKey,
	})
	if err != nil {
		return "", fmt.Errorf("bind catalog cursor: %w", err)
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
