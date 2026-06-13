package broker

import "math"

/*
DeriveEntryArmOffset sizes the first trail so bid at fill sits above the stop.
economicEntry is fee-adjusted ask-side fill; exitBid is the touch bid at arm time.
*/
func DeriveEntryArmOffset(spreadBps, economicEntry, exitBid float64) float64 {
	base := DeriveTrailOffset(spreadBps, 0)

	if economicEntry <= 0 || exitBid <= 0 || exitBid >= economicEntry {
		return base
	}

	frictionOffset := (economicEntry - exitBid) / economicEntry
	spreadPct := spreadBps / 10000
	buffer := spreadPct / 4

	if buffer <= 0 {
		buffer = frictionOffset / 8
	}

	required := frictionOffset + buffer

	if required > base {
		return required
	}

	return base
}

/*
DeriveTrailOffset sizes the trailing stop from live spread and realized micro-volatility.
*/
func DeriveTrailOffset(spreadBps, volatilityRatio float64) float64 {
	spreadPct := spreadBps / 10000
	offset := spreadPct + volatilityRatio
	floor := DeriveStopFloor(spreadBps, volatilityRatio)

	if floor > 0 && offset < floor {
		return floor
	}

	if offset <= 0 && volatilityRatio > 0 {
		return volatilityRatio
	}

	return offset
}

/*
DeriveStopFloor is the minimum trail width implied by the touch spread and tape vol.
*/
func DeriveStopFloor(spreadBps, volatilityRatio float64) float64 {
	spreadPct := spreadBps / 10000
	floor := spreadPct * 2

	if volatilityRatio > floor {
		floor = volatilityRatio
	}

	return floor
}

/*
DeriveMaxInitialRisk caps the hard stop from the trail offset and current vol expansion.
*/
func DeriveMaxInitialRisk(trailOffset, volatilityRatio float64) float64 {
	if trailOffset <= 0 {
		return DeriveStopFloor(0, volatilityRatio)
	}

	volatilityRatio = math.Max(0, volatilityRatio)

	return trailOffset * (1.0 + volatilityRatio)
}

/*
ClampDerivedTrailOffset bounds a trail offset using spread- and vol-derived rails.
*/
func ClampDerivedTrailOffset(offset, spreadBps, volatilityRatio float64) float64 {
	minTrail, maxTrail := DerivedTrailBounds(spreadBps, volatilityRatio)

	if minTrail > 0 && offset < minTrail {
		return minTrail
	}

	if maxTrail > 0 && offset > maxTrail {
		return maxTrail
	}

	return offset
}

/*
DerivedTrailBounds returns dynamic min/max trail percentages for the current tape.
*/
func DerivedTrailBounds(spreadBps, volatilityRatio float64) (minTrail, maxTrail float64) {
	minTrail = DeriveStopFloor(spreadBps, volatilityRatio)
	maxTrail = minTrail * 4

	if volatilityRatio > 0 {
		expanded := minTrail + volatilityRatio*2

		if expanded > maxTrail {
			maxTrail = expanded
		}
	}

	if maxTrail < minTrail {
		maxTrail = minTrail
	}

	if math.IsNaN(minTrail) || math.IsInf(minTrail, 0) {
		minTrail = 0
	}

	if math.IsNaN(maxTrail) || math.IsInf(maxTrail, 0) {
		maxTrail = minTrail
	}

	return minTrail, maxTrail
}
