package perspectives

/*
noiseFloorSNR is the boundary a category reading must clear to count as present
rather than noise. SNR is a z-score each signal computes live against its own
running noise floor, so 1.0 means "one sigma above this signal's own noise" — the
canonical point where a reading stops being indistinguishable from the signal's
baseline churn. It is not a tuned market value: the scaling is the signal's and
moves with conditions. What separates one strategy from another is which categories
it requires and how many must agree to reach a leaf, never a hand-picked threshold.
*/
const noiseFloorSNR = 1.0

/*
strategy is the shared decision-tree body every named playbook is built from. A
playbook is just a branch set plus the regime it reads the market as; the walking,
the entry/exit verdict, and the Perspective contract are identical across all of
them and live here so each strategy file stays a declaration of its tree, not a
re-implementation of the engine.
*/
type strategy struct {
	tree   *Tree
	regime Regime
}

/*
newStrategy builds a strategy perspective from a branch set and the regime it
reads the market as.
*/
func newStrategy(regime Regime, branches []Branch) *strategy {
	return &strategy{
		tree:   &Tree{Branches: branches},
		regime: regime,
	}
}

/*
Walk reports whether the playbook is traversable for the measurement set, returning
the perspective itself when a leaf is reachable and nil when it is not.
*/
func (strat *strategy) Walk(measurements []Measurement) Perspective {
	if strat.tree.Walk(measurements, nil) == nil {
		return nil
	}

	return strat
}

/*
Decide returns the action at the deepest reachable leaf for the current measurements
and observation state, or nil when no path is traversable. With no observations it is
the flat-entry view; passing ObservationHolding reaches the exit-thesis leaves, so the
same method that authorizes an entry also calls the exit — entries and exits are one
continuously re-evaluated thesis, not two strategies.
*/
func (strat *strategy) Decide(
	measurements []Measurement, observations []ObservationType,
) *ActionType {
	return strat.tree.Walk(measurements, observations)
}

func (strat *strategy) Regime() Regime {
	return strat.regime
}

func (strat *strategy) Confidence() float64 {
	return 0.0
}

/*
snrBranch is a leaf that fires action once category clears its own noise floor. It
is the atom every playbook is assembled from: a single category answering a single
question with a single verdict.
*/
func snrBranch(category CategoryType, action ActionType) Branch {
	return Branch{
		Category:  category,
		Unit:      UnitSNR,
		Condition: ConditionIsGreaterThan,
		Value:     noiseFloorSNR,
		Action:    action,
	}
}

/*
snrGate is a category that must be present above its noise floor to reach its
children but carries no verdict of its own. It expresses a hard precondition: the
path beyond it is dead unless this category confirms, and because each signal emits
exactly one category at a time, requiring the confirming category also excludes its
contradicting siblings (requiring HardSupport excludes ToxicBluff) with no need for
negation.
*/
func snrGate(category CategoryType, children ...Branch) Branch {
	return Branch{
		Category:  category,
		Unit:      UnitSNR,
		Condition: ConditionIsGreaterThan,
		Value:     noiseFloorSNR,
		Branches:  children,
	}
}

/*
entryBranch is a trigger category that authorizes entry on its own, while also
carrying the position-management subtree that takes over once the trade is held.
While flat the management subtree is unreachable (it is observation-gated) so the
branch falls back to ActionEnter; once held, the deeper management leaves win.
*/
func entryBranch(trigger CategoryType, management Branch) Branch {
	return Branch{
		Category:  trigger,
		Unit:      UnitSNR,
		Condition: ConditionIsGreaterThan,
		Value:     noiseFloorSNR,
		Action:    ActionEnter,
		Branches:  []Branch{management},
	}
}

/*
holdingExitBranch is the exit thesis shared by every entry playbook: once a position
is held, an urgent decay category trips the stop, while softer ones harvest profit.
It is reached only when ObservationHolding is active, so it never fires on a flat
book, and it embodies the principle that entry and exit are one continuously
re-evaluated thesis rather than two strategies — the same tree that opened the trade
decides when its reason is gone.
*/
func holdingExitBranch(urgentStop CategoryType, softExits ...CategoryType) Branch {
	children := []Branch{snrBranch(urgentStop, ActionStopLoss)}

	for _, category := range softExits {
		children = append(children, snrBranch(category, ActionTakeProfit))
	}

	return Branch{
		Observation: ObservationHolding,
		Branches:    children,
	}
}
