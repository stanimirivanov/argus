package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

const (
	defaultAPIVersion   = "2026-03-10"
	defaultMaxBodyBytes = 4 * 1024 * 1024
	maxDocumentBytes    = 5 * 1024 * 1024
	maxTotalPatchBytes  = 2 * 1024 * 1024
	filesPerPage        = 100
)

// Client resolves immutable pull-request metadata and bounded changed-file
// evidence using the GitHub REST API.
type Client struct {
	httpClient   *http.Client
	baseURL      *url.URL
	token        string
	apiVersion   string
	maxBodyBytes int64
}

// ClientOptions configures a GitHub or GitHub Enterprise REST endpoint.
type ClientOptions struct {
	HTTPClient *http.Client
	BaseURL    string
	Token      string
	APIVersion string
}

// NewClient validates configuration without making a network request.
func NewClient(options ClientOptions) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(options.BaseURL, "/") + "/")
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("configure GitHub client: invalid API URL")
	}
	if baseURL.Scheme != "https" && baseURL.Hostname() != "127.0.0.1" && baseURL.Hostname() != "localhost" {
		return nil, fmt.Errorf("configure GitHub client: API URL must use HTTPS")
	}
	if strings.TrimSpace(options.Token) == "" {
		return nil, fmt.Errorf("configure GitHub client: token is required")
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	apiVersion := options.APIVersion
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}

	return &Client{
		httpClient:   httpClient,
		baseURL:      baseURL,
		token:        options.Token,
		apiVersion:   apiVersion,
		maxBodyBytes: defaultMaxBodyBytes,
	}, nil
}

// Resolve reads the pull request before and after paginating its files. Any
// revision or file-count movement rejects the observation as stale instead of
// attributing a mixed response to the signed delivery.
func (client *Client) Resolve(ctx context.Context, delivery ingest.Delivery) (change.Set, error) {
	before, err := client.pullRequest(ctx, delivery)
	if err != nil {
		return change.Set{}, err
	}
	if err := matchesDelivery(before, delivery); err != nil {
		return change.Set{}, err
	}

	files, truncated, err := client.changedFiles(ctx, delivery, before.ChangedFiles)
	if err != nil {
		return change.Set{}, err
	}
	after, err := client.pullRequest(ctx, delivery)
	if err != nil {
		return change.Set{}, err
	}
	if before.Base.SHA != after.Base.SHA || before.Head.SHA != after.Head.SHA ||
		before.ChangedFiles != after.ChangedFiles {
		return change.Set{}, change.ErrStale
	}

	set := change.CanonicalSet(change.Set{
		APIVersion:        change.SetAPIVersion,
		SourceRepository:  delivery.Repository,
		PullRequestNumber: delivery.PullRequestNumber,
		BaseRevision:      delivery.BaseRevision,
		HeadRevision:      delivery.HeadRevision,
		ObservedAt:        delivery.ObservedAt,
		Trigger: change.Trigger{
			Provider:   delivery.Provider,
			DeliveryID: delivery.ID,
			Event:      delivery.Event,
			Action:     delivery.Action,
		},
		Files:          files,
		FilesTruncated: truncated,
	})
	if err := change.ValidateSet(set); err != nil {
		return change.Set{}, fmt.Errorf("normalize GitHub files: %w", err)
	}

	return set, nil
}

func (client *Client) pullRequest(ctx context.Context, delivery ingest.Delivery) (pullRequestResponse, error) {
	var response pullRequestResponse
	path := fmt.Sprintf(
		"repos/%s/%s/pulls/%d",
		url.PathEscape(delivery.Repository.Owner),
		url.PathEscape(delivery.Repository.Name),
		delivery.PullRequestNumber,
	)
	if err := client.getJSON(ctx, path, &response); err != nil {
		return pullRequestResponse{}, err
	}

	return response, nil
}

func (client *Client) changedFiles(
	ctx context.Context,
	delivery ingest.Delivery,
	providerCount int,
) ([]change.File, bool, error) {
	if providerCount < 0 {
		return nil, false, change.ErrUnavailable
	}
	limit := min(providerCount, change.MaxFiles)
	files := make([]change.File, 0, limit)
	remainingPatchBytes := maxTotalPatchBytes
	for page := 1; len(files) < limit; page++ {
		var response []fileResponse
		path := fmt.Sprintf(
			"repos/%s/%s/pulls/%d/files?per_page=%d&page=%d",
			url.PathEscape(delivery.Repository.Owner),
			url.PathEscape(delivery.Repository.Name),
			delivery.PullRequestNumber,
			filesPerPage,
			page,
		)
		if err := client.getJSON(ctx, path, &response); err != nil {
			return nil, false, err
		}
		if len(response) == 0 {
			return nil, false, change.ErrUnavailable
		}
		if len(response) > filesPerPage || len(response) > limit-len(files) {
			return nil, false, change.ErrUnavailable
		}
		for _, providerFile := range response {
			if len(files) == limit {
				break
			}
			file, consumed, err := normalizeFile(providerFile, remainingPatchBytes)
			if err != nil {
				return nil, false, err
			}
			remainingPatchBytes -= consumed
			files = append(files, file)
		}
	}

	return files, providerCount > change.MaxFiles, nil
}

