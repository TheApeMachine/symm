package mcts

import "math"

/*
SearchNode is one node of the economic search tree. It accumulates only
economic rollout outcomes; no unrelated signal scores are injected into node
reward after causal simulation.
*/
type SearchNode struct {
	State          State
	Action         Action
	Parent         *SearchNode
	Children       []*SearchNode
	UntakenActions []Action
	Visits         int
	// Mean is the running mean reward maintained with Welford's method.
	Mean float64
	// SumSquaredDeviations is Welford's M2 accumulator for the sample
	// variance (sum of squared deviations from the running mean).
	SumSquaredDeviations float64

	// CounterfactualReward is the precision-weighted sum of virtual outcomes
	// this branch accrued from rollouts that took a sibling action. It is
	// kept apart from the observed reward so real and virtual experience are
	// never confused in a trace.
	CounterfactualReward float64
	// CounterfactualMass is the total precision weight behind
	// CounterfactualReward. It is the virtual visit count: fractional,
	// because a counterfactual whose noise term is large contributes less
	// than a full visit.
	CounterfactualMass float64
	// CounterfactualPrecision is the precision of the most recent
	// counterfactual update, retained for provenance.
	CounterfactualPrecision float64

	// CausalExpectation is the last interventional estimate
	// E[reward | do(action)] used to bias selection toward this branch.
	CausalExpectation float64
	// CausalExpectationDefined reports whether CausalExpectation came from an
	// identified query. A zero expectation from a failed query is not the same
	// as a genuine zero, and selection must not treat it as one.
	CausalExpectationDefined bool
	// Pruned reports that the branch was causally rejected: its identified
	// interventional expectation fell below the policy's rejection floor, so
	// selection withdraws it instead of letting UCB's exploration term keep
	// resampling a branch the causal model already condemned.
	Pruned bool

	Depth int
}

/*
MeanReward is the average economic reward observed at this node.
*/
func (node *SearchNode) MeanReward() float64 {
	if node == nil || node.Visits == 0 {
		return 0
	}

	return node.Mean
}

/*
RewardStandardDeviation is the sample standard deviation of the economic
reward, used for outcome uncertainty. It is the real reward dispersion, never
a pseudo-precision transform. The sample variance uses the Welford
accumulator with a Visits-1 (Bessel) denominator; it is undefined (reported
as zero) below two visits and clamped to zero when numerically negative.
*/
func (node *SearchNode) RewardStandardDeviation() float64 {
	if node == nil || node.Visits < 2 {
		return 0
	}

	variance := node.SumSquaredDeviations / float64(node.Visits-1)

	if variance < 0 {
		variance = 0
	}

	return math.Sqrt(variance)
}

/*
StandardError is the standard error of the mean reward.
*/
func (node *SearchNode) StandardError() float64 {
	if node == nil || node.Visits == 0 {
		return 0
	}

	return node.RewardStandardDeviation() / math.Sqrt(float64(node.Visits))
}

/*
CounterfactualMean is the precision-weighted mean of the virtual outcomes this
branch accrued without ever being rolled out. It reports zero when no
counterfactual mass has accumulated.
*/
func (node *SearchNode) CounterfactualMean() float64 {
	if node == nil || node.CounterfactualMass <= 0 {
		return 0
	}

	return node.CounterfactualReward / node.CounterfactualMass
}

/*
EffectiveVisits combines real rollout visits with precision-weighted virtual
experience. It is the confidence budget used by exploration: a branch that has
been counterfactually evaluated many times is better understood than an
untouched one, even with no rollout of its own.

It is deliberately not used as the denominator of MeanReward. Dividing observed
reward by virtual visits would shrink the mean toward zero for a branch that was
never actually rolled out, which is a fabricated pessimism the search contract
forbids.
*/
func (node *SearchNode) EffectiveVisits() float64 {
	if node == nil {
		return 0
	}

	return float64(node.Visits) + node.CounterfactualMass
}

/*
BlendedValue is the branch value used for selection: observed reward when the
branch has been rolled out, blended with counterfactual evidence in proportion
to the mass behind each.

A branch with no rollouts falls back to its counterfactual mean, which is what
makes Pearl's third rung pay for itself: an unvisited sibling arrives at
selection already carrying an estimate rather than nothing.
*/
func (node *SearchNode) BlendedValue() float64 {
	if node == nil {
		return 0
	}

	observedWeight := float64(node.Visits)
	virtualWeight := node.CounterfactualMass
	total := observedWeight + virtualWeight

	if total <= 0 {
		return 0
	}

	return (node.Mean*observedWeight + node.CounterfactualMean()*virtualWeight) / total
}

/*
BestChild returns the child with the highest blended economic outcome that has
been evaluated with at least one real rollout, or nil if no such child exists.
*/
func (node *SearchNode) BestChild() *SearchNode {
	if node == nil || len(node.Children) == 0 {
		return nil
	}

	var best *SearchNode

	for _, child := range node.Children {
		if child.Visits == 0 {
			continue
		}

		if best == nil || child.BlendedValue() > best.BlendedValue() ||
			(child.BlendedValue() == best.BlendedValue() &&
				child.EffectiveVisits() > best.EffectiveVisits()) {
			best = child
		}
	}

	return best
}
