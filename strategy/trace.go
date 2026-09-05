package strategy

import (
	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/types"
)

/*
buildTrace records the full reasoning behind one decision round so an operator
can watch the process live rather than only its verdict.

The trace keeps real rollout evidence and virtual counterfactual evidence
separate at every level. A branch that won because Pearl's third rung filled in
its value should look different on screen from one that won by being rolled out
repeatedly, and pre-blending the two would erase exactly that distinction.
*/
func buildTrace(
	consensus *advisor.DeliberationOutcome,
	result *mcts.SearchResult,
	transitionSource string,
) *types.DecisionTrace {
	trace := &types.DecisionTrace{
		Deliberation: deliberationTrace(consensus),
	}

	if result == nil {
		return trace
	}

	trace.MCTS = types.MCTSTrace{
		RecommendedAction:    result.SelectedAction.String(),
		ExpectedOutcome:      result.ExpectedEconomicOutcome,
		OutcomeUncertainty:   result.OutcomeUncertainty,
		IdentificationStatus: result.IdentificationStatus.String(),
		DecisionUnavailable:  result.DecisionUnavailable,
		TransitionSource:     transitionSource,
	}

	if result.Trace != nil {
		trace.MCTS.Iterations = result.Trace.Iterations
		trace.MCTS.Horizon = result.Trace.Horizon
		trace.MCTS.ExplorationConstant = result.Trace.ExplorationConstant
		trace.MCTS.UncertaintyWeight = result.Trace.UncertaintyWeight
		trace.MCTS.Branches = branchTraces(result.Trace.Branches)
	}

	if result.Tree != nil {
		tree := nodeTrace(result.Tree, result.SelectedAction, 0, true)
		trace.MCTS.Tree = &tree
		trace.MCTS.MaxDepth = treeDepth(&tree)
		trace.MCTS.TotalNodes = countNodes(&tree)
	}

	return trace
}

/*
deliberationTrace records the council's consensus and the readings that shaped
it, keyed by move name so the surface is not coupled to the move ordinals.
*/
func deliberationTrace(consensus *advisor.DeliberationOutcome) types.DeliberationTrace {
	if consensus == nil {
		return types.DeliberationTrace{}
	}

	probabilities := make(map[string]float64, len(consensus.Probabilities))

	for move, probability := range consensus.Probabilities {
		probabilities[move.String()] = probability
	}

	return types.DeliberationTrace{
		DominantMove:  consensus.DominantMove.String(),
		Confidence:    consensus.Confidence,
		Participants:  consensus.Participants,
		Probabilities: probabilities,
		Vetoes:        append([]string(nil), consensus.Vetoes...),
		Synergies:     append([]string(nil), consensus.Synergies...),
		Advisors:      append([]types.AdvisorOpinion(nil), consensus.Advisors...),
		Silent:        append([]types.AdvisorSilence(nil), consensus.Silent...),
		UnmappedClasses: append(
			[]string(nil), consensus.UnmappedClasses...,
		),
	}
}

func branchTraces(branches []mcts.BranchTrace) []types.MCTSBranchTrace {
	traces := make([]types.MCTSBranchTrace, 0, len(branches))

	for _, branch := range branches {
		traces = append(traces, types.MCTSBranchTrace{
			Action:                   branch.Action.String(),
			Visits:                   branch.Visits,
			MeanReward:               branch.MeanReward,
			RewardStd:                branch.RewardStd,
			BlendedValue:             branch.BlendedValue,
			CounterfactualReward:     branch.CounterfactualReward,
			CounterfactualMass:       branch.CounterfactualMass,
			CounterfactualMean:       branch.CounterfactualMean,
			EffectiveVisits:          branch.EffectiveVisits,
			CausalExpectation:        branch.CausalExpectation,
			CausalExpectationDefined: branch.CausalExpectationDefined,
			Pruned:                   branch.Pruned,
		})
	}

	return traces
}

/*
nodeTrace projects one search node and its subtree, illuminating the complete
principal variation trajectory chosen by the search across all depth levels.
*/
func nodeTrace(
	node *mcts.SearchNode,
	selected mcts.Action,
	depth int,
	onPath bool,
) types.MCTSNodeTrace {
	action := node.Action.String()

	if depth == 0 {
		action = "root"
	}

	trace := types.MCTSNodeTrace{
		Action:                   action,
		Depth:                    depth,
		Visits:                   node.Visits,
		EffectiveVisits:          node.EffectiveVisits(),
		MeanReward:               node.MeanReward(),
		RewardStd:                node.RewardStandardDeviation(),
		BlendedValue:             node.BlendedValue(),
		CounterfactualReward:     node.CounterfactualReward,
		CounterfactualMass:       node.CounterfactualMass,
		CounterfactualPrecision:  node.CounterfactualPrecision,
		CausalExpectation:        node.CausalExpectation,
		CausalExpectationDefined: node.CausalExpectationDefined,
		Pruned:                   node.Pruned,
		Selected:                 onPath && depth > 0,
	}

	if len(node.Children) == 0 {
		return trace
	}

	var nextOnPath *mcts.SearchNode

	if depth == 0 {
		for _, child := range node.Children {
			if child.Action == selected {
				nextOnPath = child
				break
			}
		}
	} else if onPath {
		nextOnPath = node.BestChild()
	}

	trace.Children = make([]types.MCTSNodeTrace, 0, len(node.Children))

	for _, child := range node.Children {
		childOnPath := (child == nextOnPath)
		trace.Children = append(trace.Children, nodeTrace(child, selected, depth+1, childOnPath))
	}

	return trace
}

func treeDepth(node *types.MCTSNodeTrace) int {
	deepest := node.Depth

	for index := range node.Children {
		if depth := treeDepth(&node.Children[index]); depth > deepest {
			deepest = depth
		}
	}

	return deepest
}

func countNodes(node *types.MCTSNodeTrace) int {
	total := 1

	for index := range node.Children {
		total += countNodes(&node.Children[index])
	}

	return total
}
