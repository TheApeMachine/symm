package market

import "github.com/theapemachine/symm/logic"

/*
Signal is a mechanism to structure raw market data into
measurements, which are labeled as semantic categories.
*/
type Signal interface {
	Measure(Feedback) logic.Measurement
}
