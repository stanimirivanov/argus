package cli

import (
	"encoding/json"
	"flag"
	"io"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive"
)

// RunPromotion rederives original shadow evidence and uses the composition root's
// chronological archive reader. Invalid evidence writes nothing; policy denials
// emit a reason-coded shadow decision. No dispatch or persistence occurs.
func RunPromotion(args []string, stdout io.Writer, replayLoader func(string, string, int64) (predictive.PromotionReplay, error)) error {
	flags := flag.NewFlagSet("predictive-promotion", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	index := flags.String("index", "", "chronological dataset index")
	constraints := flags.String("constraints", "", "reviewed replay constraints")
	budget := flags.Int64("budget-ms", 0, "portfolio budget")
	shadow := flags.String("shadow-index", "", "original prospective shadow archive")
	policyPath := flags.String("policy", "", "owner-reviewed promotion policy")
	asOf := flags.String("as-of", "", "trusted current UTC evaluation timestamp")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *index == "" || *constraints == "" || *shadow == "" || *policyPath == "" || *budget < 1 || flags.NArg() != 0 || replayLoader == nil {
		return commandline.UsageText("usage: predictive-promotion -index <path> -constraints <path> -budget-ms <ms> -shadow-index <path> -policy <path> -as-of <timestamp>")
	}
	now, err := time.Parse(time.RFC3339Nano, *asOf)
	if err != nil {
		return err
	}
	data, err := readBounded(*policyPath)
	if err != nil {
		return err
	}
	policyDoc, err := contracts.DecodePredictivePromotionPolicyV1(data)
	if err != nil {
		return err
	}
	policy, err := importPromotionPolicy(policyDoc)
	if err != nil {
		return err
	}
	replay, err := replayLoader(*index, *constraints, *budget)
	if err != nil {
		return err
	}
	snapshot, shadowSHA, err := loadSafetySnapshot(*shadow)
	if err != nil {
		return err
	}
	decision, err := predictive.EvaluatePromotion(policy, replay, snapshot, now)
	if err != nil {
		return err
	}
	output := contracts.PredictivePromotionDecisionV1{APIVersion: "argus.dev/predictive-promotion-decision/v1", PolicySHA256: safetySHA(data), ReplaySHA256: replay.EvidenceSHA256, ShadowIndexSHA256: shadowSHA, ScheduleSHA256: policy.ScheduleSHA256, ModelSHA256: policy.ModelSHA256, CalibrationSHA256: policy.CalibrationSHA256, Repository: policyDoc.Repository, Family: policy.Family, EvaluatedAt: now.UTC().Format(time.RFC3339Nano), ExpiresAt: decision.ExpiresAt.UTC().Format(time.RFC3339Nano), Mode: decision.Mode, RemainingRequired: true, Reasons: decision.Reasons,
		Replay: exportPromotionMeasurement(decision.Replay), Shadow: exportPromotionMeasurement(decision.Shadow)}
	encoded, err := json.Marshal(output)
	if err != nil {
		return err
	}
	if _, err := contracts.DecodePredictivePromotionDecisionV1(encoded); err != nil {
		return err
	}
	_, err = stdout.Write(append(encoded, '\n'))

	return err
}

func importPromotionPolicy(p contracts.PredictivePromotionPolicyV1) (predictive.PromotionPolicy, error) {
	reviewed, err := time.Parse(time.RFC3339Nano, p.ReviewedAt)
	if err != nil {
		return predictive.PromotionPolicy{}, err
	}
	expires, err := time.Parse(time.RFC3339Nano, p.ExpiresAt)
	if err != nil {
		return predictive.PromotionPolicy{}, err
	}
	identity := catalog.RepositoryIdentity{Provider: catalog.Provider(p.Repository.Provider), Host: p.Repository.Host, ProviderRepositoryID: p.Repository.ProviderRepositoryID}
	encoded, err := json.Marshal(struct{ Provider, Host, ProviderRepositoryID string }{p.Repository.Provider, p.Repository.Host, p.Repository.ProviderRepositoryID})
	if err != nil {
		return predictive.PromotionPolicy{}, err
	}
	policy := predictive.PromotionPolicy{Repository: identity, RepositoryKey: safetySHA(encoded), Family: p.Family, ReviewID: p.ReviewID, ReviewedBy: p.ReviewedBy, ReviewedAt: reviewed, ExpiresAt: expires, ScheduleSHA256: p.ScheduleSHA256, ModelSHA256: p.ModelSHA256, CalibrationSHA256: p.CalibrationSHA256, MaxEvidenceAge: time.Duration(p.MaxEvidenceAgeSeconds) * time.Second, ValidFor: time.Duration(p.ValidForSeconds) * time.Second, MinReplayEpisodes: p.MinReplayEpisodes, MinShadowRuns: p.MinShadowRuns, MinFailures: p.MinFailures, MaxMissRate: p.MaxMissRate, MaxECE: p.MaxECE, MaxECEDrift: p.MaxECEDrift, MaxStageDurationMS: p.MaxStageDurationMS, MaxStageResourceUnitMS: p.MaxStageResourceUnitMS}

	return policy, predictive.ValidatePromotionPolicy(policy)
}

func exportPromotionMeasurement(m predictive.PromotionMeasurement) contracts.PredictivePromotionMeasurement {
	return contracts.PredictivePromotionMeasurement{Samples: m.Samples, Failures: m.Failures, Misses: m.Misses, MissRate: m.MissRate}
}
