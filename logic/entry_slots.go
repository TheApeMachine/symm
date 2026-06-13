package logic

import "github.com/theapemachine/symm/config"

/*
EntrySlotOccupancy tracks held and in-flight entry slots for admission.
*/
type EntrySlotOccupancy struct {
	BaseHeld           int
	OpportunityHeld    int
	BasePending        int
	OpportunityPending int
}

/*
EntrySlotOccupancyFromHoldings snapshots confirmed inventory slots.
*/
func EntrySlotOccupancyFromHoldings(holdings *Holdings) EntrySlotOccupancy {
	if holdings == nil {
		return EntrySlotOccupancy{}
	}

	return EntrySlotOccupancy{
		BaseHeld:        holdings.BaseSlotCount(),
		OpportunityHeld: holdings.OpportunitySlotCount(),
	}
}

func (occupancy EntrySlotOccupancy) OpenCount() int {
	return occupancy.BaseHeld + occupancy.OpportunityHeld
}

func (occupancy EntrySlotOccupancy) PendingCount() int {
	return occupancy.BasePending + occupancy.OpportunityPending
}

func (occupancy EntrySlotOccupancy) CommittedCount() int {
	return occupancy.OpenCount() + occupancy.PendingCount()
}

func (occupancy EntrySlotOccupancy) BaseCommitted() int {
	return occupancy.BaseHeld + occupancy.BasePending
}

func (occupancy EntrySlotOccupancy) OpportunityCommitted() int {
	return occupancy.OpportunityHeld + occupancy.OpportunityPending
}

/*
EntrySlotAdmission decides whether a new entry may open and whether it consumes
a reserved opportunity slot instead of a base slot.
*/
func EntrySlotAdmission(
	occupancy EntrySlotOccupancy,
	tradingConfig config.TradingConfig,
	qualifiesForOpportunity bool,
) (allowed bool, opportunitySlot bool) {
	totalCapacity := tradingConfig.MaxConcurrentPositions +
		tradingConfig.OpportunitySlotCount

	if occupancy.CommittedCount() >= totalCapacity {
		return false, false
	}

	if occupancy.BaseCommitted() < tradingConfig.MaxConcurrentPositions {
		return true, false
	}

	if !qualifiesForOpportunity {
		return false, false
	}

	if occupancy.OpportunityCommitted() >= tradingConfig.OpportunitySlotCount {
		return false, false
	}

	return true, true
}
