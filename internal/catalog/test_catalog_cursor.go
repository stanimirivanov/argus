package catalog

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
	"regexp"
	"strings"
)

const (
	testCatalogCursorVersion   = 1
	testCatalogCursorNamespace = "argus/test-catalog-query/v1"
	maxTestCatalogCursorLength = 4096
)

var localKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,62}$`)

type testCatalogCursor struct {
	Version   int                       `json:"v"`
	QueryHash string                    `json:"q"`
	After     testCatalogCursorIdentity `json:"after"`
}

type testCatalogCursorIdentity struct {
	Provider             Provider `json:"provider"`
	Host                 string   `json:"host"`
	ProviderRepositoryID string   `json:"providerRepositoryId"`
	SuiteKey             string   `json:"suiteKey"`
	TestKey              string   `json:"testKey"`
}

type testCatalogCursorBinding struct {
	Namespace            string            `json:"namespace"`
	Provider             Provider          `json:"provider"`
	Host                 string            `json:"host"`
	ProviderRepositoryID string            `json:"providerRepositoryId"`
	RevisionAlgorithm    RevisionAlgorithm `json:"revisionAlgorithm"`
	RevisionDigest       string            `json:"revisionDigest"`
	DescriptorAPIVersion string            `json:"descriptorApiVersion"`
	CapabilityKey        string            `json:"capabilityKey"`
}

func normalizeTestCatalogQuery(query TestCatalogQuery) (TestCatalogQuery, *TestCatalogIdentity, error) {
	if err := validateSnapshotKey(query.Snapshot); err != nil {
		return TestCatalogQuery{}, nil, err
	}
	if query.CapabilityKey != "" && !localKeyPattern.MatchString(query.CapabilityKey) {
		return TestCatalogQuery{}, nil, fmt.Errorf("%w: capability key", ErrInvalidQuery)
	}
	if query.PageSize == 0 {
		query.PageSize = DefaultTestCatalogPageSize
	}
	if query.PageSize < 1 || query.PageSize > MaxTestCatalogPageSize {
		return TestCatalogQuery{}, nil, fmt.Errorf("%w: page size must be between 1 and %d", ErrInvalidQuery, MaxTestCatalogPageSize)
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

func validateSnapshotKey(key SnapshotKey) error {
	switch key.Repository.Provider {
	case ProviderGitHub, ProviderGitLab, ProviderAzureDevOps, ProviderOther:
	default:
		return fmt.Errorf("%w: repository provider", ErrInvalidQuery)
	}
	if strings.TrimSpace(key.Repository.Host) == "" ||
		strings.TrimSpace(key.Repository.ProviderRepositoryID) == "" ||
		strings.TrimSpace(key.APIVersion) == "" {
		return fmt.Errorf("%w: incomplete snapshot identity", ErrInvalidQuery)
	}
	validatedRevision, err := NewRevision(key.Revision.Algorithm, key.Revision.Digest)
	if err != nil || validatedRevision != key.Revision {
		return fmt.Errorf("%w: revision", ErrInvalidQuery)
	}

	return nil
}

func encodeTestCatalogCursor(query TestCatalogQuery, after TestCatalogIdentity) (string, error) {
	if err := validateTestCatalogIdentity(after); err != nil {
		return "", ErrUnavailable
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

func decodeTestCatalogCursor(query TestCatalogQuery, encoded string) (TestCatalogIdentity, error) {
	if len(encoded) > maxTestCatalogCursorLength {
		return TestCatalogIdentity{}, fmt.Errorf("%w: token length", ErrInvalidCursor)
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return TestCatalogIdentity{}, fmt.Errorf("%w: token encoding", ErrInvalidCursor)
	}

	var cursor testCatalogCursor
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return TestCatalogIdentity{}, fmt.Errorf("%w: token document", ErrInvalidCursor)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return TestCatalogIdentity{}, err
	}
	if cursor.Version != testCatalogCursorVersion {
		return TestCatalogIdentity{}, fmt.Errorf("%w: token version", ErrInvalidCursor)
	}

	wantHash, err := testCatalogQueryHash(query)
	if err != nil {
		return TestCatalogIdentity{}, err
	}
	if subtle.ConstantTimeCompare([]byte(cursor.QueryHash), []byte(wantHash)) != 1 {
		return TestCatalogIdentity{}, fmt.Errorf("%w: token query", ErrInvalidCursor)
	}
	identity := TestCatalogIdentity{
		TestRepository: RepositoryIdentity{
			Provider:             cursor.After.Provider,
			Host:                 cursor.After.Host,
			ProviderRepositoryID: cursor.After.ProviderRepositoryID,
		},
		SuiteKey: cursor.After.SuiteKey,
		TestKey:  cursor.After.TestKey,
	}
	if err := validateTestCatalogIdentity(identity); err != nil {
		return TestCatalogIdentity{}, err
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

func validateTestCatalogIdentity(identity TestCatalogIdentity) error {
	if strings.TrimSpace(identity.TestRepository.Host) == "" ||
		strings.TrimSpace(identity.TestRepository.ProviderRepositoryID) == "" ||
		!localKeyPattern.MatchString(identity.SuiteKey) ||
		!localKeyPattern.MatchString(identity.TestKey) {
		return fmt.Errorf("%w: token position", ErrInvalidCursor)
	}
	switch identity.TestRepository.Provider {
	case ProviderGitHub, ProviderGitLab, ProviderAzureDevOps, ProviderOther:
		return nil
	default:
		return fmt.Errorf("%w: token position", ErrInvalidCursor)
	}
}

func requireJSONEnd(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}

	return fmt.Errorf("%w: token document", ErrInvalidCursor)
}
