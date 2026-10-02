package githubreview

import (
	"context"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/outcome"
	"github.com/stanimirivanov/argus/internal/adaptation/review"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// SourceReader uses a credential intended only for immutable repository reads.
// It cannot publish a review through its exported method set.
type SourceReader struct{ client *client }

// Publisher owns the separate credential that may create branches, commits,
// and draft pull requests. Its own recovery reads use that same credential.
type Publisher struct{ client *client }

// OutcomeObserver uses a read-only credential to collect terminal review evidence.
type OutcomeObserver struct{ client *client }

var (
	_ review.SourceReader = (*SourceReader)(nil)
	_ review.Publisher    = (*Publisher)(nil)
	_ outcome.Gateway     = (*OutcomeObserver)(nil)
)

// NewSourceReader validates source-read configuration without network access.
func NewSourceReader(options ClientOptions) (*SourceReader, error) {
	client, err := newClient(options)
	if err != nil {
		return nil, err
	}

	return &SourceReader{client: client}, nil
}

// NewPublisher validates review-write configuration without network access.
func NewPublisher(options ClientOptions) (*Publisher, error) {
	client, err := newClient(options)
	if err != nil {
		return nil, err
	}

	return &Publisher{client: client}, nil
}

// NewOutcomeObserver validates review-observation configuration without network access.
func NewOutcomeObserver(options ClientOptions) (*OutcomeObserver, error) {
	client, err := newClient(options)
	if err != nil {
		return nil, err
	}

	return &OutcomeObserver{client: client}, nil
}

// LoadSource returns bytes from one immutable revision after repository checks.
func (reader *SourceReader) LoadSource(
	ctx context.Context,
	repository catalog.Repository,
	revision catalog.Revision,
	path string,
) ([]byte, error) {
	if reader == nil || reader.client == nil {
		return nil, adaptation.ErrUnavailable
	}

	return reader.client.LoadSource(ctx, repository, revision, path)
}

// Publish creates or recovers one deterministic draft review.
func (publisher *Publisher) Publish(
	ctx context.Context,
	request review.PublicationRequest,
) (review.PublishedPullRequest, error) {
	if publisher == nil || publisher.client == nil {
		return review.PublishedPullRequest{}, adaptation.ErrUnavailable
	}

	return publisher.client.Publish(ctx, request)
}

// ObserveOutcome reads one terminal review and its bounded final diff.
func (observer *OutcomeObserver) ObserveOutcome(
	ctx context.Context,
	publication adaptation.ReviewPublication,
) (outcome.TerminalReview, error) {
	if observer == nil || observer.client == nil {
		return outcome.TerminalReview{}, adaptation.ErrUnavailable
	}

	return observer.client.ObserveOutcome(ctx, publication)
}
