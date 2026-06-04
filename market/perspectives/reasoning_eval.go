package perspectives

/*
This is the evaluator for the proposed Thought/Predicate language — the heart that
replay, live, and the optimizer will all call. It is deliberately decoupled from
where the data comes from: a ReasonContext answers "what is X right now / N ago",
and the evaluator does only the logic (boolean composition, level/temporal/edge
comparisons, metric-to-metric). The temporal state (entry time, peak, lifecycle,
the ring window for lookbacks) lives in whoever builds the ReasonContext, so this
function stays pure and testable.

NOTE — the Thought walk below is the single-tick v1: it returns the deepest
reachable decision given the current context. The fully temporal "then =
monitored over the ticks that follow" semantics (a position carries the active
reasoning path) is the next design step; the predicate logic here is the part
that does not change.
*/

// ReasonContext supplies the live values a predicate reads. Implementations build
// it from the per-symbol measurement window, the regime, and the open position.
type ReasonContext interface {
	// Regime is the current price-action regime.
	Regime() Regime
	// Lifecycle reports whether the position is in the given state
	// (not_holding / holding / has_started / has_continued / has_ended).
	Lifecycle(state ObservationType) bool
	// Signal returns a category's strength in the given unit (snr/confidence),
	// ago measurements ago (0 = now); ok is false when the category is absent.
	Signal(category CategoryType, unit UnitType, ago int) (value float64, ok bool)
	// Scalar returns a scalar subject (price/volume/spread/elapsed) in the given
	// unit, ago measurements ago; ok is false when it is unavailable.
	Scalar(subject Subject, unit UnitType, ago int) (value float64, ok bool)
}

/*
Evaluate walks the thoughts against the context and returns the decision at the
deepest reachable, satisfied node (deeper = more specific reasoning). The bool is
false when no thought resolves to an action.
*/
func Evaluate(thoughts []Thought, ctx ReasonContext) (Act, bool) {
	best := Act{}
	bestDepth := -1
	found := false

	var visit func(nodes []Thought, depth int)
	visit = func(nodes []Thought, depth int) {
		for _, node := range nodes {
			if !holds(node.When, ctx) {
				continue
			}

			if node.Do.Type != ActionNone && depth > bestDepth {
				best, bestDepth, found = node.Do, depth, true
			}

			visit(node.Then, depth+1)
		}
	}

	visit(thoughts, 0)

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
	ctx ReasonContext, subject Subject, category CategoryType, unit UnitType, ago int,
) (float64, bool) {
	switch subject {
	case SubjectSignal:
		return ctx.Signal(category, unit, ago)
	case SubjectPrice, SubjectVolume, SubjectSpread, SubjectElapsed:
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
func temporalOp(op Comparison, now, then, target float64, unit UnitType) bool {
	switch op {
	case ComparisonRoseBy:
		return change(now, then, unit) >= target
	case ComparisonFellBy:
		return change(then, now, unit) >= target
	case ComparisonCrossedUp:
		return then < target && now >= target
	case ComparisonCrossedDown:
		return then > target && now <= target
	default:
		return false
	}
}

func change(to, from float64, unit UnitType) float64 {
	if unit == UnitPercentage {
		if from == 0 {
			return 0
		}

		return (to - from) / from * 100
	}

	return to - from
}
