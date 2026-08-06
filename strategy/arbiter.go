package strategy

import (
	"sort"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

type Arbiter struct {
	desk *broker.Desk
}

func NewArbiter(desk *broker.Desk) *Arbiter {
	return &Arbiter{desk: desk}
}

// Arbitrate ranks candidate entries and allocates available slots.
func (arbiter *Arbiter) Arbitrate(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	enters := make([]*types.Decision, 0)

	// The map holds pointers, so a verdict revised here is revised on the
	// thesis. Only the entries are collected, and only because they have to be
	// ranked against each other before any of them can claim a slot.
	thesis.Decisions.Range(func(key, value any) bool {
		decision, ok := value.(*types.Decision)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"arbiter: decision map holds a value that is not a decision",
				nil,
			))

			return true
		}

		if decision.Action == types.ActionEnter {
			enters = append(enters, decision)
			return true
		}

		if decision.Action == types.ActionExit {
			decision.Action = types.ActionHold
			decision.Cause = "stoploss_only"
			decision.Reason = "strategy exit suppressed; open position is governed by its stoploss"
			decision.ProposedQuantity = decimal.NewFromInt64(0)
		}

		return true
	})

	// Sort enter candidates by Opportunity first, then Utility descending
	sort.Slice(enters, func(left, right int) bool {
		if enters[left].Opportunity != enters[right].Opportunity {
			return enters[left].Opportunity
		}

		return enters[left].Utility > enters[right].Utility
	})

	openSlots := max(arbiter.desk.OpenSlots(false), 0)
	totalSlots := max(arbiter.desk.OpenSlots(true), 0)
	reserveSlots := max(totalSlots-openSlots, 0)
	/*
		Capacity and occupancy are recorded on every decision, because a
		decision to take a slot is only auditable next to the budget it was
		taken against.
	*/
	capacity := arbiter.desk.MaxPositions()
	open := arbiter.desk.OpenPositions()

	for _, candidate := range enters {
		candidate.SlotCapacity = capacity
		candidate.OpenPositions = open

		if openSlots > 0 {
			candidate.AllocationClass = "normal"
			thesis.Lifecycle.Store(candidate.Symbol, types.LifecycleEntrySelected)
			openSlots--
			continue
		}

		/*
			Normal capacity is gone, which is the case the reserve is held for:
			a pump worth interrupting a working desk. Claiming a reserve slot
			is marked as such rather than left looking like a normal entry.
		*/
		if reserveSlots > 0 && candidate.Opportunity {
			candidate.AllocationClass = "reserved"

			if candidate.OpportunityMargin <= 0 {
				candidate.OpportunityMargin = candidate.Utility
			}

			thesis.Lifecycle.Store(candidate.Symbol, types.LifecycleEntrySelected)
			reserveSlots--
			continue
		}

		// Open inventory belongs to its stoploss, so a challenger waits for a slot.
		candidate.Action = types.ActionNothing
		candidate.Cause = "slots_full"
		candidate.Reason = "all entry slots are occupied by stop-governed positions"
	}

	thesis.Stamp(types.SourceArbiter)
}
