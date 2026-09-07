package hindsight

import (
	"github.com/theapemachine/symm/kraken"
	"time"
)

/*
LifecycleEvent is one real trading-lifecycle transition: an entry submitted, a
fill, a position opening, a stop-loss protection change, an exit, a close.
DecisionID links the position's entry witness. ActionCorrelationID separately
identifies the instruction whose client order produced an execution. These
identities coincide for entry and differ for reductions and exits.

This is a recording of what the live/paper production system actually did, never
a separate backtest trade model.
*/
type LifecycleEvent struct {
	// ActionCorrelationID is the client order ID of the particular instruction.
	// DecisionID retains the position entry witness; it must not price later actions.
	ActionCorrelationID string    `json:"actionCorrelationId,omitempty"`
	DecisionID          string    `json:"decisionId"`
	Symbol              string    `json:"symbol"`
	Kind                string    `json:"kind"`
	Action              string    `json:"action,omitempty"`
	At                  time.Time `json:"at"`

	// Execution carries order identity for submission/failure and authoritative
	// venue economics for terminal and fill events. Position open/close events
	// have no execution fact.
	Execution *kraken.ExecutionData `json:"execution,omitempty"`

	// CaptureSeq is the capture sequence of the envelope whose decision caused
	// this transition, resolved on read by joining DecisionID to the decision
	// witness. It is what lets a reader seek the tape to the exact frame a
	// position was opened or closed on, rather than searching by wall time.
	// Zero when no decision witness recorded that identity.
	CaptureSeq uint64 `json:"captureSeq,omitempty"`
}
