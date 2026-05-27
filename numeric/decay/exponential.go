package decay

import (
	"math"
	"time"

	"github.com/theapemachine/symm/numeric/timeline"
)

/*
ExpNeg returns exp(-beta * age).
*/
func ExpNeg(beta, age float64) float64 {
	return math.Exp(-beta * age)
}

/*
LogPositive returns ln(value), or ln(1e-9) when value is non-positive.
*/
func LogPositive(value float64) float64 {
	if value <= 0 {
		return math.Log(1e-9)
	}

	return math.Log(value)
}

/*
KernelSupport sums 1 - exp(-beta * remaining) over events before horizon.
*/
func KernelSupport(events timeline.Timeline, horizon time.Time, beta float64) float64 {
	var support float64

	for _, eventTime := range events.Times() {
		remaining := horizon.Sub(eventTime).Seconds()

		if remaining > 0 {
			support += 1 - ExpNeg(beta, remaining)
		}
	}

	return support
}

/*
IntensityAt evaluates mu plus alphaOnBuy * sum(buy impulses) plus alphaOnSell * sum(sell impulses).
Both timelines stay in buy/sell order for buy-side and sell-side Hawkes intensities.
*/
func IntensityAt(
	buyEvents, sellEvents timeline.Timeline,
	at time.Time,
	mu, alphaOnBuy, alphaOnSell, beta float64,
) float64 {
	lambda := mu
	lambda += impulseSum(buyEvents.Times(), at, alphaOnBuy, beta)
	lambda += impulseSum(sellEvents.Times(), at, alphaOnSell, beta)

	return lambda
}

func impulseSum(
	eventTimes []time.Time,
	at time.Time,
	alpha, beta float64,
) float64 {
	var sum float64

	for _, eventTime := range eventTimes {
		if !eventTime.Before(at) {
			continue
		}

		age := at.Sub(eventTime).Seconds()

		if age >= 0 {
			sum += alpha * ExpNeg(beta, age)
		}
	}

	return sum
}
