// Package githubreview separates GitHub source reads, draft publication, and
// terminal outcome observation behind role-specific concrete adapters.
package githubreview

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/outcome"
	"github.com/stanimirivanov/argus/internal/adaptation/review"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/githubtransport"
)

const (
	defaultAPIVersion = "2026-03-10"
	maxJSONBytes      = 24 * 1024 * 1024
)

// client owns transport and provider-policy mechanics shared by three narrow
// concrete authorities. It is deliberately not exported as an all-purpose adapter.
type client struct {
	httpClient *http.Client
	baseURL    *url.URL
	host       string
	token      string
	apiVersion string
	now        func() time.Time
}

// ClientOptions configures one scoped GitHub.com or GitHub Enterprise credential.
type ClientOptions struct {
	HTTPClient *http.Client
	BaseURL    string
	Host       string
	Token      string
	APIVersion string
	Now        func() time.Time
}

// newClient validates configuration without making a network request.
func newClient(options ClientOptions) (*client, error) {
	baseURL, httpClient, err := githubtransport.Configure(options.BaseURL, options.HTTPClient, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("configure GitHub review client: invalid API URL")
	}
	host := strings.ToLower(strings.TrimSpace(options.Host))
	if host == "" || strings.TrimSpace(options.Token) == "" {
		return nil, fmt.Errorf("configure GitHub review client: host and token are required")
	}
	apiVersion := options.APIVersion
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &client{
		httpClient: httpClient, baseURL: baseURL, host: host,
		token: options.Token, apiVersion: apiVersion, now: now,
	}, nil
}

// LoadSource returns bounded bytes from the exact immutable revision after
// verifying that the current coordinates still identify the cataloged repository.
func (client *client) LoadSource(
	ctx context.Context,
	repository catalog.Repository,
	revision catalog.Revision,
	path string,
) ([]byte, error) {
	if err := client.validateCoordinates(repository, revision, path); err != nil {
		return nil, err
	}
	if err := client.verifyRepository(ctx, repository); err != nil {
		return nil, err
	}
	file, err := client.loadFile(ctx, repository, revision.Digest, path)
	if err != nil {
		return nil, err
	}

	return file.content, nil
}

// Publish creates or recovers a deterministic branch, one source-file commit,
// and an open draft pull request. Existing divergent state is a conflict.
func (client *client) Publish(
	ctx context.Context,
	request review.PublicationRequest,
) (review.PublishedPullRequest, error) {
	if err := client.validateRequest(request); err != nil {
		return review.PublishedPullRequest{}, err
	}
	if err := client.verifyRepository(ctx, request.Repository); err != nil {
		return review.PublishedPullRequest{}, err
	}
	if existing, found, err := client.findPullRequest(ctx, request); err != nil {
		return review.PublishedPullRequest{}, err
	} else if found {
		return client.validateExistingReview(ctx, request, existing)
	}
	branchRevision, err := client.ensureBranch(ctx, request)
	if err != nil {
		return review.PublishedPullRequest{}, err
	}
	file, err := client.loadFile(ctx, request.Repository, request.HeadBranch, request.Path)
	if err != nil {
		return review.PublishedPullRequest{}, err
	}
	candidateSHA := digest(request.Candidate)
	switch digest(file.content) {
	case candidateSHA:
		// A previous attempt committed the candidate before pull-request creation.
	case request.OriginalSHA256:
		if branchRevision != request.BaseRevision.Digest {
			return review.PublishedPullRequest{}, adaptation.ErrReviewConflict
		}
		branchRevision, err = client.updateFile(ctx, request, file.blobSHA)
		if err != nil {
			return review.PublishedPullRequest{}, err
		}
	default:
		return review.PublishedPullRequest{}, adaptation.ErrReviewConflict
	}
	published, err := client.createPullRequest(ctx, request, branchRevision)
	if err == nil {
		return published, nil
	}
	if !errors.Is(err, errUnprocessable) {
		return review.PublishedPullRequest{}, err
	}
	existing, found, findErr := client.findPullRequest(ctx, request)
	if findErr != nil || !found {
		return review.PublishedPullRequest{}, adaptation.ErrUnavailable
	}

	return client.validateExistingReview(ctx, request, existing)
}

