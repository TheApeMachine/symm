package perspectives

type PumpPerspective struct {
	Tree *Tree
}

/*
NewPumpPerspective creates a new PumpPerspective from a slice of measurements.
The Pump Perspective break down to the following decisions:

- The flash pump and dump
 1. We see some signal of coiled compression (slow breakout or pre-spike precursor)
 2. Followed by a signal of vertical ignition (spike up)
 3. We enter the position with a stop loss that we ratchet up with the spike
 4. We let the dump drop right into our stop loss

- The slow pump
 1. We see some signal of coiled compression (slow breakout or pre-spike precursor)
 2. We enter the position with a stop loss that we ratchet up with the spike
 3. We let the dump drop right into our stop loss

- Spoof onto them as they spoof onto us
 1. We see a crystal clear signal of spoofing happening (fake bid/ask volume)
 2. We enter the position early
 3. We dedicate a high-priority process to monitor the asset pair
 4. The moment we see the signal spike we predict the best moment to switch our long to a short positoin
*/
func NewPumpPerspective() *PumpPerspective {
	return &PumpPerspective{
		Tree: &Tree{Branches: pumpBranches},
	}
}

func (pump *PumpPerspective) Walk(measurements []Measurement) Perspective {
	if pump.Tree.Walk(measurements, nil) == nil {
		return nil
	}

	return pump
}

// Decide returns the Action at the deepest reachable leaf of the pump tree for the
// current measurement set and observation state, or nil when no playbook path is
// traversable. Observations reach the position-management leaves; nil is the
// flat-entry view.
func (pump *PumpPerspective) Decide(
	measurements []Measurement, observations []ObservationType,
) *ActionType {
	return pump.Tree.Walk(measurements, observations)
}

func (pump *PumpPerspective) Regime() Regime {
	return RegimeTrending
}

func (pump *PumpPerspective) Confidence() float64 {
	return 0.0
}

/*
pumpBranches is the pump playbook as a decision tree. Each entry branch carries
its own ActionEnter as the fallback, with deeper observation-gated branches that
manage an already-open position. Every threshold is an SNR noise floor (1 = one
sigma above the signal's own noise), so the tree is self-scaling, not tuned to
fixed prices.

  - Coiled compression clearing the floor is the slow-pump entry. While the
    position is held and the move continues, the stop ratchets up; when the move
    ends, profit is taken. A vertical-ignition confirmation under the same branch
    is the flash-pump (still an entry).
  - A spoof trap clearing the floor is an early entry; once held, a vertical
    ignition is the cue to flip the long into a short.
*/
var pumpBranches = []Branch{
	{
		Category:  CategoryCoiledCompression,
		Unit:      UnitSNR,
		Condition: ConditionIsGreaterThan,
		Value:     1.0,
		Action:    ActionEnter,
		Branches: []Branch{
			{
				Observation: ObservationHolding,
				Branches: []Branch{
					{Observation: ObservationHasContinued, Action: ActionStopLoss},
					{Observation: ObservationHasEnded, Action: ActionTakeProfit},
				},
			},
		},
	},
	{
		Category:  CategorySpoofTrap,
		Unit:      UnitSNR,
		Condition: ConditionIsGreaterThan,
		Value:     1.0,
		Action:    ActionEnter,
		Branches: []Branch{
			{
				Observation: ObservationHolding,
				Branches: []Branch{
					{
						Category:  CategoryVerticalIgnition,
						Unit:      UnitSNR,
						Condition: ConditionIsGreaterThan,
						Value:     1.0,
						Action:    ActionShort,
					},
				},
			},
		},
	},
}
