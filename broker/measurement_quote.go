package broker

import "github.com/theapemachine/symm/market/perspectives/types"

/*
DeriveBidAsk returns bid and ask from explicit tape fields or from Last plus SpreadBPS.
It never invents a spread when SpreadBPS is zero and bid/ask are absent.
*/
func DeriveBidAsk(measurement types.Measurement) (bid, ask float64, ok bool) {
	if measurement.Bid > 0 && measurement.Ask > 0 && measurement.Ask >= measurement.Bid {
		return measurement.Bid, measurement.Ask, true
	}

	if measurement.Last <= 0 || measurement.SpreadBPS <= 0 {
		return 0, 0, false
	}

	halfSpread := measurement.SpreadBPS / 20_000 * measurement.Last

	return measurement.Last - halfSpread, measurement.Last + halfSpread, true
}

/*
ApplyDerivedBidAsk copies bid/ask onto a measurement when they can be derived honestly.
Book depth is never synthesized; only fields present on the tape are preserved.
*/
func ApplyDerivedBidAsk(measurement types.Measurement) types.Measurement {
	bid, ask, ok := DeriveBidAsk(measurement)

	if !ok {
		return measurement
	}

	if measurement.Bid <= 0 {
		measurement.Bid = bid
	}

	if measurement.Ask <= 0 {
		measurement.Ask = ask
	}

	return measurement
}
