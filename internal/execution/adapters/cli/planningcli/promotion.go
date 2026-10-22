package planningcli

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
)

type promotionFlags struct{ policy, decision, digest, asOf *string }

func addPromotionFlags(flags *flag.FlagSet) promotionFlags {
	return promotionFlags{policy: flags.String("promotion-policy", "", "authenticated owner policy file"), decision: flags.String("promotion-decision", "", "fresh promotion decision file"), digest: flags.String("promotion-sha256", "", "trusted full decision SHA-256"), asOf: flags.String("promotion-as-of", "", "trusted current timestamp, identical to gate evaluation")}
}

func (f promotionFlags) provided() bool {
	return *f.policy != "" || *f.decision != "" || *f.digest != "" || *f.asOf != ""
}

func (f promotionFlags) mode(requested string, source scheduledplan.Schedule) (scheduledplan.Mode, error) {
	if requested != "staged" {
		return scheduledplan.Mode(requested), nil
	}
	// No configured authority means rollback to independent full control.
	if !f.provided() {
		return scheduledplan.Shadow, nil
	}
	policyBytes, err := readDocument(*f.policy, nil, "promotion policy")
	if err != nil {
		return "", err
	}
	policy, err := contracts.DecodePredictivePromotionPolicyV1(policyBytes)
	if err != nil {
		return "", err
	}
	decisionBytes, err := readDocument(*f.decision, nil, "promotion decision")
	if err != nil {
		return "", err
	}
	decision, err := contracts.DecodePredictivePromotionDecisionV1(decisionBytes)
	if err != nil {
		return "", err
	}
	if promotionSHA(decisionBytes) != *f.digest || decision.PolicySHA256 != promotionSHA(policyBytes) {
		return scheduledplan.Shadow, nil
	}
	now, err := time.Parse(time.RFC3339Nano, *f.asOf)
	if err != nil {
		return "", err
	}
	if !contracts.PromotionDecisionCurrent(policy, decision, now) || decision.ScheduleSHA256 != source.Reference.SHA256 {
		return scheduledplan.Shadow, nil
	}
	for _, group := range source.Groups {
		repo := group.TestRepository.Identity
		if string(repo.Provider) != policy.Repository.Provider || repo.Host != policy.Repository.Host || repo.ProviderRepositoryID != policy.Repository.ProviderRepositoryID || string(group.Family) != policy.Family {
			return scheduledplan.Shadow, nil
		}
	}

	return scheduledplan.Staged, nil
}

func promotionSHA(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
