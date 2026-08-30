package hindsight

import "time"

/*
LifecycleEvent is one real trading-lifecycle transition: an entry submitted, a
fill, a position opening, a stop-loss protection change, an exit, a close. It is
decision-correlated (keyed by the decision ID that caused it) rather than
envelope-correlated, because a lifecycle transition happens inside the broker
after the planner committed a decision — the decision witness carries the exact
EnvelopeRef, and the lifecycle event names that decision as its cause.

This is a recording of what the live/paper production system actually did, never
a separate backtest trade model.
*/
type LifecycleEvent struct {
	// DecisionID is the correlation key back to the decision witness that
	// caused this transition.
	DecisionID string    `json:"decisionId"`
	Symbol     string    `json:"symbol"`
	Kind       string    `json:"kind"`
	Action     string    `json:"action,omitempty"`
	At         time.Time `json:"at"`
}