// ObserveOutcome resolves one terminal pull request and captures the complete
// bounded diff introduced after Argus' generated head revision.
func (client *client) ObserveOutcome(
	ctx context.Context,
	publication adaptation.ReviewPublication,
) (outcome.TerminalReview, error) {
	if err := adaptation.ValidateReviewPublication(publication); err != nil {
		return outcome.TerminalReview{}, err
	}
	if strings.ToLower(publication.Repository.Identity.Host) != client.host {
		return outcome.TerminalReview{}, adaptation.ErrInvalid
	}
	if err := client.verifyRepository(ctx, publication.Repository); err != nil {
		return outcome.TerminalReview{}, err
	}
	providerReview, err := client.pullRequest(ctx, publication)
	if err != nil {
		return outcome.TerminalReview{}, err
	}
	if providerReview.State != "closed" {
		return outcome.TerminalReview{}, adaptation.ErrReviewNotFinal
	}
	if providerReview.Number != publication.PullRequestNumber ||
		providerReview.HTMLURL != publication.PullRequestURL ||
		providerReview.Base.Ref != publication.BaseBranch || providerReview.Head.Ref != publication.HeadBranch {
		return outcome.TerminalReview{}, adaptation.ErrReviewConflict
	}
	closedAt, err := parseProviderTime(providerReview.ClosedAt)
	if err != nil {
		return outcome.TerminalReview{}, err
	}
	var mergedAt *time.Time
	if providerReview.Merged {
		parsed, err := parseProviderTime(providerReview.MergedAt)
		if err != nil {
			return outcome.TerminalReview{}, err
		}
		mergedAt = &parsed
	} else if providerReview.MergedAt != "" {
		return outcome.TerminalReview{}, adaptation.ErrUnavailable
	}
	finalRevision := revision(publication.HeadRevision.Algorithm, providerReview.Head.SHA)
	if adaptation.ValidateRevision(finalRevision) != nil {
		return outcome.TerminalReview{}, adaptation.ErrUnavailable
	}
	edits := []adaptation.ReviewFileEdit{}
	if finalRevision != publication.HeadRevision {
		edits, err = client.compareReviewerEdits(
			ctx, publication.Repository, publication.HeadRevision, finalRevision,
		)
		if err != nil {
			return outcome.TerminalReview{}, err
		}
	}

	return outcome.TerminalReview{
		FinalRevision: finalRevision, ClosedAt: closedAt, MergedAt: mergedAt,
		ObservedAt: client.now().UTC(), ReviewerEdits: edits,
	}, nil
}

func (client *client) pullRequest(
	ctx context.Context,
	publication adaptation.ReviewPublication,
) (pullRequestResponse, error) {
	var response pullRequestResponse
	path := fmt.Sprintf(
		"%s/pulls/%d", client.repositoryPath(publication.Repository), publication.PullRequestNumber,
	)
	if err := client.getJSON(ctx, path, &response); err != nil {
		return pullRequestResponse{}, err
	}

	return response, nil
}

