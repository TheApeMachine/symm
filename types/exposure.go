package types

/*
Exposure is the immutable portfolio state strategy needs for continuation
utility. It carries the originating Thesis without exposing broker.Position or
its transport-owned mutable state.
*/
type Exposure struct {
	Thesis    *Thesis
	Quantity  float64
	Mark      float64
	Notional  float64
	ReturnPct float64
}
