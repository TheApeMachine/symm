package mcts

import "fmt"

/*
Action is one strategic intervention. Actions are interventions, never
graph-node selections.
*/
type Action int

const (
	// Wait holds the current position.
	Wait Action = iota
	// Enter opens a position from flat.
	Enter
	// Exit closes an open position.
	Exit
	// Scale increases an open position.
	Scale
)

func (action Action) String() string {
	switch action {
	case Wait:
		return "wait"
	case Enter:
		return "enter"
	case Exit:
		return "exit"
	case Scale:
		return "scale"
	default:
		return fmt.Sprintf("unknown:%d", int(action))
	}
}

/*
ActionEstimate is the causal economic estimate for one action. An action
whose outcome cannot be validly estimated is Undefined: Defined=false. It is
never assigned zero, correlation, an old estimate, or an arbitrary penalty.
*/
type ActionEstimate struct {
	Action               Action
	ExpectedOutcome      float64
	Uncertainty          float64
	IdentificationStatus IdentificationStatus
	Support              float64
	Defined              bool
}

/*
DecisionUnavailable is the explicit result when no feasible action has an
estimable objective. It is not equivalent to Wait; an outer safety policy may
choose to do nothing, but that is outside the causal evidence model.
*/
const DecisionUnavailable = "decision_unavailable"
