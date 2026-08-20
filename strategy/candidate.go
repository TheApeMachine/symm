package strategy

import (
	"github.com/theapemachine/symm/types"
)

func (planner *Planner) removeCandidate(symbol string) {
	planner.candidateMu.Lock()
	delete(planner.candidates, symbol)
	planner.candidateMu.Unlock()
}

func (planner *Planner) hasCandidates() bool {
	planner.candidateMu.Lock()
	defer planner.candidateMu.Unlock()

	return len(planner.candidates) > 0
}

func (planner *Planner) retainCandidate(decision *types.Decision) {
	if planner == nil || decision == nil || decision.Symbol == "" {
		return
	}

	planner.candidateMu.Lock()
	defer planner.candidateMu.Unlock()

	if planner.candidates == nil {
		planner.candidates = make(map[string]*types.Decision)
	}

	if decision.Action != types.ActionEnter {
		delete(planner.candidates, decision.Symbol)
		return
	}

	candidate := *decision

	candidate.AvailableCapital = nil
	candidate.ProposedNotional = nil
	candidate.ProposedQuantity = nil
	candidate.EntryPrice = nil
	candidate.Mark = nil
	candidate.Stoploss = nil
	candidate.ExpectedFees = nil
	candidate.ExpectedSpread = nil
	candidate.ExpectedImpact = nil
	candidate.EntryCost = nil
	candidate.Utility = 0
	candidate.OpportunityMargin = 0

	planner.candidates[decision.Symbol] = &candidate
}

func (planner *Planner) candidateCopies() []*types.Decision {
	planner.candidateMu.Lock()
	defer planner.candidateMu.Unlock()

	decisions := make([]*types.Decision, 0, len(planner.candidates))

	for symbol, retained := range planner.candidates {
		if retained == nil {
			delete(planner.candidates, symbol)
			continue
		}

		if planner.Holding(symbol) {
			delete(planner.candidates, symbol)
			continue
		}

		candidate := *retained
		candidate.Action = types.ActionEnter
		candidate.Reason = ""
		candidate.Stoploss = nil

		decisions = append(decisions, &candidate)
	}

	return decisions
}
