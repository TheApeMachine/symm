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

	// Execution carries the authoritative venue fill facts when the transition
	// is an entry_fill or exit_fill. It is nil for transitions that carry no
	// fill (order submission, position open, position close).
	Execution *ExecutionFact `json:"execution,omitempty"`
}

/*
ExecutionFact is the authoritative fill record for one execution: the venue's
reported order, quantity, price, cumulative economics, fee, and the resulting
position transition. It is correlated to the decision that produced the order
through the enclosing LifecycleEvent's DecisionID.
*/
type ExecutionFact struct {
	OrderID       string    `json:"orderId"`
	ClientOrderID string    `json:"clientOrderId"`
	ExecID        string    `json:"execId"`
	Side          string    `json:"side"`
	OrderStatus   string    `json:"orderStatus"`
	LastQty       string    `json:"lastQty,omitempty"`
	LastPrice     string    `json:"lastPrice,omitempty"`
	CumQty        string    `json:"cumQty,omitempty"`
	CumCost       string    `json:"cumCost,omitempty"`
	AvgPrice      string    `json:"avgPrice,omitempty"`
	FeeUsdEquiv   string    `json:"feeUsdEquiv,omitempty"`
	FillAt        time.Time `json:"fillAt"`
}
