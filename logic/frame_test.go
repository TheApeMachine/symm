package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestBatchContinuation(testingTB *testing.T) {
	Convey("Given per-symbol directional frames", testingTB, func() {
		Convey("When the forecast, momentum and causal read all point up", func() {
			batch := Batch{
				Resonance: []*ResonanceFrame{{Symbol: "ETH/USD", Forecast: 0.8}},
				Manifold:  []*ManifoldFrame{{Symbol: "ETH/USD", Momentum: 1.0}},
				Causal:    []*CausalFrame{{Symbol: "ETH/USD", Uplift: 0.6, Panic: 0.1}},
			}

			prob := batch.Continuation()["ETH/USD"]

			Convey("Then P(up) is well above 0.5", func() {
				So(prob, ShouldBeGreaterThan, 0.5)
			})
		})

		Convey("When the signals point down (negative forecast, high panic)", func() {
			batch := Batch{
				Resonance: []*ResonanceFrame{{Symbol: "ETH/USD", Forecast: -0.9}},
				Manifold:  []*ManifoldFrame{{Symbol: "ETH/USD", Momentum: -0.5}},
				Causal:    []*CausalFrame{{Symbol: "ETH/USD", Uplift: 0.0, Panic: 0.8}},
			}

			prob := batch.Continuation()["ETH/USD"]

			Convey("Then P(up) is well below 0.5", func() {
				So(prob, ShouldBeLessThan, 0.5)
			})
		})

		Convey("When there is no directional signal", func() {
			batch := Batch{
				Resonance: []*ResonanceFrame{{Symbol: "ETH/USD", Forecast: 0}},
			}

			prob := batch.Continuation()["ETH/USD"]

			Convey("Then P(up) is exactly 0.5 (no edge)", func() {
				So(prob, ShouldAlmostEqual, 0.5)
			})
		})
	})
}

func TestBatchMomentum(testingTB *testing.T) {
	Convey("Given a batch of per-symbol field frames", testingTB, func() {
		Convey("When a symbol has one manifold and one resonance frame", func() {
			batch := Batch{
				Manifold: []*ManifoldFrame{{
					Symbol:   "ETH/USD",
					Momentum: 0.6,
					Reading:  ManifoldReading{GuidanceSpeed: 0.2},
				}},
				Resonance: []*ResonanceFrame{{
					Symbol: "ETH/USD",
					Energy: 0.3,
					Flow:   0.1,
				}},
			}

			scores := batch.Momentum()

			Convey("Then the score blends manifold drive and resonance energy", func() {
				// (|0.6| + |0.2|) + (|0.3| + |0.1|) = 1.2
				So(scores["ETH/USD"], ShouldAlmostEqual, 1.2)
			})
		})

		Convey("When a symbol has duplicate manifold frames in one cycle", func() {
			batch := Batch{
				Manifold: []*ManifoldFrame{
					{Symbol: "ETH/USD", Momentum: 0.6},
					{Symbol: "ETH/USD", Momentum: 0.9},
				},
			}

			scores := batch.Momentum()

			Convey("Then the max is taken, not the sum, so it does not double-count", func() {
				So(scores["ETH/USD"], ShouldAlmostEqual, 0.9)
			})
		})

		Convey("When a sparse fallback frame accompanies a full frame", func() {
			// A price_zero source emits a minimal frame; another source emits the
			// full field frame in the same cycle. The fallback must not shadow the
			// stronger full reading.
			batch := Batch{
				Manifold: []*ManifoldFrame{
					{Symbol: "ETH/USD", Momentum: 0.1}, // sparse fallback
					{Symbol: "ETH/USD", Momentum: 0.8, Reading: ManifoldReading{GuidanceSpeed: 0.3}}, // full
				},
			}

			scores := batch.Momentum()

			Convey("Then the full frame's stronger score wins", func() {
				So(scores["ETH/USD"], ShouldAlmostEqual, 1.1)
			})
		})

		Convey("When only a sparse fallback frame exists for a held symbol", func() {
			// This is the gap the minimal frame fills: without it the symbol would
			// be absent from the momentum map entirely.
			batch := Batch{
				Manifold: []*ManifoldFrame{{Symbol: "ETH/USD", Momentum: 0.4}},
			}

			scores := batch.Momentum()

			Convey("Then the symbol still has a live momentum score", func() {
				_, ok := scores["ETH/USD"]
				So(ok, ShouldBeTrue)
				So(scores["ETH/USD"], ShouldAlmostEqual, 0.4)
			})
		})
	})
}

func TestBoundaryFrameMinimalManifold(testingTB *testing.T) {
	Convey("Given a boundary frame with clamps carrying momentum and pressure", testingTB, func() {
		frame := boundaryFrame{
			symbol: "ETH/USD",
			clamps: []fieldClamp{
				{momX: 0.5, pressure: 0.2},
				{momX: 0.3, pressure: 0.1},
			},
		}

		manifold := frame.minimalManifold()

		Convey("Then the minimal frame carries the net momentum and pressure", func() {
			So(manifold, ShouldNotBeNil)
			So(manifold.Symbol, ShouldEqual, "ETH/USD")
			So(manifold.Source, ShouldEqual, types.SourceManifold)
			So(manifold.Momentum, ShouldAlmostEqual, 0.8)
			So(manifold.Pressure, ShouldAlmostEqual, 0.3)
		})

		Convey("And it contributes to Batch.Momentum", func() {
			scores := Batch{Manifold: []*ManifoldFrame{manifold}}.Momentum()
			So(scores["ETH/USD"], ShouldAlmostEqual, 0.8)
		})
	})
}
