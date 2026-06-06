package market

import (
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
Signal is a mechanism to structure raw market data into
measurements, which are labeled as semantic categories.
*/
type Signal interface {
	Measure(perspectives.Feedback) types.Measurement
}
