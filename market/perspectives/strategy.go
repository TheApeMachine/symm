package perspectives

/*
noiseFloorSNR is the boundary a category reading must clear to count as present
rather than noise. SNR is a z-score each signal computes live against its own
running noise floor, so 1.0 means "one sigma above this signal's own noise".
*/
const noiseFloorSNR = 1.0

/*
strategy is the shared decision-tree body every named playbook uses. Entry and
exit are separate trees so a decayed entry gate does not strand an open position.
*/
type strategy struct {
	name   PlaybookName
	regime Regime
	policy EntryPolicy
	deny   *Tree
	entry  *Tree
	exit   *Tree
}

/*
newStrategy builds a playbook with separate entry and exit trees.
*/
func newStrategy(
	name PlaybookName,
	regime Regime,
	policy EntryPolicy,
	entry []Branch,
	exit Branch,
) *strategy {
	return &strategy{
		name:   name,
		regime: regime,
		policy: policy,
		deny:   denyTreeFor(policy),
		entry:  &Tree{Branches: entry},
		exit:   &Tree{Branches: []Branch{exit}},
	}
}

func (strat *strategy) Name() PlaybookName {
	return strat.name
}

// DecideExit satisfies Perspective.
func (strat *strategy) DecideExit(
	measurements []Measurement,
	observations []ObservationType,
) *ActionType {
	return strat.decideExit(measurements, observations)
}

/*
Walk reports whether the entry playbook is traversable while flat.
*/
func (strat *strategy) Walk(measurements []Measurement) Perspective {
	if strat.entry.Walk(measurements, nil) == nil {
		return nil
	}

	return strat
}

/*
Decide returns an entry, exit, deny, or wait verdict for the measurement set.
*/
func (strat *strategy) Decide(
	measurements []Measurement,
	observations []ObservationType,
) *ActionType {
	if holding(observations) {
		return strat.decideExit(measurements, observations)
	}

	return strat.decideEntryWithTrace(measurements, observations, nil)
}

/*
DecideWithTrace returns an entry verdict and records the decision path in trace.
*/
func (strat *strategy) DecideWithTrace(
	measurements []Measurement,
	observations []ObservationType,
	trace *DecisionTrace,
) *ActionType {
	if holding(observations) {
		return strat.decideExit(measurements, observations)
	}

	return strat.decideEntryWithTrace(measurements, observations, trace)
}

func (strat *strategy) Regime() Regime {
	return strat.regime
}

func (strat *strategy) decideEntryWithTrace(
	measurements []Measurement,
	observations []ObservationType,
	trace *DecisionTrace,
) *ActionType {
	if action := strat.deny.WalkWithTrace(measurements, observations, trace); action != nil {
		if IsEntryBlocked(*action) {
			return action
		}
	}

	action := strat.entry.WalkWithTrace(measurements, observations, trace)

	if action == nil || *action == ActionNone {
		return nil
	}

	if IsEntryBlocked(*action) {
		return action
	}

	if *action != ActionEnter {
		return nil
	}

	return action
}

func (strat *strategy) decideExit(
	measurements []Measurement,
	observations []ObservationType,
) *ActionType {
	action := strat.exit.Walk(measurements, observations)

	if action == nil || !IsExitAction(*action) {
		return nil
	}

	return action
}

func holding(observations []ObservationType) bool {
	for _, observation := range observations {
		if observation == ObservationHolding {
			return true
		}
	}

	return false
}

/*
snrBranch is a leaf that fires action once category clears its own noise floor.
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
snrGate is a category that must be present above its noise floor to reach children.
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
entryLeaf authorizes entry when its trigger category clears the floor.
*/
func entryLeaf(trigger CategoryType) Branch {
	return Branch{
		Category:  trigger,
		Unit:      UnitSNR,
		Condition: ConditionIsGreaterThan,
		Value:     noiseFloorSNR,
		Action:    ActionEnter,
	}
}

/*
holdingExitBranch is the exit thesis reached only when ObservationHolding is active.
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
