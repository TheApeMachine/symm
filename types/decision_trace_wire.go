package types

import (
	"sort"

	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
decisionTraceWire projects the reasoning record onto the telemetry schema so
the operator surface can render the live decision process.
*/
func decisionTraceWire(trace *DecisionTrace) *telemetry.DecisionTraceT {
	if trace == nil {
		return nil
	}

	search := trace.MCTS
	council := trace.Deliberation

	branches := make([]*telemetry.MCTSBranchT, 0, len(search.Branches))

	for _, branch := range search.Branches {
		branches = append(branches, &telemetry.MCTSBranchT{
			Action:                   branch.Action,
			Visits:                   int64(branch.Visits),
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

	// Sorted so the same council reading always projects to the same wire
	// order, matching how Decision.Alternatives is emitted.
	moves := make([]string, 0, len(council.Probabilities))

	for move := range council.Probabilities {
		moves = append(moves, move)
	}

	sort.Strings(moves)
	probabilities := make([]*telemetry.NamedNumberT, 0, len(moves))

	for _, move := range moves {
		probabilities = append(probabilities, &telemetry.NamedNumberT{
			Name:  move,
			Value: council.Probabilities[move],
		})
	}

	advisors := make([]*telemetry.AdvisorOpinionT, 0, len(council.Advisors))

	for _, opinion := range council.Advisors {
		classes := make([]*telemetry.AdvisorClassT, 0, len(opinion.Classes))

		for _, class := range opinion.Classes {
			classes = append(classes, &telemetry.AdvisorClassT{
				State:       class.State,
				Probability: class.Probability,
			})
		}

		contribution := make([]*telemetry.AdvisorMoveMassT, 0, len(opinion.Contribution))

		for _, mass := range opinion.Contribution {
			contribution = append(contribution, &telemetry.AdvisorMoveMassT{
				Move: mass.Move,
				Mass: mass.Mass,
			})
		}

		advisors = append(advisors, &telemetry.AdvisorOpinionT{
			Advisor:      opinion.Advisor,
			State:        opinion.State,
			Probability:  opinion.Probability,
			Credibility:  opinion.Credibility,
			Weight:       opinion.Weight,
			Classes:      classes,
			Maturity:     opinion.Maturity,
			Contribution: contribution,
			Unmapped:     append([]string(nil), opinion.Unmapped...),
			Unscored:     append([]string(nil), opinion.Unscored...),
			Clock:        opinion.Clock,
			LeaseFrom:    opinion.LeaseFrom,
			LeaseUntil:   opinion.LeaseUntil,
			ClockNow:     opinion.ClockNow,
		})
	}

	silences := make([]*telemetry.AdvisorSilenceT, 0, len(council.Silent))

	for _, silence := range council.Silent {
		silences = append(silences, &telemetry.AdvisorSilenceT{
			Advisor:    silence.Advisor,
			Reason:     silence.Reason,
			Missing:    append([]string(nil), silence.Missing...),
			Declared:   int32(silence.Declared),
			LeaseUntil: silence.LeaseUntil,
			ClockNow:   silence.ClockNow,
		})
	}

	return &telemetry.DecisionTraceT{
		Iterations:               int64(search.Iterations),
		Horizon:                  int64(search.Horizon),
		ExplorationConstant:      search.ExplorationConstant,
		UncertaintyWeight:        search.UncertaintyWeight,
		RecommendedAction:        search.RecommendedAction,
		ExpectedOutcome:          search.ExpectedOutcome,
		OutcomeUncertainty:       search.OutcomeUncertainty,
		IdentificationStatus:     search.IdentificationStatus,
		DecisionUnavailable:      search.DecisionUnavailable,
		TransitionSource:         search.TransitionSource,
		Branches:                 branches,
		Tree:                     mctsNodeWire(search.Tree),
		MaxDepth:                 int64(search.MaxDepth),
		TotalNodes:               int64(search.TotalNodes),
		ConsensusDominantMove:    council.DominantMove,
		ConsensusConfidence:      council.Confidence,
		ConsensusParticipants:    int64(council.Participants),
		ConsensusProbabilities:   probabilities,
		Vetoes:                   append([]string(nil), council.Vetoes...),
		Synergies:                append([]string(nil), council.Synergies...),
		Advisors:                 advisors,
		AdvisorSilences:          silences,
		ConsensusUnmappedClasses: append([]string(nil), council.UnmappedClasses...),
	}
}

/*
mctsNodeWire projects one search node and its subtree.
*/
func mctsNodeWire(node *MCTSNodeTrace) *telemetry.MCTSNodeT {
	if node == nil {
		return nil
	}

	wire := &telemetry.MCTSNodeT{
		ActionName:               node.Action,
		Depth:                    int64(node.Depth),
		Visits:                   int64(node.Visits),
		EffectiveVisits:          node.EffectiveVisits,
		MeanReward:               node.MeanReward,
		RewardStd:                node.RewardStd,
		BlendedValue:             node.BlendedValue,
		CounterfactualReward:     node.CounterfactualReward,
		CounterfactualMass:       node.CounterfactualMass,
		CounterfactualPrecision:  node.CounterfactualPrecision,
		CausalExpectation:        node.CausalExpectation,
		CausalExpectationDefined: node.CausalExpectationDefined,
		Pruned:                   node.Pruned,
		Selected:                 node.Selected,
	}

	if len(node.Children) == 0 {
		return wire
	}

	wire.Children = make([]*telemetry.MCTSNodeT, 0, len(node.Children))

	for index := range node.Children {
		wire.Children = append(wire.Children, mctsNodeWire(&node.Children[index]))
	}

	return wire
}
