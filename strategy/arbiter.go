package strategy

import (
	"sort"

	"github.com/theapemachine/api-go/v2/pkg/decimal"
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
	enters := make([]*types.Decision, 0)
	openPositions := arbiter.desk.OpenPositions()
	normalSlots := arbiter.desk.OpenSlots(false)
	opportunitySlots := arbiter.desk.OpenSlots(true)
	normalCapacity := openPositions + normalSlots
	opportunityCapacity := openPositions + opportunitySlots

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

	for _, candidate := range enters {
		candidate.OpenPositions = openPositions
		candidate.SlotCapacity = normalCapacity

		if normalSlots > 0 {
			candidate.AllocationClass = "normal"
			normalSlots--
			opportunitySlots--
			openPositions++
			thesis.Lifecycle.Store(candidate.Symbol, types.LifecycleEntrySelected)
			continue
		}

		if candidate.Opportunity && opportunitySlots > 0 {
			candidate.AllocationClass = "reserved"
			candidate.SlotCapacity = opportunityCapacity
			opportunitySlots--
			openPositions++

			if candidate.OpportunityMargin <= 0 {
				candidate.OpportunityMargin = candidate.Utility
			}

			thesis.Lifecycle.Store(candidate.Symbol, types.LifecycleEntrySelected)
			continue
		}

		// Open inventory belongs to its stoploss, so a challenger waits for a slot.
		candidate.Action = types.ActionNothing
		candidate.AllocationClass = ""
		candidate.Cause = "slots_full"
		candidate.Reason = "all entry slots are occupied by stop-governed positions"
	}

	thesis.Stamp(types.SourceArbiter)
}
