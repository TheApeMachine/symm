package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResonanceTarget(testingTB *testing.T) {
	Convey("Given a resonance target buffer", testingTB, func() {
		target := newResonanceTarget()

		Convey("When a symbol is first observed", func() {
			matured := target.Observe("ETH/USD", []float64{1, 2}, 100)

			Convey("Then nothing has matured yet (no future accrued)", func() {
				So(matured, ShouldBeEmpty)
			})
		})

		Convey("When a symbol is observed over a rising price path", func() {
			// Feed a steady up-drift; with the ceiling gamma (0.95) the horizon is
			// long, so samples mature only after many steps accrue.
			var lastMatured []maturedSample
			price := 100.0

			for step := 0; step < 400; step++ {
				price *= 1.001 // +0.1% per step
				matured := target.Observe("ETH/USD", []float64{float64(step)}, price)

				if len(matured) > 0 {
					lastMatured = matured
				}
			}

			Convey("Then early samples eventually mature with a positive label", func() {
				So(lastMatured, ShouldNotBeEmpty)
				// A monotonic up path yields a positive decayed forward return.
				So(lastMatured[0].label, ShouldBeGreaterThan, 0)
				// Labels are tanh-squashed into (-1, 1).
				So(lastMatured[0].label, ShouldBeLessThan, 1)
			})
		})

		Convey("When a symbol falls steadily", func() {
			var lastMatured []maturedSample
			price := 100.0

			for step := 0; step < 400; step++ {
				price *= 0.999
				matured := target.Observe("DOWN/USD", []float64{float64(step)}, price)

				if len(matured) > 0 {
					lastMatured = matured
				}
			}

			Convey("Then matured labels are negative", func() {
				So(lastMatured, ShouldNotBeEmpty)
				So(lastMatured[0].label, ShouldBeLessThan, 0)
			})
		})
	})
}

func TestResonanceTargetAdaptiveHorizon(testingTB *testing.T) {
	Convey("Given the adaptive gamma", testingTB, func() {
		target := newResonanceTarget()

		Convey("When a token has not moved", func() {
			state := &symbolTargetState{}

			Convey("Then gamma is the ceiling (longest horizon)", func() {
				So(target.gammaFor(state), ShouldAlmostEqual, target.gammaMax)
			})
		})

		Convey("When a token moves a lot per sample", func() {
			quiet := &symbolTargetState{seen: true, lastMove: 0.0005}
			active := &symbolTargetState{seen: true, lastMove: 0.05}

			Convey("Then a more active token gets a tighter horizon (lower gamma)", func() {
				So(target.gammaFor(active), ShouldBeLessThan, target.gammaFor(quiet))
				So(target.gammaFor(active), ShouldBeGreaterThanOrEqualTo, target.gammaMin)
			})
		})
	})
}
