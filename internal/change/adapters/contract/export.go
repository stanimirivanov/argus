// Package contract converts the change domain into versioned public transport
// documents. It does not own change semantics or provider behavior.
package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/change"
)

// ExportV1 converts a validated change set to the Effect-authored v1 contract.
func ExportV1(set change.Set) (contracts.ChangeSetV1, error) {
	set = change.CanonicalSet(set)
	if err := change.ValidateSet(set); err != nil {
		return contracts.ChangeSetV1{}, err
	}

	files := make([]contracts.ChangeFile, 0, len(set.Files))
	for _, file := range set.Files {
		files = append(files, contracts.ChangeFile{
			Path:         file.Path,
			PreviousPath: cloneString(file.PreviousPath),
			Kind:         string(file.Kind),
			Additions:    file.Additions,
			Deletions:    file.Deletions,
			Patch:        cloneString(file.Patch),
			PatchStatus:  string(file.PatchStatus),
		})
	}
	document := contracts.ChangeSetV1{
		APIVersion: contracts.ChangeSetV1APIVersion,
		SourceRepository: contracts.RepositoryReference{
			Provider:             string(set.SourceRepository.Identity.Provider),
			Host:                 set.SourceRepository.Identity.Host,
			ProviderRepositoryID: set.SourceRepository.Identity.ProviderRepositoryID,
			Owner:                set.SourceRepository.Owner,
			Name:                 set.SourceRepository.Name,
		},
		PullRequest: contracts.PullRequestReference{Number: set.PullRequestNumber},
		BaseRevision: contracts.RevisionReference{
			Algorithm: string(set.BaseRevision.Algorithm),
			Digest:    set.BaseRevision.Digest,
		},
		HeadRevision: contracts.RevisionReference{
			Algorithm: string(set.HeadRevision.Algorithm),
			Digest:    set.HeadRevision.Digest,
		},
		ObservedAt: set.ObservedAt.Format(time.RFC3339Nano),
		Trigger: contracts.ChangeTrigger{
			Provider:   string(set.Trigger.Provider),
			DeliveryID: set.Trigger.DeliveryID,
			Event:      set.Trigger.Event,
			Action:     set.Trigger.Action,
		},
		Files:          files,
		FilesTruncated: set.FilesTruncated,
	}
	if err := contracts.ValidateChangeSetV1(document); err != nil {
		return contracts.ChangeSetV1{}, fmt.Errorf("export change set v1: %w", err)
	}

	return document, nil
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value

	return &cloned
}
