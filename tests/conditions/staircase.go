package conditions

import (
	"iter"
	"math"
	"time"

	"github.com/theapemachine/symm/tests"
)

/*
Staircase is the sustained multi-leg trend: repeated legs of steady gain
separated by pauses with shallow pullbacks, on an honest book that stays
populated throughout. Its per-epoch return is mostly noise — the opportunity
only exists at a deeper horizon, which is exactly the shape single-event
forecasting is blind to. legReturn is one leg's total gain; legs counts them;
legFrames and pauseFrames set the rhythm.
*/
func Staircase(
	legReturn float64,
	legs int,
	legFrames int,
	pauseFrames int,
) iter.Seq[tests.Frame] {
	horizon := legs * (legFrames + pauseFrames)
	prices := make([]float64, horizon)
	quantities := make([]float64, horizon)
	spreads := make([]float64, horizon)
	depths := make([]float64, horizon)
	sides := make([]string, horizon)
	bids := make([][]float64, horizon)
	asks := make([][]float64, horizon)
	stamps := make([]time.Time, horizon)
	price := 0.0188
	step := math.Pow(1+legReturn, 1/float64(legFrames))

	for index := range horizon {
		cycle := index % (legFrames + pauseFrames)
		rising := cycle < legFrames

		if rising {
			price *= step
		} else if cycle%3 == 1 {
			price *= 1 - legReturn/float64(legFrames)/2
		}

		prices[index] = price
		quantities[index] = 8 + 4*float64(index%7)
		spreads[index] = 0.0002
		depths[index] = 500
		stamps[index] = trapStamp(index)
		bids[index] = []float64{240, 80}
		asks[index] = []float64{60, 20}
		sides[index] = "buy"

		if !rising && cycle%2 == 1 {
			sides[index] = "sell"
		}
	}

	return trapFrames(prices, quantities, sides, spreads, depths, bids, asks, stamps)
}
