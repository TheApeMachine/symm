package logic

import "github.com/theapemachine/symm/config"

/*
EntrySlotAdmission decides whether a new entry may open and whether it consumes
a reserved opportunity slot instead of a base slot.
*/
func EntrySlotAdmission(
	holdings *Holdings,
	tradingConfig config.TradingConfig,
	qualifiesForOpportunity bool,
) (allowed bool, opportunitySlot bool) {
	if holdings == nil {
		return false, false
	}

	openCount := holdings.OpenCount()
	totalCapacity := tradingConfig.MaxConcurrentPositions +
		tradingConfig.OpportunitySlotCount

	if openCount >= totalCapacity {
		return false, false
	}

	baseUsed := holdings.BaseSlotCount()

	if baseUsed < tradingConfig.MaxConcurrentPositions {
		return true, false
	}

	if !qualifiesForOpportunity {
		return false, false
	}

	if holdings.OpportunitySlotCount() >= tradingConfig.OpportunitySlotCount {
		return false, false
	}

	return true, true
}
