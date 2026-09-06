package strategy

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/types"
)

/*
	CapitalHistory restores completed capital evidence with no account, candidate,

pending allocation or execution authority. It shares the current quantity registry.
*/
type CapitalHistory struct {
	knowledge  *Knowledge
	capital    *CapitalKnowledge
	Unverified int
}

/* Warmup uses prospective inputs and separately attached funding-adjusted labels. */
func (history *CapitalHistory) Warmup(events []hindsight.LearningEvent) (int, error) {
	type identity struct {
		run hindsight.RunID
		id  uint64
	}
	pending := make(map[identity]hindsight.LearningEvent)
	count := 0
	for _, event := range events {
		key := identity{event.Run, event.ID}

		if event.Kind == "portfolio_issued" {
			pending[key] = event
			continue
		}

		if event.Kind == "portfolio_aborted" {
			delete(pending, key)
			continue
		}

		if event.Kind != "portfolio_resolved" {
			continue
		}
		issue, found := pending[key]
		delete(pending, key)

		if !found {
			return count, errnie.Err(errnie.Validation, "capital warmup: incomplete allocation experience", nil)
		}

		if issue.Mode != event.Mode || (issue.Mode != "capital_virtual" && issue.Mode != "capital_account") {
			return count, errnie.Err(errnie.Validation, "capital warmup: teacher source differs from issue", nil)
		}

		if issue.PortfolioID == "" || issue.PortfolioID != event.PortfolioID || event.TargetUnit != "return_per_second" || issue.Account == nil || !issue.Account.HasFunding || event.Account == nil || !event.Account.HasFunding {
			return count, errnie.Err(errnie.Validation, "capital warmup: causal identity, funding and target units required", nil)
		}

		if issue.At.IsZero() || !event.At.After(issue.At) {
			return count, errnie.Err(errnie.Validation, "capital warmup: ordered producer times required", nil)
		}

		if issue.Action != string(types.ActionHold) && (event.Allocation == nil || event.Allocation.State != "filled") {
			history.Unverified++
			continue
		}

		if event.Allocation != nil && (event.Allocation.At.IsZero() || event.Allocation.At.After(event.At)) {
			return count, errnie.Err(errnie.Validation, "capital warmup: fill confirmation must precede its account outcome", nil)
		}
		context := append([]uint64(nil), issue.Context...)
		regions := 0
		for regions < len(context) && context[regions] != 0 {
			regions++
		}

		if regions != len(issue.Quantities) {
			return count, errnie.Err(errnie.Validation, "capital warmup: named quantities required for Region context", nil)
		}
		for index, quantity := range issue.Quantities {
			context[index] = uint64(history.knowledge.grid.Column(quantity[0], quantity[1]) + 1)
		}
		action := CapitalAction{Symbol: issue.CapitalSymbol, Kind: types.Action(issue.Action), Power: issue.Power}

		if err := history.capital.Observe(issue.Mode, context, action, event.Target, issue.Authority); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
