/*
Package advisor hosts SYMM's descriptive context layer: operational components
(Advisors) that consume already-produced observations, maintain bounded resident
state, and emit Perspectives — current descriptive context that is never a gate,
a score, or a trade instruction.

The contract every Advisor obeys:

 1. subscribe to one or more typed streams;
 2. consume each event exactly once;
 3. mutate bounded resident state;
 4. optionally emit a Perspective;
 5. retain no unbounded event backlog;
 6. never reconstruct a world snapshot to process one event.

Advisors compose existing outputs (Measurements, Categories, Manifold,
Cognition, Graph, Passage) rather than re-deriving raw signals, and they answer
"What context is relevant now?" — never "what should be done?".
*/
package advisor

import "github.com/theapemachine/symm/types"

/*
Advisor is the shared operational contract for context producers. One concrete
Advisor consumes one typed input stream, mutates its bounded resident state, and
returns the current Perspective for that step — or nil when the input produced
no new context. The contract is deliberately minimal: the semantics that matter
(bounded state, no snapshots, no backlog, honest definedness) are enforced by
each advisor's tests, mirroring the code they test.
*/
type Advisor[T any] interface {
	Step(input T) *types.Perspective
}
