package hindsight

/*
counterfactualQuantity derives the quantity a missed opportunity should be
evaluated against, from the recorded decision/valuation/allocation state alone.
It never invents a size: one unit, a fixed dollar, the full wallet, an arbitrary
percentage, and maximum book capacity are all rejected. It returns ok=false when
no defensible quantity exists, in which case the executable opportunity is
undefined (not zero).
*/
func counterfactualQuantity(decisions []Decision, leg Leg) (float64, bool) {
	decision := bestDecisionFor(decisions, leg)

	if decision == nil {
		return 0, false
	}

	// The decision's own proposed size is the primary defensible quantity.
	if decision.ProposedQuantity.Float() > 0 {
		return decision.ProposedQuantity.Float(), true
	}

	// Otherwise reconstruct from proposed notional against the reference price.
	if decision.ProposedNotional.Float() > 0 && decision.EntryCost.EntryPrice.Float() > 0 {
		quantity := decision.ProposedNotional.Float() / decision.EntryCost.EntryPrice.Float()

		if quantity > 0 {
			return quantity, true
		}
	}

	return 0, false
}

/*
counterfactualFeeRate derives the taker fee rate (as a fraction) to charge the
counterfactual round trip, from the recorded entry risk plan. When the plan is
absent it returns a zero fee rate rather than inventing a rate — fee omission is
the honest floor only when no fee was recorded.
*/
func counterfactualFeeRate(decisions []Decision, leg Leg) float64 {
	decision := bestDecisionFor(decisions, leg)

	if decision == nil {
		return 0
	}

	// Risk.EntryFeeRate is the fraction the entry crossing paid.
	return decision.Risk.EntryFeeRate.Float()
}

/*
regretLayers assigns evidence to the correct regret layer for a missed leg:
detection (opportunity observable/detected), valuation (economic consequence
estimable), selection (what MCTS compared/selected), execution (proposed vs
executable size), and management (the risk decision). Each layer answers only
its own question.
*/
func regretLayers(
	context SignalContext,
	leg Leg,
	recorded bool,
	observerAvailable bool,
) RegretLayer {
	regret := RegretLayer{}

	// Detection: was the opportunity observable and detected before the move?
	if !recorded || !context.Opportunity {
		regret.Detection = true
	}

	// Valuation: was economic consequence estimable at the decision point?
	if !context.ValuationAttempted || !context.ValuationAvailable {
		regret.Valuation = true
	}

	// Selection: did MCTS actually compare/select a non-CASH/WAIT action?
	if context.UtilityAvailable {
		if context.MCTS.RecommendedAction == "" ||
			context.MCTS.RecommendedAction == "nothing" ||
			context.MCTS.RecommendedAction == "wait" {
			regret.Selection = true
		}
	} else {
		regret.Selection = true
	}

	// Execution: was the proposed size actually executable? When no quantity
	// is recorded, execution itself could not be judged — the layer stays false.
	if context.ProposedQuantity.Float() > 0 {
		regret.Execution = true
	}

	// Management: no position was opened, so there is no management regret.
	// This remains false unless a position was entered and managed badly,
	// which is the loss path, not the missed-leg path.
	_ = observerAvailable

	return regret
}
