package perspectives

import (
	"strconv"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
This is the evaluator for the Thought/Predicate language — the heart that replay,
live, and the optimizer all call. It is deliberately decoupled from where the data
comes from: a ReasonContext answers "what is X right now / N ago", and the evaluator
does only the logic (boolean composition, level/temporal/edge comparisons,
metric-to-metric). The temporal state (entry time, peak, lifecycle, the ring window
for lookbacks) lives in whoever builds the ReasonContext, so the logic stays pure.

The Thought walk has two modes over the SAME predicate logic. Evaluate is the
stateless single-tick walk: it returns the deepest reachable decision for the
current context, reading a tree as "all of this true right now". EvaluateStateful
threads a ReasonState so a node's Then children stay watched on the ticks that
FOLLOW the node firing — "see X, then later when Y, do Z" — which is what makes tree
depth express a sequence over time. holds/levelOp/temporalOp are identical for both.
*/

// ReasonContext supplies the live values a predicate reads. Implementations build
// it from the per-symbol measurement window, the regime, and the open position.
type ReasonContext interface {
	// Regime is the current price-action regime.
	Regime() types.Regime
	// Lifecycle reports whether the position is in the given state
	// (not_holding / holding / has_started / has_continued / has_ended).
	Lifecycle(state types.ObservationType) bool
	// PositionSide is the open position's entry side (buy = long, sell = short).
	PositionSide() trading.Side
	// Signal returns a category's strength in the given unit (snr/confidence),
	// ago measurements ago (0 = now); ok is false when the category is absent.
	Signal(category types.CategoryType, unit reasoning.UnitType, ago int) (value float64, ok bool)
	// Scalar returns a scalar subject (price/volume/spread/elapsed) in the given
	// unit, ago measurements ago; ok is false when it is unavailable.
	Scalar(subject reasoning.Subject, unit reasoning.UnitType, ago int) (value float64, ok bool)
}

/*
ReasonState is the per-symbol memory EvaluateStateful threads across ticks: the set
of thought nodes that have fired ("latched") within the current episode, keyed by
their path in the tree. A node that fired on an earlier tick keeps its Then children
reachable until the episode resets, which is what turns tree depth into a sequence
over time rather than a set of conditions that must all hold at once. The latch set
is cleared whenever the holding state flips (an entry filled, or a position closed),
so each flat stretch and each held stretch reasons from a clean frontier. Reuse one
ReasonState per symbol across ticks; a fresh one yields the single-tick semantics.
*/
type ReasonState struct {
	active      map[string]bool
	next        map[string]bool
	lastHolding bool
	primed      bool
}

// NewReasonState returns an empty, ready-to-thread state.
func NewReasonState() *ReasonState {
	return &ReasonState{
		active: make(map[string]bool),
		next:   make(map[string]bool),
	}
}

/*
Reset clears the cross-tick frontier while keeping the backing maps. Replay reuses
one ReasonState per symbol across candidate scores, so reset must not allocate.
*/
func (state *ReasonState) Reset() {
	if state == nil {
		return
	}

	clear(state.active)
	clear(state.next)

	state.lastHolding = false
	state.primed = false
}

/*
Evaluate walks the thoughts against the context and returns the decision at the
deepest reachable, satisfied node (deeper = more specific reasoning); ties at the
same depth resolve to the first sibling in tree order. It is the stateless
single-tick walk — EvaluateStateful over a throwaway state — so the tree is read as
"all of this true right now". The bool is false when nothing resolves to an action.
*/
func Evaluate(thoughts []Thought, ctx ReasonContext) (Act, bool) {
	return EvaluateStateful(thoughts, ctx, NewReasonState())
}

/*
EvaluateStateful is Evaluate with cross-tick memory. A node's Then children become
reachable once the node has fired and STAY reachable (latched) until the episode
resets, so an ordered chain advances over the ticks instead of needing everything
true on one tick. The action returned is still the deepest node whose own When holds
THIS tick, so with a fresh state (no prior latch) it is identical to Evaluate.
*/
/*
NodeTrace is one node's outcome during a single evaluation: whether it was
reachable this tick, whether its own When held (Fires), whether it was latched from
an earlier tick, and whether its action was the one chosen (Fired).
*/
type NodeTrace struct {
	Key       string `json:"key"`
	Depth     int    `json:"depth"`
	Reachable bool   `json:"reachable"`
	Fires     bool   `json:"fires"`
	Latched   bool   `json:"latched"`
	Fired     bool   `json:"fired"`
	Leaves    []bool `json:"-"` // per-leaf holds for multi-condition nodes (decision tree breakdown)
}

// ReasonTrace collects the per-node outcomes of one evaluation for the live
// decision-tree view. The zero value is ready to use.
type ReasonTrace struct {
	Nodes []NodeTrace
}

func EvaluateStateful(thoughts []Thought, ctx ReasonContext, state *ReasonState) (Act, bool) {
	return evaluateStateful(thoughts, ctx, state, nil)
}

/*
EvaluateStatefulTraced is EvaluateStateful that also records, into trace, every
node's outcome so the live decision tree can show where evaluations travel and
where they die. The returned action is identical to EvaluateStateful — tracing has
no effect on the decision.
*/
func EvaluateStatefulTraced(thoughts []Thought, ctx ReasonContext, state *ReasonState, trace *ReasonTrace) (Act, bool) {
	return evaluateStateful(thoughts, ctx, state, trace)
}

func evaluateStateful(thoughts []Thought, ctx ReasonContext, state *ReasonState, trace *ReasonTrace) (Act, bool) {
	if state == nil {
		state = NewReasonState()
	}

	// Episode boundary: when holding flips, the chain that drove the decision has
	// done its job — start the next stretch with a clean frontier so it can re-arm.
	holding := ctx.Lifecycle(types.ObservationHolding)

	if state.primed && holding != state.lastHolding {
		clear(state.active)
	}

	state.lastHolding = holding
	state.primed = true

	if state.next == nil {
		state.next = make(map[string]bool, len(state.active))
	}

	clear(state.next)

	next := state.next
	best := Act{}
	bestDepth := -1
	bestKey := ""
	found := false

	var visit func(nodes []Thought, depth int, prefix string, parentOpen bool)
	visit = func(nodes []Thought, depth int, prefix string, parentOpen bool) {
		for index := range nodes {
			node := nodes[index]
			key := prefix + strconv.Itoa(index)
			latched := state.active[key]

			// A node is reachable when its parent is open this tick, or when it
			// already latched on an earlier tick of this episode.
			reachable := parentOpen || latched

			firesNow := false

			if reachable {
				firesNow = holds(node.When, ctx)

				if firesNow || latched {
					next[key] = true
				}

				if firesNow && node.Do.Type != reasoning.ActionNone && depth > bestDepth {
					best, bestDepth, bestKey, found = node.Do, depth, key, true
				}
			}

			if trace != nil {
				nodeTrace := reasoning.NodeTrace{
					Key:       key,
					Depth:     depth,
					Reachable: reachable,
					Fires:     firesNow,
					Latched:   latched,
				}

				// For a reached compound node, record each leaf condition's own
				// truth so the tree can show which sub-condition is the one failing.
				if reachable {
					if leaves := reasoning.FlattenLeaves(node.When); len(leaves) > 0 {
						holdsPerLeaf := make([]bool, len(leaves))

						for leafIndex := range leaves {
							holdsPerLeaf[leafIndex] = holds(leaves[leafIndex].Predicate, ctx)
						}

						nodeTrace.Leaves = holdsPerLeaf
					}
				}

				trace.Nodes = append(trace.Nodes, nodeTrace)
			}

			if reachable {
				visit(node.Then, depth+1, key+".", firesNow || latched)
			} else if trace != nil {
				// In trace mode, descend into the unreachable subtree too so the
				// tree view shows the whole playbook with these branches marked
				// never-reached. Untraced callers keep the original early exit.
				visit(node.Then, depth+1, key+".", false)
			}
		}
	}

	visit(thoughts, 0, "", true)

	state.active, state.next = state.next, state.active

	if trace != nil && found {
		for index := range trace.Nodes {
			if trace.Nodes[index].Key == bestKey {
				trace.Nodes[index].Fired = true

				break
			}
		}
	}

	return best, found
}

// holds reports whether a predicate is satisfied in the context.
func holds(pred Predicate, ctx ReasonContext) bool {
	switch {
	case len(pred.All) > 0:
		for _, operand := range pred.All {
			if !holds(operand, ctx) {
				return false
			}
		}

		return true
	case len(pred.Any) > 0:
		for _, operand := range pred.Any {
			if holds(operand, ctx) {
				return true
			}
		}

		return false
	case pred.Not != nil:
		return !holds(*pred.Not, ctx)
	}

	switch pred.Subject {
	case SubjectRegime:
		return ctx.Regime() == pred.Regime
	case SubjectPosition:
		if pred.Side != "" {
			return ctx.PositionSide() == pred.Side
		}

		return ctx.Lifecycle(pred.Lifecycle)
	default:
		return holdsNumeric(pred, ctx)
	}
}

func holdsNumeric(pred Predicate, ctx ReasonContext) bool {
	switch pred.Op {
	case ComparisonRoseBy, ComparisonFellBy, ComparisonCrossedUp, ComparisonCrossedDown:
		now, okNow := subjectValue(ctx, pred.Subject, pred.Category, pred.Unit, 0)
		then, okThen := subjectValue(ctx, pred.Subject, pred.Category, pred.Unit, pred.Ago)

		if !okNow || !okThen {
			return false
		}

		return temporalOp(pred.Op, now, then, pred.Value, pred.Unit)
	default:
		left, okLeft := subjectValue(ctx, pred.Subject, pred.Category, pred.Unit, pred.Ago)

		if !okLeft {
			return false
		}

		right, okRight := rightHandSide(pred, ctx)

		if !okRight {
			return false
		}

		return levelOp(pred.Op, left, right)
	}
}

// rightHandSide resolves the comparison target: another live subject (Versus) for
// metric-to-metric gating, otherwise the static Value.
func rightHandSide(pred Predicate, ctx ReasonContext) (float64, bool) {
	if pred.Versus == nil {
		return pred.Value, true
	}

	operand := pred.Versus

	return subjectValue(ctx, operand.Subject, operand.Category, operand.Unit, operand.Ago)
}

func subjectValue(
	ctx ReasonContext, subject reasoning.Subject, category types.CategoryType, unit reasoning.UnitType, ago int,
) (float64, bool) {
	switch subject {
	case reasoning.SubjectSignal:
		return ctx.Signal(category, unit, ago)
	case reasoning.SubjectPrice, reasoning.SubjectVolume, reasoning.SubjectSpread, reasoning.SubjectElapsed:
		return ctx.Scalar(subject, unit, ago)
	default:
		return 0, false
	}
}

func levelOp(op Comparison, left, right float64) bool {
	switch op {
	case ComparisonAtLeast:
		return left >= right
	case ComparisonAtMost:
		return left <= right
	case ComparisonAbove:
		return left > right
	case ComparisonBelow:
		return left < right
	case ComparisonEquals:
		return left == right
	default:
		return false
	}
}

// temporalOp compares now against then over the lookback. When Unit is
// percentage the change is relative; otherwise it is absolute.
func temporalOp(op reasoning.Comparison, now, then, target float64, unit reasoning.UnitType) bool {
	switch op {
	case reasoning.ComparisonRoseBy:
		return change(now, then, unit) >= target
	case reasoning.ComparisonFellBy:
		return change(then, now, unit) >= target
	case reasoning.ComparisonCrossedUp:
		return then < target && now >= target
	case reasoning.ComparisonCrossedDown:
		return then > target && now <= target
	default:
		return false
	}
}

func change(to, from float64, unit reasoning.UnitType) float64 {
	if unit == reasoning.UnitPercentage {
		if from == 0 {
			return 0
		}

		return (to - from) / from * 100
	}

	return to - from
}
