package replay

import (
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
QuotedMeasurement attaches bid/ask when the tape row carries an honest quote:
explicit bid/ask or Last plus SpreadBPS. It never narrows spread or fabricates L2 depth.
*/
func QuotedMeasurement(measurement types.Measurement) types.Measurement {
	return broker.ApplyDerivedBidAsk(measurement)
}