func (client *client) compareReviewerEdits(
	ctx context.Context,
	repository catalog.Repository,
	generated catalog.Revision,
	final catalog.Revision,
) ([]adaptation.ReviewFileEdit, error) {
	var response compareResponse
	path := fmt.Sprintf(
		"%s/compare/%s...%s?per_page=%d&page=1",
		client.repositoryPath(repository), generated.Digest, final.Digest, adaptation.MaxReviewerEditFiles,
	)
	if err := client.getJSON(ctx, path, &response); err != nil {
		return nil, err
	}
	if response.Status != "ahead" || response.AheadBy < 1 ||
		response.BaseCommit.SHA != generated.Digest || response.MergeBaseCommit.SHA != generated.Digest {
		return nil, adaptation.ErrReviewConflict
	}
	if len(response.Files) >= adaptation.MaxReviewerEditFiles {
		return nil, adaptation.ErrReviewEvidenceIncomplete
	}
	edits := make([]adaptation.ReviewFileEdit, 0, len(response.Files))
	totalPatchBytes := 0
	for _, file := range response.Files {
		edit, err := normalizeReviewerEdit(file)
		if err != nil {
			return nil, err
		}
		totalPatchBytes += len(edit.Patch)
		if totalPatchBytes > adaptation.MaxReviewerTotalPatchBytes {
			return nil, adaptation.ErrReviewEvidenceIncomplete
		}
		edits = append(edits, edit)
	}
	slices.SortFunc(edits, func(left, right adaptation.ReviewFileEdit) int {
		return cmp.Or(cmp.Compare(left.Path, right.Path), cmp.Compare(left.PreviousPath, right.PreviousPath))
	})

	return edits, nil
}

func normalizeReviewerEdit(file compareFileResponse) (adaptation.ReviewFileEdit, error) {
	if file.Patch == nil || len(*file.Patch) > adaptation.MaxReviewerPatchBytes {
		return adaptation.ReviewFileEdit{}, adaptation.ErrReviewEvidenceIncomplete
	}
	kind, err := normalizeReviewerFileKind(file.Status)
	if err != nil {
		return adaptation.ReviewFileEdit{}, err
	}
	edit := adaptation.ReviewFileEdit{
		Path: file.Filename, Kind: kind, Additions: file.Additions,
		Deletions: file.Deletions, Patch: *file.Patch,
	}
	if file.PreviousFilename != nil {
		edit.PreviousPath = *file.PreviousFilename
	}

	return edit, nil
}

func normalizeReviewerFileKind(status string) (adaptation.ReviewFileKind, error) {
	switch status {
	case "added":
		return adaptation.ReviewFileAdded, nil
	case "modified", "changed":
		return adaptation.ReviewFileModified, nil
	case "removed":
		return adaptation.ReviewFileDeleted, nil
	case "renamed":
		return adaptation.ReviewFileRenamed, nil
	case "copied":
		return adaptation.ReviewFileCopied, nil
	default:
		return "", adaptation.ErrReviewEvidenceIncomplete
	}
}

func parseProviderTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, adaptation.ErrUnavailable
	}

	return parsed.UTC(), nil
}

func (client *client) validateRequest(request review.PublicationRequest) error {
	if !isSHA256(request.ReviewID) || request.Repository.Identity.Provider != catalog.ProviderGitHub ||
		adaptation.ValidateRepository(request.Repository) != nil ||
		adaptation.ValidateRevision(request.BaseRevision) != nil ||
		adaptation.ValidateBranchName(request.BaseBranch) != nil ||
		adaptation.ValidateBranchName(request.HeadBranch) != nil || request.BaseBranch == request.HeadBranch ||
		adaptation.ValidateSourcePath(request.Path) != nil || !isSHA256(request.OriginalSHA256) ||
		len(request.Candidate) == 0 || len(request.Candidate) > adaptation.MaxSourceBytes ||
		strings.TrimSpace(request.Title) == "" || len(request.Title) > 256 ||
		strings.TrimSpace(request.Body) == "" || len(request.Body) > 65536 ||
		strings.TrimSpace(request.CommitMessage) == "" || len(request.CommitMessage) > 256 {
		return fmt.Errorf("%w: GitHub review request", adaptation.ErrInvalid)
	}

	return nil
}

func (client *client) validateCoordinates(
	repository catalog.Repository,
	revision catalog.Revision,
	path string,
) error {
	if repository.Identity.Provider != catalog.ProviderGitHub ||
		adaptation.ValidateRepository(repository) != nil || adaptation.ValidateRevision(revision) != nil ||
		adaptation.ValidateSourcePath(path) != nil || strings.ToLower(repository.Identity.Host) != client.host {
		return adaptation.ErrInvalid
	}

	return nil
}

