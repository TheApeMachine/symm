package strategy

import "github.com/theapemachine/symm/types"

/*
Incumbent is one open holding's continuation value for rotate comparisons.
HoldUtility is the forward SNR of staying; ExitCost is the one-way friction
paid to free the slot; Notional sizes the challenger from capital being freed.
*/
type Incumbent struct {
	Symbol      string
	HoldUtility float64
	ExitCost    float64
	Notional    float64
	Displaced   bool
}

/*
holdUtility is the executable value of keeping an open name: expected return
net of uncertainty. Entry fees are sunk and do not belong in the keep score.
*/
func (planner *Planner) holdUtility(forecast types.Forecasts) float64 {
	return forecast.ExpectedReturn - forecast.Uncertainty
}

/*
exitCost is the one-way taker friction paid to liquidate an incumbent when a
challenger displaces it.
*/
func (planner *Planner) exitCost(forecast types.Forecasts, fee float64) float64 {
	return fee + forecast.ExpectedSpread/2
}

/*
rotateSurplus is the net utility of replacing an incumbent with a challenger.
Positive means rotate; non-positive means wait for the open thesis to mature.
*/
func rotateSurplus(challenger, hold, exitCost float64) float64 {
	return challenger - hold - exitCost
}

/*
weakest returns the displaceable incumbent with the lowest hold utility.
*/
func weakest(incumbents []Incumbent) (int, bool) {
	best := -1

	for index := range incumbents {
		if incumbents[index].Displaced {
			continue
		}

		if best < 0 || incumbents[index].HoldUtility < incumbents[best].HoldUtility {
			best = index
		}
	}

	return best, best >= 0
}
