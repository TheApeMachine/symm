package perspectives

/*
Perspective is one trade thesis encoded as a decision tree.

The new market layer works like this:

  - Signals continuously emit engine.Measurement values (each carries a
    Category string — the signal's row from DECISION.md).
  - Story holds the latest measurements and registers many Perspective values
    (pump, dump, organic trend, …). Each Perspective owns a *perspectives.Tree:
    a playbook of CategoryType branches ending in Action or Observation nodes.
  - To see if a thesis applies, walk the tree: at each CategoryType branch,
    check whether the current measurement set includes that category (via
    Measurement.HasCategory). If any required step is missing, that perspective
    is not traversable for this symbol and is ignored.
  - Observation nodes branch on ObservationType (holding, not holding, …) using
    runtime state; Category branches use ingested signal measurements only.
  - A traversable path that reaches an Action leaf is an active playbook
    (stop loss, take profit, etc.). Several perspectives can be active in parallel.

This type is the market-layer wrapper: PerspectiveType identifies which playbook,
Measurements is the ingested verdict set for one symbol, Tree is the static
structure from market/perspectives (e.g. NewPumpPerspective().Tree).
*/
type Perspective interface {
	Walk(measurements []Measurement) Perspective
	Decide(measurements []Measurement) *ActionType
	Regime() Regime
	Confidence() float64
}
