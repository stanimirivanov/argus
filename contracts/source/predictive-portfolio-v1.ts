import { Schema } from "effect";
import { PredictiveAbstentionV1 } from "./predictive-abstention-v1.js";
import { PredictiveStageCostV1 } from "./predictive-cost-v1.js";
import { LocalKey } from "./repository-descriptor-v1.js";

const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const IDs = Schema.Array(Digest).check(Schema.isMaxLength(10000));
const Total = Schema.Int.check(
  Schema.isGreaterThanOrEqualTo(0),
  Schema.isLessThanOrEqualTo(8640000000000000),
);
/** Offline estimated serial plans, with complete review and safety evidence. */
export const PredictivePortfolioV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-portfolio/v1"]),
  budgetMs: Schema.Int.check(Schema.isGreaterThan(0), Schema.isLessThanOrEqualTo(86400000)),
  costs: PredictiveStageCostV1,
  abstention: PredictiveAbstentionV1,
  episodes: Schema.Array(
    Schema.Struct({
      id: LocalKey,
      mode: Schema.Literals(["portfolio", "full-control", "required-over-budget"]),
      fallbackReason: Schema.Literals(["", "abstention", "optimizer-limit"]),
      requiredTestIds: IDs,
      omittedTestIds: IDs,
      coveredCapabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(1000000)),
      selected: Schema.Array(
        Schema.Struct({
          id: Digest,
          reason: Schema.Literals(["required", "dependency", "diversity", "ranked", "fallback"]),
          startMs: Total,
          endMs: Total,
        }),
      ).check(Schema.isMaxLength(10000)),
      estimatedDurationMs: Total,
      overBudget: Schema.Boolean,
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(1000)),
}).annotate({ identifier: "PredictivePortfolioV1", title: "Argus predictive portfolio v1" });
export type PredictivePortfolioV1 = typeof PredictivePortfolioV1.Type;
