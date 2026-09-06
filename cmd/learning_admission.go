package cmd

import (
	"math/big"
	"time"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/* Account exposes producer identity and exact available cash without venue I/O. */
func (bridge *learningDesk) Account() strategy.AccountState {
	return bridge.funds.Observe(bridge.desk.Equity())
}

/* admit rechecks authority, spot depth and cash at the final broker boundary. */
func (bridge *learningDesk) admit(intent strategy.ExecutionIntent, cost *types.EntryCost) error {
	if intent.Reduce {
		return nil
	}

	if intent.Allowed == nil || !intent.Allowed.Load() || (bridge.realization != nil && !bridge.realization.AllowsTrading()) {
		return &types.ExecutionRefusal{State: "authorization blocked", Detail: "increase authority unavailable at placement"}
	}

	if intent.Candidate == nil {
		return &types.ExecutionRefusal{State: "no longer executable", Detail: "prospective candidate required"}
	}

	if err := bridge.desk.PublishEquity(); err != nil {
		return &types.ExecutionRefusal{State: "account unavailable", Detail: err.Error()}
	}

	if _, state := intent.Candidate.Reprice(bridge.books, bridge.instrument.Pair(intent.Symbol), bridge.fee(intent.Symbol), time.Now().UTC()); state != "" {
		return &types.ExecutionRefusal{State: state, Detail: "candidate economics no longer hold"}
	}
	actualCost := new(big.Rat).Add(cost.GrossNotional.Rat(), cost.EntryFee.Rat())

	if actualCost.Cmp(intent.MaximumCost) > 0 {
		return &types.ExecutionRefusal{State: "repricing failed", Detail: "current fee-inclusive cost exceeds frozen commitment"}
	}
	bridge.Account()

	if !bridge.funds.Reserve(intent.CorrelationID, intent.MaximumCost) {
		return &types.ExecutionRefusal{State: "insufficient capital", Detail: "current available quote cash cannot fund commitment"}
	}
	return nil
}

/* refused journals a pre-venue outcome independently from Realization. */
func (bridge *learningDesk) refused(intent strategy.ExecutionIntent, refusal *types.ExecutionRefusal) error {
	intent.Allocation.Report(hindsight.AllocationResult{State: "aborted", At: time.Now().UTC(), Detail: refusal.Error()})
	bridge.refusedCount.Add(1)
	reason := refusal.Error()
	bridge.lastRefusal.Store(&reason)
	bridge.funds.Release(intent.CorrelationID, time.Time{})

	if intent.Candidate == nil {
		return nil
	}
	result := hindsight.CandidateResult{ID: intent.CorrelationID, State: refusal.State, Detail: refusal.Detail, At: time.Now().UTC(), PortfolioID: intent.PortfolioID}
	return bridge.record(hindsight.LearningEvent{ID: intent.Candidate.Record.Decision, Symbol: intent.Symbol, Mode: "candidate", Kind: "candidate_status", At: result.At, CandidateID: result.ID, CandidateResult: &result})
}
