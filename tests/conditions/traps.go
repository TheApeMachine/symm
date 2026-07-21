package conditions

import (
	"iter"
	"math"
	"time"

	"github.com/theapemachine/symm/tests"
)

/*
SpoofedPump is the adversarial twin of the honest pump: the same accelerating
tape and buy-side aggression, but quotes reload on each mark-up step and then
drain away while the level holds, so the withdrawal is an observable cancel at
a stable touch instead of being hidden inside a reprice. Honest ignition looks
identical on the tape; only the liquidity behind it differs.
*/
func SpoofedPump() iter.Seq[tests.Frame] {
	const horizon = 40
	prices := make([]float64, horizon)
	quantities := make([]float64, horizon)
	spreads := make([]float64, horizon)
	depths := make([]float64, horizon)
	sides := make([]string, horizon)
	bids := make([][]float64, horizon)
	asks := make([][]float64, horizon)
	stamps := make([]time.Time, horizon)
	price := 0.5667

	for index := range horizon {
		if index%4 == 0 {
			price *= 1.18
		}

		withdrawal := 1 - float64(index%4)/4
		prices[index] = price
		quantities[index] = 10 + float64(index)
		spreads[index] = 0.0002
		depths[index] = 500 * withdrawal
		stamps[index] = trapStamp(index)
		bids[index] = []float64{240 * withdrawal, 80 * withdrawal}
		asks[index] = []float64{60, 20}
		sides[index] = "buy"

		if index%4 == 3 {
			sides[index] = "sell"
		}
	}

	return trapFrames(prices, quantities, sides, spreads, depths, bids, asks, stamps)
}

/*
Vacuum drains visible liquidity from both sides of a price that goes nowhere:
touch depth bleeds toward a sliver and the spread widens as quotes step back,
while only a trickle of two-sided volume prints. It is the liquidity-vacuum
trap — nothing about the tape invites a position, and the book could not
absorb one.
*/
func Vacuum() iter.Seq[tests.Frame] {
	const horizon = 48
	prices := make([]float64, horizon)
	quantities := make([]float64, horizon)
	spreads := make([]float64, horizon)
	depths := make([]float64, horizon)
	sides := make([]string, horizon)
	bids := make([][]float64, horizon)
	asks := make([][]float64, horizon)
	stamps := make([]time.Time, horizon)

	for index := range horizon {
		drain := 1 - float64(index)/float64(horizon)
		prices[index] = 0.5667
		quantities[index] = 1
		spreads[index] = 0.0002 + 0.002*float64(index)/float64(horizon)
		depths[index] = math.Max(500*drain, 5)
		stamps[index] = trapStamp(index)
		bids[index] = []float64{240 * drain, 80 * drain}
		asks[index] = []float64{240 * drain, 80 * drain}
		sides[index] = "buy"

		if index%2 == 1 {
			sides[index] = "sell"
		}
	}

	return trapFrames(prices, quantities, sides, spreads, depths, bids, asks, stamps)
}

/*
Coil contracts a two-sided oscillation around a stable level on steady depth:
each swing is smaller than the last while the book stays fully populated, the
classic compression that precedes an ignition. It exists so compression is
provable as typed evidence rather than assumed from a pump's prologue.
*/
func Coil() iter.Seq[tests.Frame] {
	const horizon = 64
	prices := make([]float64, horizon)
	quantities := make([]float64, horizon)
	spreads := make([]float64, horizon)
	depths := make([]float64, horizon)
	sides := make([]string, horizon)
	bids := make([][]float64, horizon)
	asks := make([][]float64, horizon)
	stamps := make([]time.Time, horizon)

	for index := range horizon {
		phase := float64(index) / 4 * math.Pi
		amplitude := 0.04 * (1 - float64(index)/float64(horizon))
		swing := amplitude * math.Sin(phase)
		prices[index] = 0.5667 * (1 + swing)
		quantities[index] = 10
		spreads[index] = 0.0002
		depths[index] = 500
		stamps[index] = trapStamp(index)
		bids[index] = []float64{240, 80}
		asks[index] = []float64{240, 80}
		sides[index] = "buy"

		if swing < 0 {
			sides[index] = "sell"
		}
	}

	return trapFrames(prices, quantities, sides, spreads, depths, bids, asks, stamps)
}

/*
trapFrames interleaves the checksum-valid L3 population with the public
trade/book/ticker path so every trap drives the manifold and the touch-honesty
signals from one coherent event, exactly like the production feed.
*/
func trapFrames(
	prices []float64,
	quantities []float64,
	sides []string,
	spreads []float64,
	depths []float64,
	bids [][]float64,
	asks [][]float64,
	stamps []time.Time,
) iter.Seq[tests.Frame] {
	level3 := Level3Path(prices, bids, asks, stamps)
	market := MarketPathWithSides(prices, quantities, sides, spreads, depths)

	return tests.RoundRobin(level3.Frames(), market.Frames())
}

/*
trapStamp aligns trap tapes on the shared synthetic session start.
*/
func trapStamp(index int) time.Time {
	return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).
		Add(time.Duration(index) * time.Second)
}
