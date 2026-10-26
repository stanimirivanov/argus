# Synthetic operator inputs

These canonical artifacts derive from `requestReview(t, false)` and `sources(t, "v1")`
in the selectionio adapter tests. They use the existing orders impact and Perfeng
page fixtures with the fixed test clock 2026-10-22T12:00:00Z.

They are expired synthetic test data, not authenticated production approval.
Approval JSON combines the independent authority and provider expectations;
the review's SHA-256 is pinned by that approval. Tests perform real compilation.