func (client *client) verifyRepository(ctx context.Context, repository catalog.Repository) error {
	if strings.ToLower(repository.Identity.Host) != client.host {
		return adaptation.ErrInvalid
	}
	var response repositoryResponse
	if err := client.getJSON(ctx, client.repositoryPath(repository), &response); err != nil {
		return err
	}
	if response.ID.String() != repository.Identity.ProviderRepositoryID ||
		!strings.EqualFold(response.Owner.Login, repository.Owner) || response.Name != repository.Name {
		return adaptation.ErrReviewConflict
	}

	return nil
}

func (client *client) ensureBranch(ctx context.Context, request review.PublicationRequest) (string, error) {
	path := client.repositoryPath(request.Repository) + "/git/ref/heads/" + escapePath(request.HeadBranch)
	var existing referenceResponse
	err := client.getJSON(ctx, path, &existing)
	if err == nil {
		return existing.Object.SHA, nil
	}
	if !errors.Is(err, errNotFound) {
		return "", err
	}
	payload := struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	}{Ref: "refs/heads/" + request.HeadBranch, SHA: request.BaseRevision.Digest}
	var created referenceResponse
	if err := client.sendJSON(ctx, http.MethodPost, client.repositoryPath(request.Repository)+"/git/refs", payload, &created); err != nil {
		if !errors.Is(err, errUnprocessable) {
			return "", err
		}
		if err := client.getJSON(ctx, path, &existing); err != nil {
			return "", err
		}

		return existing.Object.SHA, nil
	}

	return created.Object.SHA, nil
}

func (client *client) updateFile(
	ctx context.Context,
	request review.PublicationRequest,
	blobSHA string,
) (string, error) {
	payload := struct {
		Message string `json:"message"`
		Content string `json:"content"`
		SHA     string `json:"sha"`
		Branch  string `json:"branch"`
	}{
		Message: request.CommitMessage, Content: base64.StdEncoding.EncodeToString(request.Candidate),
		SHA: blobSHA, Branch: request.HeadBranch,
	}
	var response updateFileResponse
	path := client.repositoryPath(request.Repository) + "/contents/" + escapePath(request.Path)
	if err := client.sendJSON(ctx, http.MethodPut, path, payload, &response); err != nil {
		if errors.Is(err, errConflict) || errors.Is(err, errUnprocessable) {
			return "", adaptation.ErrReviewConflict
		}
		return "", err
	}
	if response.Commit.SHA == "" {
		return "", adaptation.ErrUnavailable
	}

	return response.Commit.SHA, nil
}

func (client *client) createPullRequest(
	ctx context.Context,
	request review.PublicationRequest,
	expectedHeadRevision string,
) (review.PublishedPullRequest, error) {
	payload := struct {
		Title string `json:"title"`
		Head  string `json:"head"`
		Base  string `json:"base"`
		Body  string `json:"body"`
		Draft bool   `json:"draft"`
	}{Title: request.Title, Head: request.HeadBranch, Base: request.BaseBranch, Body: request.Body, Draft: true}
	var response pullRequestResponse
	if err := client.sendJSON(
		ctx, http.MethodPost, client.repositoryPath(request.Repository)+"/pulls", payload, &response,
	); err != nil {
		return review.PublishedPullRequest{}, err
	}
	if response.Base.Ref != request.BaseBranch || response.Head.Ref != request.HeadBranch ||
		response.Head.SHA != expectedHeadRevision {
		return review.PublishedPullRequest{}, adaptation.ErrReviewConflict
	}

	return normalizePullRequest(response, request.BaseRevision.Algorithm)
}

