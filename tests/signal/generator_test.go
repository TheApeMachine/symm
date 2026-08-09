package signal

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestGeneratorNewGenerator(t *testing.T) {
	Convey("Given a symbol and a valid starting price", t, func() {
		generator := NewGenerator("SIM1/USD", 100, 0.01, 2, 42)

		Convey("It should initialize in the stationary baseline", func() {
			So(generator.symbol, ShouldEqual, "SIM1/USD")
			So(generator.midPrice, ShouldEqual, 100.0)
			So(generator.currentState, ShouldEqual, testtypes.Baseline)
		})
	})
}

func TestGeneratorSetState(t *testing.T) {
	Convey("Given a baseline generator", t, func() {
		generator := NewGenerator("SIM1/USD", 100, 0.01, 2, 42)
		generator.SetState(testtypes.FastPump)

		Convey("The observable precursor should begin before ignition", func() {
			So(generator.targetState, ShouldEqual, testtypes.FastPump)
			So(generator.PrecursorPending(), ShouldBeTrue)
			So(generator.IgnitionArmed(), ShouldBeFalse)
		})

		Convey("An undeclared latent state should fail loudly", func() {
			So(func() {
				generator.SetState(testtypes.MarketState(10_000))
			}, ShouldPanic)
		})

		Convey("Ambiguous transition momentum should fail loudly", func() {
			So(func() {
				generator.SetState(testtypes.FastPump, 1, 2)
			}, ShouldPanic)
		})
	})
}

func TestGeneratorStep(t *testing.T) {
	Convey("Given a stationary baseline", t, func() {
		generator := NewGenerator("SIM1/USD", 100, 0.01, 2, 42)
		buyTrades := 0
		sellTrades := 0

		for range 128 {
			sample := generator.Step()
			So(sample.Bid, ShouldBeGreaterThan, 0)
			So(sample.Ask, ShouldBeGreaterThan, sample.Bid)
			So(sample.StepVolume, ShouldBeGreaterThan, 0)

			if sample.AggressorSide == "buy" {
				buyTrades++
			}

			if sample.AggressorSide == "sell" {
				sellTrades++
			}
		}

		Convey("Price should stay flat while flow remains two-sided", func() {
			So(generator.trendPrice, ShouldEqual, 100.0)
			So(generator.midPrice, ShouldEqual, 100.0)
			So(buyTrades, ShouldBeGreaterThan, 0)
			So(sellTrades, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a FastPump transition", t, func() {
		generator := NewGenerator("SIM1/USD", 100, 0.01, 2, 42)
		profile := testtypes.DefaultProfiles[testtypes.FastPump]
		generator.SetState(testtypes.FastPump, testtypes.MomentumMap[testtypes.FastPump])

		for generator.PrecursorPending() {
			sample := generator.Step()
			So(sample.ChangePct, ShouldBeLessThan, profile.IgnitionMove*100)
			So(sample.AggressorSide, ShouldEqual, "buy")
		}

		Convey("The full discontinuity should remain armed for the next sample", func() {
			So(generator.IgnitionArmed(), ShouldBeTrue)
			ignition := generator.Step()
			So(ignition.ChangePct,
				ShouldBeGreaterThanOrEqualTo, profile.IgnitionMove*100)
			So(generator.IgnitionArmed(), ShouldBeFalse)
		})
	})

	Convey("Given opposite loadings on one shared factor", t, func() {
		positiveSymbol := testtypes.NewSymbol("POS/USD", 100, 91)
		negativeSymbol := testtypes.NewSymbol("NEG/USD", 100, 92)
		positiveSymbol.FactorLoading = 1
		negativeSymbol.FactorLoading = -1
		positive := NewGeneratorFromSymbol(positiveSymbol)
		negative := NewGeneratorFromSymbol(negativeSymbol)
		positive.SetState(testtypes.SidewaysChop)
		negative.SetState(testtypes.SidewaysChop)

		for positive.PrecursorPending() || negative.PrecursorPending() {
			positive.Step(0)
			negative.Step(0)
		}

		positive.Step(1)
		negative.Step(1)

		Convey("Their observation shocks should move in opposite directions", func() {
			So(positive.midPrice, ShouldBeGreaterThan, positive.trendPrice)
			So(negative.midPrice, ShouldBeLessThan, negative.trendPrice)
		})
	})

	Convey("Given an explicitly tiered symbol", t, func() {
		symbol := testtypes.NewSymbol("DEPTH/USD", 100, 93)
		symbol.BookDepthLevels = 3
		symbol.DepthQuantityScale = 1.5
		sample := NewGeneratorFromSymbol(symbol).Step()

		Convey("Every generated tier should be finite and ordered", func() {
			So(sample.Bids, ShouldHaveLength, 3)
			So(sample.Asks, ShouldHaveLength, 3)
			So(sample.Bids[1].Price, ShouldBeLessThan, sample.Bids[0].Price)
			So(sample.Asks[1].Price, ShouldBeGreaterThan, sample.Asks[0].Price)
			So(sample.Bids[1].Quantity,
				ShouldBeGreaterThan, sample.Bids[0].Quantity)
		})
	})

	Convey("Given a fractional tick whose binary representation is inexact", t, func() {
		symbol := testtypes.NewSymbol("FRACTION/USD", 3.1415, 94)
		sample := NewGeneratorFromSymbol(symbol).Step()

		Convey("The canonical quote should equal the first rendered depth tier", func() {
			So(sample.Bids[0].Price, ShouldEqual, sample.Bid)
			So(sample.Asks[0].Price, ShouldEqual, sample.Ask)
		})
	})

	Convey("Given random-walk and false-breakout controls", t, func() {
		randomWalk := NewGenerator("RANDOM/USD", 100, 0.01, 2, 95)
		randomWalk.SetState(testtypes.RandomWalk)

		for randomWalk.PrecursorPending() {
			randomWalk.Step()
		}

		walkStart := randomWalk.trendPrice

		for range 16 {
			randomWalk.Step()
		}

		falseBreak := NewGenerator("FALSE/USD", 100, 0.01, 2, 96)
		falseBreak.SetState(testtypes.FalseBreakout)

		for falseBreak.PrecursorPending() {
			falseBreak.Step()
		}

		falseBreak.Step()
		breakoutDistance := math.Abs(falseBreak.trendPrice - falseBreak.openPrice)

		for range 16 {
			falseBreak.Step()
		}

		Convey("The null should diffuse and the false break should revert", func() {
			So(randomWalk.trendPrice, ShouldNotEqual, walkStart)
			So(math.Abs(falseBreak.trendPrice-falseBreak.openPrice),
				ShouldBeLessThan, breakoutDistance)
		})
	})
}

func BenchmarkGeneratorStep(b *testing.B) {
	generator := NewGenerator("SIM1/USD", 100, 0.01, 2, 42)

	for b.Loop() {
		_ = generator.Step()
	}
}
