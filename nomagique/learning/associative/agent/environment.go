package agent

import (
	"time"

	"github.com/theapemachine/symm/nomagique/cognition"

	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
)

/*
Environment owns one agent's domain resources and execution. Feasible returns
only operations possible in the current state, including an explicit no-op when
appropriate. Context contains agent-specific state identities, not outcomes.
Execute accepts a decision once; an error may leave acceptance uncertain, so
the agent retains its ticket until Resolve or an explicit Abort.
Objective returns nil while the objective cannot be measured; this supplies no
feedback and does not replace the last measured value. Errors remain failures.
Methods consume resident state; network and durable storage run outside Step.
The workload serializes calls on one environment.
*/
type Environment[Action comparable] interface {
	Feasible(label string) (actions []Action, context []uint64, err error)
	Execute(decision *Decision[Action]) error
	Objective() (*reward.Mark, error)
}

/*
Decision is the original issued action and its immutable causal evidence.
Context is retained for delayed evaluation and cross-agent consolidation;
it contains identities, not a snapshot of observations or mutable grid values.
Environment must treat Context as read-only.
*/
type Decision[Action comparable] struct {
	ID           uint64
	Label        string
	At           time.Time
	Action       Action
	Context      []uint64
	Authority    float64
	Outcome      *float64
	Evaluation   cognition.Evaluation
	Alternatives []Action
}