func (client *client) findPullRequest(
	ctx context.Context,
	request review.PublicationRequest,
) (pullRequestResponse, bool, error) {
	query := url.Values{}
	query.Set("state", "all")
	query.Set("head", request.Repository.Owner+":"+request.HeadBranch)
	query.Set("per_page", "2")
	var response []pullRequestResponse
	if err := client.getJSON(
		ctx, client.repositoryPath(request.Repository)+"/pulls?"+query.Encode(), &response,
	); err != nil {
		return pullRequestResponse{}, false, err
	}
	if len(response) > 1 {
		return pullRequestResponse{}, false, adaptation.ErrReviewConflict
	}
	if len(response) == 0 {
		return pullRequestResponse{}, false, nil
	}

	return response[0], true, nil
}

func (client *client) validateExistingReview(
	ctx context.Context,
	request review.PublicationRequest,
	existing pullRequestResponse,
) (review.PublishedPullRequest, error) {
	if existing.State != "open" || !existing.Draft || existing.Base.Ref != request.BaseBranch ||
		existing.Head.Ref != request.HeadBranch {
		return review.PublishedPullRequest{}, adaptation.ErrReviewConflict
	}
	file, err := client.loadFile(ctx, request.Repository, existing.Head.SHA, request.Path)
	if err != nil {
		return review.PublishedPullRequest{}, err
	}
	if digest(file.content) != digest(request.Candidate) {
		return review.PublishedPullRequest{}, adaptation.ErrReviewConflict
	}

	return normalizePullRequest(existing, request.BaseRevision.Algorithm)
}

func normalizePullRequest(
	response pullRequestResponse,
	algorithm catalog.RevisionAlgorithm,
) (review.PublishedPullRequest, error) {
	createdAt, err := time.Parse(time.RFC3339, response.CreatedAt)
	if err != nil || response.Number < 1 || response.HTMLURL == "" || response.Head.SHA == "" {
		return review.PublishedPullRequest{}, adaptation.ErrUnavailable
	}
	result := review.PublishedPullRequest{
		Number: response.Number, URL: response.HTMLURL, HeadRevision: revision(algorithm, response.Head.SHA),
		Draft: response.Draft, State: response.State, CreatedAt: createdAt,
	}
	if adaptation.ValidateRevision(result.HeadRevision) != nil {
		return review.PublishedPullRequest{}, adaptation.ErrUnavailable
	}

	return result, nil
}

type loadedFile struct {
	content []byte
	blobSHA string
}

func (client *client) loadFile(
	ctx context.Context,
	repository catalog.Repository,
	reference string,
	path string,
) (loadedFile, error) {
	relativePath := client.repositoryPath(repository) + "/contents/" + escapePath(path) + "?ref=" + url.QueryEscape(reference)
	var metadata contentResponse
	if err := client.getJSON(ctx, relativePath, &metadata); err != nil {
		return loadedFile{}, err
	}
	if metadata.Type != "file" || metadata.SHA == "" || metadata.Size < 0 || metadata.Size > adaptation.MaxSourceBytes {
		return loadedFile{}, adaptation.ErrInvalid
	}
	data, err := client.getBytes(ctx, relativePath, "application/vnd.github.raw+json", adaptation.MaxSourceBytes)
	if err != nil {
		return loadedFile{}, err
	}
	if len(data) != metadata.Size {
		return loadedFile{}, adaptation.ErrUnavailable
	}

	return loadedFile{content: data, blobSHA: metadata.SHA}, nil
}

func (client *client) repositoryPath(repository catalog.Repository) string {
	return "repos/" + url.PathEscape(repository.Owner) + "/" + url.PathEscape(repository.Name)
}

func (client *client) getJSON(ctx context.Context, relativePath string, target any) error {
	data, err := client.getBytes(ctx, relativePath, "application/vnd.github+json", maxJSONBytes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return adaptation.ErrUnavailable
	}

	return nil
}

