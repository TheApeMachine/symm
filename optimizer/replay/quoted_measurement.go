package replay

import (
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
QuotedMeasurement adds bid/ask and walkable L2 depth so replay entries pass the same
broker preflight gates as live paper trading.
*/
func QuotedMeasurement(measurement types.Measurement) types.Measurement {
	if measurement.Last > 0 && measurement.Bid <= 0 && measurement.Ask <= 0 {
		halfSpread := measurement.Last * 0.00005
		measurement.Bid = measurement.Last - halfSpread
		measurement.Ask = measurement.Last + halfSpread
	}

	if measurement.HasBookDepth() {
		return measurement
	}

	if measurement.Ask > 0 && measurement.Bid > 0 {
		measurement.BookAsks = []types.BookLevel{{Price: measurement.Ask, Qty: 1_000}}
		measurement.BookBids = []types.BookLevel{{Price: measurement.Bid, Qty: 1_000}}
	}

	return measurement
}
