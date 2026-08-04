package strategy

import (
	"sort"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
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

	enters := make([]types.Decision, 0, len(thesis.Decisions))
	retained := make([]types.Decision, 0, len(thesis.Decisions))

	for _, decision := range thesis.Decisions {
		if decision.Action == types.ActionEnter {
			enters = append(enters, decision)
			continue
		}

		if decision.Action == types.ActionExit {
			decision.Action = types.ActionHold
			decision.Cause = "stoploss_only"
			decision.Reason = "strategy exit suppressed; open position is governed by its stoploss"
			decision.ProposedQuantity = decimal.NewFromInt64(0)
		}

		retained = append(retained, decision)
	}

	// Sort enter candidates by Opportunity first, then Utility descending
	sort.Slice(enters, func(i, j int) bool {
		if enters[i].Opportunity != enters[j].Opportunity {
			return enters[i].Opportunity
		}

		return enters[i].Utility > enters[j].Utility
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
			thesis.NoteLifecycle(candidate.Symbol, types.LifecycleEntrySelected, thesis.At)
			retained = append(retained, candidate)
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

			thesis.NoteLifecycle(candidate.Symbol, types.LifecycleEntrySelected, thesis.At)
			retained = append(retained, candidate)
			reserveSlots--
			continue
		}

		// Open inventory belongs to its stoploss, so a challenger waits for a slot.
		candidate.Action = types.ActionNothing
		candidate.Cause = "slots_full"
		candidate.Reason = "all entry slots are occupied by stop-governed positions"
		retained = append(retained, candidate)
	}

	thesis.Decisions = retained
}