func (client *client) getBytes(
	ctx context.Context,
	relativePath string,
	accept string,
	maxBytes int,
) ([]byte, error) {
	request, err := client.newRequest(ctx, http.MethodGet, relativePath, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", accept)
	response, err := client.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, adaptation.ErrUnavailable
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // Response processing already has an authoritative error.
	if err := responseError(response.StatusCode); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(maxBytes)+1))
	if err != nil || len(data) > maxBytes {
		return nil, adaptation.ErrUnavailable
	}

	return data, nil
}

func (client *client) sendJSON(
	ctx context.Context,
	method string,
	relativePath string,
	payload any,
	target any,
) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return adaptation.ErrInvalid
	}
	request, err := client.newRequest(ctx, method, relativePath, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Content-Type", "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return adaptation.ErrUnavailable
	}
	defer func() { _ = response.Body.Close() }() //nolint:errcheck // Response processing already has an authoritative error.
	if err := responseError(response.StatusCode); err != nil {
		return err
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, maxJSONBytes+1))
	if err != nil || len(data) > maxJSONBytes {
		return adaptation.ErrUnavailable
	}
	if err := json.Unmarshal(data, target); err != nil {
		return adaptation.ErrUnavailable
	}

	return nil
}

func (client *client) newRequest(
	ctx context.Context,
	method string,
	relativePath string,
	body io.Reader,
) (*http.Request, error) {
	endpoint, err := githubtransport.ResolvePath(client.baseURL, relativePath)
	if err != nil {
		return nil, adaptation.ErrInvalid
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, adaptation.ErrInvalid
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("X-GitHub-Api-Version", client.apiVersion)

	return request, nil
}

var (
	errNotFound      = errors.New("GitHub resource not found")
	errConflict      = errors.New("GitHub write conflict")
	errUnprocessable = errors.New("GitHub request unprocessable")
)

func responseError(status int) error {
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return nil
	case status == http.StatusNotFound:
		return errNotFound
	case status == http.StatusConflict:
		return errConflict
	case status == http.StatusUnprocessableEntity:
		return errUnprocessable
	default:
		return adaptation.ErrUnavailable
	}
}

func escapePath(value string) string {
	segments := strings.Split(value, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}

	return strings.Join(segments, "/")
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)

	return err == nil && strings.ToLower(value) == value
}

func revision(algorithm catalog.RevisionAlgorithm, digest string) catalog.Revision {
	return catalog.Revision{Algorithm: algorithm, Digest: digest}
}

type repositoryResponse struct {
	ID    json.Number `json:"id"`
	Name  string      `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
}

type contentResponse struct {
	Type string `json:"type"`
	SHA  string `json:"sha"`
	Size int    `json:"size"`
}

type referenceResponse struct {
	Object struct {
		SHA string `json:"sha"`
	} `json:"object"`
}

type updateFileResponse struct {
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

type pullRequestResponse struct {
	Number    int    `json:"number"`
	HTMLURL   string `json:"html_url"`
	State     string `json:"state"`
	Draft     bool   `json:"draft"`
	CreatedAt string `json:"created_at"`
	ClosedAt  string `json:"closed_at"`
	MergedAt  string `json:"merged_at"`
	Merged    bool   `json:"merged"`
	Base      struct {
		Ref string `json:"ref"`
	} `json:"base"`
	Head struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
}

type compareResponse struct {
	Status     string `json:"status"`
	AheadBy    int    `json:"ahead_by"`
	BaseCommit struct {
		SHA string `json:"sha"`
	} `json:"base_commit"`
	MergeBaseCommit struct {
		SHA string `json:"sha"`
	} `json:"merge_base_commit"`
	Files []compareFileResponse `json:"files"`
}

type compareFileResponse struct {
	Filename         string  `json:"filename"`
	PreviousFilename *string `json:"previous_filename"`
	Status           string  `json:"status"`
	Additions        int     `json:"additions"`
	Deletions        int     `json:"deletions"`
	Patch            *string `json:"patch"`
}
