# Build a chronological predictive replay

## TL;DR

- Capture one [observation](predictive-observations.md) for each full-suite control group.
- List every expected observation file for each decision in an index beside the files.
- Run `predictive-replay -index index.json` and retain its JSON output with the immutable inputs.

The index format is `argus.dev/predictive-dataset-index/v1`:

~~~json
{
  "apiVersion": "argus.dev/predictive-dataset-index/v1",
  "holdoutStart": "2026-10-20T10:00:00Z",
  "holdoutEnd": "2026-10-21T10:00:00Z",
  "episodes": [
    { "id": "decision-1", "observations": ["decision-1-api.json", "decision-1-browser.json"] },
    { "id": "decision-2", "observations": ["decision-2-api.json"] }
  ]
}
~~~

An episode is one change and one manifest. A different manifest for the same change is a separate episode. Files must be relative to the index directory and remain inside it after symlink resolution. The command reads at most 1 MiB of index data, 16 MiB per observation, 1,000 episodes, and 10,000 observations. It emits no partial JSON on error.

The output `argus.dev/predictive-replay/v1` sorts episodes by feature cutoff and ID. It records the digest of each archived observation, the split, exclusion reason, and the history eligible for each holdout decision. Training requires every control to finish **before** `holdoutStart`; the holdout is `[holdoutStart, holdoutEnd)`. A training episode with a late control is excluded. Later decisions are excluded as outside the window. If a source pull request would appear in both eligible training and holdout, replay fails.

The output contains counts and references, not copied labels. Ranking and baseline evaluators must load the referenced observation envelopes and use only the `history` declared for a held-out episode. The index declares expected groups but cannot prove that its producer did not omit an undeclared group; retain the original manifest and plan for archive audits. Input authenticity, privacy, and retention remain source-CI responsibilities.