func (client *Client) getJSON(ctx context.Context, relativePath string, target any) error {
	data, err := client.get(ctx, relativePath, "application/vnd.github+json", client.maxBodyBytes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return change.ErrUnavailable
	}

	return nil
}

// LoadDocument reads a repository file at an immutable revision using the raw
// contents media type. Path segments are escaped independently so nested
// repository paths remain nested while reserved characters cannot alter the
// request URL.
func (client *Client) LoadDocument(
	ctx context.Context,
	repository catalog.Repository,
	revision catalog.Revision,
	path string,
) ([]byte, error) {
	if repository.Identity.Provider != catalog.ProviderGitHub || repository.Owner == "" || repository.Name == "" {
		return nil, change.ErrInvalid
	}
	validatedRevision, err := catalog.NewRevision(revision.Algorithm, revision.Digest)
	if err != nil || validatedRevision != revision || !validDocumentPath(path) {
		return nil, change.ErrInvalid
	}
	segments := strings.Split(path, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	relativePath := fmt.Sprintf(
		"repos/%s/%s/contents/%s?ref=%s",
		url.PathEscape(repository.Owner),
		url.PathEscape(repository.Name),
		strings.Join(segments, "/"),
		url.QueryEscape(revision.Digest),
	)

	return client.get(ctx, relativePath, "application/vnd.github.raw+json", maxDocumentBytes)
}

func (client *Client) get(ctx context.Context, relativePath, accept string, maxBytes int64) ([]byte, error) {
	reference, err := url.Parse(relativePath)
	if err != nil {
		return nil, change.ErrUnavailable
	}
	endpoint := client.baseURL.ResolveReference(reference)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, change.ErrUnavailable
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("X-GitHub-Api-Version", client.apiVersion)

	response, err := client.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, change.ErrUnavailable
	}
	defer func() {
		_ = response.Body.Close() //nolint:errcheck // The request result is already complete.
	}()
	if response.StatusCode == http.StatusNotFound {
		return nil, change.ErrNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, change.ErrUnavailable
	}

	limited := io.LimitReader(response.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil || int64(len(data)) > maxBytes {
		return nil, change.ErrUnavailable
	}

	return data, nil
}

func validDocumentPath(path string) bool {
	if path == "" || len(path) > 4096 || strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") ||
		strings.Contains(path, "\\") || strings.Contains(path, "//") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}

	return true
}

func matchesDelivery(response pullRequestResponse, delivery ingest.Delivery) error {
	if response.Number != delivery.PullRequestNumber || strconv.FormatInt(response.Base.Repository.ID, 10) !=
		delivery.Repository.Identity.ProviderRepositoryID ||
		response.Base.SHA != delivery.BaseRevision.Digest || response.Head.SHA != delivery.HeadRevision.Digest {
		return change.ErrStale
	}

	return nil
}

func normalizeFile(providerFile fileResponse, remaining int) (change.File, int, error) {
	kind, err := normalizeKind(providerFile.Status)
	if err != nil {
		return change.File{}, 0, err
	}
	file := change.File{
		Path:         providerFile.Filename,
		PreviousPath: providerFile.PreviousFilename,
		Kind:         kind,
		Additions:    providerFile.Additions,
		Deletions:    providerFile.Deletions,
	}
	if providerFile.Patch == nil {
		file.PatchStatus = change.PatchUnavailable
		return file, 0, nil
	}
	if remaining <= 0 {
		file.PatchStatus = change.PatchBudgetExhausted
		return file, 0, nil
	}
	limit := min(change.MaxPatchBytes, remaining)
	patch, truncated := utf8Prefix(*providerFile.Patch, limit)
	if patch == "" && *providerFile.Patch != "" {
		file.PatchStatus = change.PatchBudgetExhausted
		return file, 0, nil
	}
	file.Patch = &patch
	file.PatchStatus = change.PatchComplete
	if truncated {
		file.PatchStatus = change.PatchTruncated
	}

	return file, len(patch), nil
}

func normalizeKind(status string) (change.Kind, error) {
	switch status {
	case "added":
		return change.KindAdded, nil
	case "modified", "changed":
		return change.KindModified, nil
	case "removed":
		return change.KindDeleted, nil
	case "renamed":
		return change.KindRenamed, nil
	case "copied":
		return change.KindCopied, nil
	default:
		return "", fmt.Errorf("%w: unsupported GitHub file status", change.ErrInvalid)
	}
}

func utf8Prefix(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	if limit <= 0 {
		return "", true
	}
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}

	return value[:end], true
}

type pullRequestResponse struct {
	Number       int `json:"number"`
	ChangedFiles int `json:"changed_files"`
	Base         struct {
		SHA        string `json:"sha"`
		Repository struct {
			ID int64 `json:"id"`
		} `json:"repo"`
	} `json:"base"`
	Head struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

type fileResponse struct {
	Filename         string  `json:"filename"`
	PreviousFilename *string `json:"previous_filename"`
	Status           string  `json:"status"`
	Additions        int     `json:"additions"`
	Deletions        int     `json:"deletions"`
	Patch            *string `json:"patch"`
}
