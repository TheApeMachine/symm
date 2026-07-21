package conditions

import (
	"iter"
	"time"

	"github.com/theapemachine/symm/tests"
)

/*
TapeCalm is a multi-leg calm baseline for contrast against opportunity tapes.
*/
func TapeCalm() *tests.Market {
	return Calm(32)
}

/*
TapePumpDump is a multi-leg calibration, ignition, continuation, and rejection
tape whose ticker volume follows Kraken's cumulative-volume semantics.
*/
func TapePumpDump() *tests.Market {
	return PumpDump()
}

/*
TapeExhaustion withdraws book depth into mechanical exhaustion structure.
*/
func TapeExhaustion() *tests.Market {
	return Decay(32, 0, 0.9)
}

/*
TapeSectorLift is a co-moving herd cohort (sector breadth).
*/
func TapeSectorLift() *tests.Market {
	return Herd(32)
}

/*
TapeThinBook starves subject touch size inside a herd.
*/
func TapeThinBook() *tests.Market {
	return ThinHerd(32, 0.05)
}

/*
TapeNoise is unstructured multi-symbol motion.
*/
func TapeNoise() *tests.Market {
	return Noise(32)
}

/*
TapeAlpha is a peer-aligned subject path with distinctly greater return energy.
*/
func TapeAlpha() *tests.Market {
	return Alpha(32)
}

/*
TapeDivergence raises the subject while the peer cohort falls.
*/
func TapeDivergence() *tests.Market {
	return Divergence(32)
}

/*
TapeSlump lowers the entire cohort without manufacturing a standout leader.
*/
func TapeSlump() *tests.Market {
	return Slump(32)
}

/*
TapeStall establishes a leader and then stops it while peers keep moving.
*/
func TapeStall() *tests.Market {
	return Stall(48)
}

/*
TapeToxicChase is buy aggression without supportive book.
*/
func TapeToxicChase() *tests.Market {
	return Aggression(32, 0, 20)
}

/*
TapeLag is follower lag without a tradeable lead claim for the subject.
*/
func TapeLag() *tests.Market {
	return Lag(32, 4)
}

/*
TapeImbalance is a bid-loaded book for depthflow loaded-score proofs.
*/
func TapeImbalance() *tests.Market {
	return Imbalance(32, 0, 6, 0.15)
}

/*
TapeBalancedBook is a two-sided balanced book contrast for depthflow.
*/
func TapeBalancedBook() *tests.Market {
	return Imbalance(32, 0, 1, 1)
}

/*
TapeSpoofedPump is the honest pump's adversarial twin: identical tape
signature with quotes that reload on each step and drain while it holds.
*/
func TapeSpoofedPump() iter.Seq[tests.Frame] {
	return SpoofedPump()
}

/*
TapeVacuum bleeds both sides of the book to a sliver under a flat tape.
*/
func TapeVacuum() iter.Seq[tests.Frame] {
	return Vacuum()
}

/*
TapeCoil contracts a two-sided oscillation on steady depth toward ignition.
*/
func TapeCoil() iter.Seq[tests.Frame] {
	return Coil()
}

/*
TapeStaircase is the sustained multi-leg grind: fourteen ~6% legs separated by
pauses with shallow pullbacks, the ESPORTS-style day whose edge only exists
beyond the next event.
*/
func TapeStaircase() iter.Seq[tests.Frame] {
	return Staircase(0.06, 14, 8, 8)
}

/*
TapeTakeProfit raises the subject well beyond its armed survival band, then
introduces a shallow sincere fade while price remains near the peak. The fade
is long enough to expose forward weakening without crossing the locked floor.
*/
func TapeTakeProfit() iter.Seq[tests.Frame] {
	const (
		horizon       = 80
		liftFrames    = 48
		entryPrice    = 0.10035
		liftReturn    = 0.08
		fadeReturn    = 0.004
		touchSpread   = 0.0002
		touchDepth    = 1000.0
		tradeQuantity = 10.0
	)
	startedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	prices := make([]float64, horizon)
	quantities := make([]float64, horizon)
	spreads := make([]float64, horizon)
	depths := make([]float64, horizon)
	sides := make([]string, horizon)
	bids := make([][]float64, horizon)
	asks := make([][]float64, horizon)
	stamps := make([]time.Time, horizon)

	for index := range horizon {
		liftProgress := float64(min(index, liftFrames-1)) / float64(liftFrames-1)
		pathReturn := liftReturn * liftProgress
		sides[index] = "buy"

		if index >= liftFrames {
			fadeProgress := float64(index-liftFrames+1) / float64(horizon-liftFrames)
			pathReturn = liftReturn - fadeReturn*fadeProgress
			sides[index] = "sell"
		}

		if index%4 == 3 {
			switch sides[index] {
			case "buy":
				sides[index] = "sell"
			case "sell":
				sides[index] = "buy"
			}
		}

		prices[index] = entryPrice * (1 + pathReturn)
		quantities[index] = tradeQuantity
		spreads[index] = touchSpread
		depths[index] = touchDepth
		bids[index] = []float64{touchDepth, touchDepth / 3}
		asks[index] = []float64{touchDepth, touchDepth / 3}
		stamps[index] = startedAt.Add(time.Duration(index) * time.Second)
	}

	level3 := Level3Path(prices, bids, asks, stamps)
	market := MarketPathWithSides(prices, quantities, sides, spreads, depths)

	return tests.RoundRobin(level3.Frames(), market.Frames())
}
