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
*estimable at all* given the evidence actually present: an armed opportunity,
a calibrated resonance forecast, and a War Room consensus that does not veto
the direction.
*/
type opportunityEstimator struct {
	// consensus is the deliberated market-move distribution for this symbol.
	consensus *advisor.DeliberationOutcome
	// opportunity is the tracked candidate driving this planning round.
	opportunity types.OpportunityCandidate
	// entryAdmissible reports whether the precursor phase permits opening a
	// new position at all.
	entryAdmissible bool
}

/*
EstimateAction reports whether one action has a defensible causal estimate.

Exit and Wait are always estimable: they are the safe actions, and refusing to
estimate them would leave a held position with no way out. Enter and Scale are
estimable only when the evidence supports opening or adding exposure — an armed
precursor, and a consensus whose dominant move is not bearish.
*/
func (estimator *opportunityEstimator) EstimateAction(
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
		if !estimator.entryAdmissible {
			// The precursor has not armed, or has already ignited. Entering
			// is not estimable, which is distinct from being estimated as
			// bad: the search records it undefined.
			estimate.IdentificationStatus = mcts.IdentificationInsufficientSupport

			return estimate
		}

		if estimator.consensus == nil {
			estimate.IdentificationStatus = mcts.IdentificationNotIdentifiable

			return estimate
		}

		if estimator.consensus.DominantMove < advisor.MoveWeakDrift {
			// The War Room's deliberated consensus does not support adding
			// exposure. This is a veto on identification, not a penalty
			// applied to a reward.
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

/*
entryAdmissible reports whether an opportunity's phase permits a new entry.

This is the precursor rule the architecture mandates: position during
PhaseArmed, and never open a new entry once PhaseIgnition has printed, because
by then the move is visible and the entry is buying someone else's exit
liquidity.
*/
func entryAdmissible(candidate types.OpportunityCandidate) bool {
	return candidate.Phase == types.PhaseArmed &&
		candidate.Direction == types.DirectionLong
}
