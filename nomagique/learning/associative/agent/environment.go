package agent

import (
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
)

/*
Environment owns one agent's domain resources and execution. Feasible returns
only operations possible in the current state, including an explicit no-op when
appropriate. Context contains agent-specific state identities, not outcomes.
Execute accepts a decision once; an error may leave acceptance uncertain, so
the agent retains its ticket until Resolve or an explicit Abort.
Methods consume resident state; network and durable storage run outside Step.
*/
type Environment[Action comparable] interface {
	Feasible(label string) (actions []Action, context []uint64, err error)
	Execute(decision *Decision[Action]) error
	Objective() (reward.Mark, error)
}

/*
Observation carries one context's measurements and ordered prior context tokens.
The producer supplies History from its observed temporal horizon. Tokens must
have stable identities across restarts and preserve sequence boundaries. The
agent appends current grid conditions; it does not invent a history window.
*/
type Observation struct {
	At           time.Time
	Measurements []*data.Measurement[float64]
	History      []uint64
}

/*
Decision is the original issued action and its immutable causal evidence.
Context is retained for delayed evaluation and cross-agent consolidation;
it contains identities, not a snapshot of observations or mutable grid values.
Environment must treat Context as read-only.
*/
type Decision[Action comparable] struct {
	ID        uint64
	Label     string
	At        time.Time
	Action    Action
	Context   []uint64
	Authority float64
	Outcome   *float64
}
