package strategy

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

/*
scaleTo sets proposed notional to the capital freed by a displaced incumbent
so Allocator redeploys that lot instead of inventing a max_fraction size.
*/
func (planner *Planner) scaleTo(decision *types.Decision, notional float64) {
	if notional <= 0 {
		return
	}

	redeploy := decimal.NewFromFloat64(notional)

	if decision.ProposedNotional != nil && decision.ProposedNotional.Sign() > 0 &&
		decision.ProposedQuantity != nil {
		decision.ProposedQuantity = decision.ProposedQuantity.Mul(
			redeploy.Div(decision.ProposedNotional),
		)
	}

	if (decision.ProposedQuantity == nil || decision.ProposedQuantity.Sign() <= 0) &&
		decision.ReferencePrice != nil && decision.ReferencePrice.Sign() > 0 {
		decision.ProposedQuantity = redeploy.Div(decision.ReferencePrice)
	}

	decision.ProposedNotional = redeploy
}

/*
persistRejectedEntry records a slot-exhausted or rotate-rejected entry as an
explicit nothing decision so the Thesis audit trail shows why utility-ranked
intent did not execute.
*/
func (planner *Planner) persistRejectedEntry(
	thesis *types.Thesis,
	decision types.Decision,
	opportunity bool,
	freeNormal int,
	admittedNormal int,
	incumbents []Incumbent,
) {
	decision.Action = types.ActionNothing
	prior := decision.Utility
	decision.Utility = 0
	decision.Cause = "slots_full"
	decision.Reason = "higher-utility entries consumed available slots"
	planner.stampCapital(&decision)

	if decision.Alternatives == nil {
		decision.Alternatives = map[string]float64{}
	}

	if !opportunity && freeNormal <= admittedNormal {
		decision.Reason = "normal slots full; reserved requires opportunity"
	}

	if index, found := weakest(incumbents); found {
		incumbent := incumbents[index]
		edge := prior - incumbent.HoldUtility
		surplus := rotateSurplus(prior, incumbent.HoldUtility, incumbent.ExitCost)
		decision.Alternatives["hold_incumbent"] = incumbent.HoldUtility
		decision.Alternatives["exit_cost"] = incumbent.ExitCost
		decision.Alternatives["clear_score"] = incumbent.ClearScore
		decision.Alternatives["rotate_value"] = edge - incumbent.ExitCost
		decision.Alternatives["wait_value"] = edge * incumbent.ClearScore
		decision.Alternatives["rotate_surplus"] = surplus

		if !shouldRotate(prior, incumbent.HoldUtility, incumbent.ExitCost, incumbent.ClearScore) {
			decision.Cause = "rotate_wait"
			decision.Reason = "challenger does not clear one-step wait threshold against weakest incumbent"
		}
	}

	thesis.Decisions = append(thesis.Decisions, decision)
}

/*
persistAcceptedEntry records lifecycle, decision, and a Thesis holding for Desk
to size and submit. Broker Position construction stays on Desk alone.
*/
func (planner *Planner) persistAcceptedEntry(
	thesis *types.Thesis,
	decision types.Decision,
	opportunity bool,
) {
	planner.stampCapital(&decision)
	thesis.Lifecycle.Store(decision.Symbol, types.LifecycleEntrySelected)
	thesis.Decisions = append(thesis.Decisions, decision)

	holding := types.NewHolding(planner.ctx, decision.Symbol, decision.ProposedQuantity)
	holding.IsOpportunity = opportunity

	// Nil qty means Allocator must size from max_fraction; rotation may leave
	// a positive redeploy quantity already on the decision.
	if decision.ProposedQuantity == nil || decision.ProposedQuantity.Sign() <= 0 {
		holding.Qty = nil
	}

	thesis.Holdings.Store(decision.Symbol, holding)
}

/*
stampCapital records wallet cash visible at admit time on the decision.
*/
func (planner *Planner) stampCapital(decision *types.Decision) {
	if planner.balance == nil || decision == nil {
		return
	}

	cash, err := planner.balance.AvailableQuote()

	if err != nil {
		return
	}

	decision.AvailableCapital = decimal.NewFromFloat64(cash)

	if planner.desk != nil {
		decision.SlotCapacity = planner.desk.MaxSlots()
		decision.OpenPositions = planner.desk.OpenPositions()
	}
}
