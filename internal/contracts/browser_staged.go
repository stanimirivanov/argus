package contracts

// BrowserStagedProvenanceV2 binds a browser invocation to complete discovery.
type BrowserStagedProvenanceV2 struct {
	StagedProvenanceV2
	DiscoverySHA256 string `json:"discoverySha256"`
}

// BrowserAdapterRequestV2 includes the independent complete project inventory.
type BrowserAdapterRequestV2 struct {
	BrowserAdapterRequestV1
	Staging    BrowserStagedProvenanceV2    `json:"staging"`
	Candidates []RequestedFunctionalAPITest `json:"candidates"`
}

// BrowserExecutionAttemptV2 preserves both exact executed and complete sets.
type BrowserExecutionAttemptV2 struct {
	BrowserExecutionAttemptV1
	Staging    BrowserStagedProvenanceV2    `json:"staging"`
	Candidates []RequestedFunctionalAPITest `json:"candidates"`
}

// BrowserStageGroupV2 partitions every discovered variant in one reviewed scope.
type BrowserStageGroupV2 struct {
	BrowserExecutionGroupBinding
	Candidates     []StagedCandidateV2 `json:"candidates"`
	RelevantNow    []string            `json:"relevantNow"`
	RemainingLater []string            `json:"remainingLater"`
}

// BrowserStagePlanV2 identifies exact schedule, manifest and discovery bytes.
type BrowserStagePlanV2 struct {
	APIVersion      string                  `json:"apiVersion"`
	Schedule        ManifestDigestReference `json:"schedule"`
	EpisodeID       string                  `json:"episodeId"`
	DiscoverySHA256 string                  `json:"discoverySha256"`
	Manifest        ManifestDigestReference `json:"manifest"`
	Mode            string                  `json:"mode"`
	Groups          []BrowserStageGroupV2   `json:"groups"`
}
