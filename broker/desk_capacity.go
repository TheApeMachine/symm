package broker

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
)

func (desk *Desk) validateEntryCapacity(action *logic.Action) error {
	if desk == nil || action == nil {
		return nil
	}

	if action.Type.IsExit() || action.Side != trading.Buy {
		return nil
	}

	if desk.positions != nil && desk.positions.HasOpen(action.Symbol) {
		return nil
	}

	occupancy := desk.entrySlotOccupancy()
	allowed, _ := logic.EntrySlotAdmission(
		occupancy,
		desk.tradingConfig,
		action.OpportunitySlot,
	)

	if allowed {
		return nil
	}

	totalCapacity := desk.tradingConfig.MaxConcurrentPositions +
		desk.tradingConfig.OpportunitySlotCount

	return errnie.Error(errnie.Err(
		errnie.Validation,
		"broker: entry capacity exhausted",
		errnie.Require(map[string]any{
			"symbol":           action.Symbol,
			"committed_count":  occupancy.CommittedCount(),
			"total_capacity":   totalCapacity,
			"opportunity_slot": action.OpportunitySlot,
		}),
	))
}

func (desk *Desk) entrySlotOccupancy() logic.EntrySlotOccupancy {
	occupancy := logic.EntrySlotOccupancy{}

	if desk == nil {
		return occupancy
	}

	if desk.positions != nil {
		openCount := desk.positions.OpenCount()
		occupancy.BaseHeld = openCount
	}

	if desk.actions == nil {
		return occupancy
	}

	desk.actions.Range(func(_, value any) bool {
		pendingAction, ok := value.(*logic.Action)

		if !ok || pendingAction == nil {
			return true
		}

		if pendingAction.Type.IsExit() || pendingAction.Side != trading.Buy {
			return true
		}

		if desk.positions != nil && desk.positions.HasOpen(pendingAction.Symbol) {
			return true
		}

		if pendingAction.OpportunitySlot {
			occupancy.OpportunityPending++
			return true
		}

		occupancy.BasePending++

		return true
	})

	return occupancy
}
