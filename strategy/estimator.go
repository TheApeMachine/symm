package strategy

import (
	"github.com/theapemachine/symm/logic/advisor"
	"github.com/theapemachine/symm/nomagique/mcts"
	"github.com/theapemachine/symm/types"
)

/*
opportunityEstimator supplies the causal economic estimate for one action.

It is the boundary the MCTS README names: identification status and model
support enter the search here, and an action whose outcome cannot be defensibly
estimated is reported Undefined rather than being handed a fabricated zero.

The estimator does not itself price outcomes — the search's rollouts do that
against the economic reward. What it decides is whether an action is
*estimable at all* given the evidence actually present: a calibrated resonance
forecast, a synthesized opportunity, and a War Room consensus that does not veto the direction.
*/
type consensusEstimator struct {
	// consensus is the deliberated market-move distribution for this symbol.
	consensus *advisor.DeliberationOutcome

	// opportunity is the synthesized quality-adjusted opportunity hypothesis.
	opportunity *types.OpportunityCandidate
}

/*
EstimateAction reports whether one action has a defensible causal estimate.

Exit and Wait are always estimable: they are the safe actions, and refusing to
estimate them would leave a held position with no way out. Enter and Scale are
estimable only when the synthesized opportunity is identified and supported by
the quality-adjusted evidence: a consensus exists, and its dominant move is not bearish.
*/
func (estimator *consensusEstimator) EstimateAction(
	state mcts.State,
	action mcts.Action,
) mcts.ActionEstimate {
	estimate := mcts.ActionEstimate{Action: action}

	switch action {
	case mcts.Wait, mcts.Exit:
		// The safe actions are always estimable. Their consequences are
		// priced by the rollout, not gated by evidence.
		estimate.Defined = true
		estimate.IdentificationStatus = mcts.IdentificationIdentified
		estimate.Support = 1

		return estimate

	case mcts.Enter, mcts.Scale:
		if estimator.consensus == nil {
			estimate.IdentificationStatus = mcts.IdentificationNotIdentifiable

			return estimate
		}

		if estimator.opportunity == nil {
			estimator.opportunity = SynthesizeOpportunity(OpportunityInput{
				Consensus: estimator.consensus,
			})
		}

		if estimator.opportunity == nil {
			// Without a coherent, quality-adjusted opportunity,
			// entering or scaling is unidentifiable. Waiting is the ordinary outcome.
			estimate.IdentificationStatus = mcts.IdentificationUnsupportedTreatment

			return estimate
		}

		estimate.Defined = true
		estimate.IdentificationStatus = mcts.IdentificationIdentified
		estimate.Support = estimator.consensus.Confidence

		return estimate

	default:
		estimate.IdentificationStatus = mcts.IdentificationUndefined

		return estimate
	}
}

