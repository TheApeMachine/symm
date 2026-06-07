package execution

import (
	"math"

	"github.com/spf13/viper"
)

const defaultMaxConcurrentPositions = 16

/*
MaxConcurrentPositions is the hard ceiling on open plus pending entries. Deploy
size comes from position_fraction; this count comes from here — never floor(1/fraction).
*/
func MaxConcurrentPositions() int {
	if viper.IsSet("trading.max_concurrent_positions") {
		limit := viper.GetInt("trading.max_concurrent_positions")

		if limit > 0 {
			return limit
		}
	}

	return defaultMaxConcurrentPositions
}

/*
AffordableSlots reports how many deployFraction-sized entries fit in affordableCash
against capitalBase (fee-aware slot spend matches EntrySlotSpend).
*/
func AffordableSlots(
	capitalBase float64,
	deployFraction float64,
	feeRate float64,
	affordableCash float64,
) int {
	slotSpend := EntrySlotSpend(capitalBase, deployFraction, feeRate, affordableCash)

	if slotSpend <= 0 {
		return 0
	}

	deployCash := slotSpend * (1 + feeRate)

	if deployCash <= 0 {
		return 0
	}

	return int(math.Floor(affordableCash/deployCash + 1e-9))
}

/*
EntrySlotAvailable reports whether another entry may reserve capital. Slot count
comes from the capital base (and max_concurrent_positions); affordableCash only
gates whether the next slot can still be paid for.
*/
func EntrySlotAvailable(
	openCount int,
	deployFraction float64,
	capitalBase float64,
	affordableCash float64,
	feeRate float64,
) bool {
	if deployFraction <= 0 || capitalBase <= 0 {
		return false
	}

	capacityByBase := AffordableSlots(capitalBase, deployFraction, feeRate, capitalBase)

	if capacityByBase <= 0 {
		return false
	}

	limit := capacityByBase

	if maxConcurrent := MaxConcurrentPositions(); maxConcurrent < limit {
		limit = maxConcurrent
	}

	if openCount >= limit {
		return false
	}

	return EntrySlotSpend(capitalBase, deployFraction, feeRate, affordableCash) > 0
}
