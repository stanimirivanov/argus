package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/contracts"
)

// ExportReviewV1 converts a provider-confirmed review to its public contract.
func ExportReviewV1(publication adaptation.ReviewPublication) (contracts.AdaptationReviewV1, error) {
	publication.PublishedAt = publication.PublishedAt.UTC()
	if err := adaptation.ValidateReviewPublication(publication); err != nil {
		return contracts.AdaptationReviewV1{}, err
	}
	document := contracts.AdaptationReviewV1{
		APIVersion: publication.APIVersion, ReviewID: publication.ReviewID,
		ProposalID: publication.ProposalID, ValidationID: publication.ValidationID,
		Repository: exportRepository(publication.Repository), Provider: publication.Provider,
		Base: contracts.ReviewBranch{
			Branch: publication.BaseBranch, Revision: exportRevision(publication.BaseRevision),
		},
		Head: contracts.ReviewBranch{
			Branch: publication.HeadBranch, Revision: exportRevision(publication.HeadRevision),
		},
		PullRequest: contracts.ReviewPullRequest{
			Number: publication.PullRequestNumber, URL: publication.PullRequestURL,
			Draft: publication.Draft, State: publication.State,
		},
		PublishedAt: publication.PublishedAt.Format(time.RFC3339Nano),
	}
	if err := contracts.ValidateAdaptationReviewV1(document); err != nil {
		return contracts.AdaptationReviewV1{}, fmt.Errorf("export adaptation review: %w", err)
	}

	return document, nil
}

// ImportReviewV1 converts a structurally valid review contract to the domain model.
func ImportReviewV1(document contracts.AdaptationReviewV1) (adaptation.ReviewPublication, error) {
	publishedAt, err := time.Parse(time.RFC3339Nano, document.PublishedAt)
	if err != nil {
		return adaptation.ReviewPublication{}, fmt.Errorf("parse adaptation review publication time: %w", err)
	}
	publication := adaptation.ReviewPublication{
		APIVersion: document.APIVersion, ReviewID: document.ReviewID,
		ProposalID: document.ProposalID, ValidationID: document.ValidationID,
		Repository: importRepository(document.Repository), Provider: document.Provider,
		BaseBranch: document.Base.Branch, BaseRevision: importRevision(document.Base.Revision),
		HeadBranch: document.Head.Branch, HeadRevision: importRevision(document.Head.Revision),
		PullRequestNumber: document.PullRequest.Number, PullRequestURL: document.PullRequest.URL,
		Draft: document.PullRequest.Draft, State: document.PullRequest.State, PublishedAt: publishedAt,
	}
	if err := adaptation.ValidateReviewPublication(publication); err != nil {
		return adaptation.ReviewPublication{}, fmt.Errorf("validate imported adaptation review: %w", err)
	}

	return publication, nil
}
