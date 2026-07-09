package logic

import (
	"math"
	"testing"
	"time"

	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/symm/strategy"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResonanceOutcome(testingTB *testing.T) {
	Convey("Given a finite resonance outcome", testingTB, func() {
		outcome := ResonanceOutcome{
			Latent:         []float64{0.1, 0.2, 0.3, 0.4, 0.5},
			Energy:         0.6,
			Surprise:       0.7,
			ReturnForecast: 0.8,
		}

		Convey("When the outcome is checked", func() {
			ok := outcome.IsFinite()

			Convey("Then it is usable causal evidence", func() {
				So(ok, ShouldBeTrue)
			})
		})
	})

	Convey("Given a resonance outcome with a non-finite latent value", testingTB, func() {
		outcome := ResonanceOutcome{
			Latent:         []float64{0.1, math.NaN(), 0.3, 0.4, 0.5},
			Energy:         0.6,
			Surprise:       0.7,
			ReturnForecast: 0.8,
		}

		Convey("When the outcome is checked", func() {
			ok := outcome.IsFinite()

			Convey("Then it is rejected before causal measurement", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given a resonance outcome with a non-finite scalar", testingTB, func() {
		outcome := ResonanceOutcome{
			Latent:         []float64{0.1, 0.2, 0.3, 0.4, 0.5},
			Energy:         math.Inf(1),
			Surprise:       0.7,
			ReturnForecast: 0.8,
		}

		Convey("When the outcome is checked", func() {
			ok := outcome.IsFinite()

			Convey("Then it is rejected before causal measurement", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given a resonance outcome with the wrong latent width", testingTB, func() {
		outcome := ResonanceOutcome{
			Latent:         []float64{0.1, 0.2},
			Energy:         0.6,
			Surprise:       0.7,
			ReturnForecast: 0.8,
		}

		Convey("When the outcome is checked", func() {
			ok := outcome.IsFinite()

			Convey("Then it is rejected before causal measurement", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestResonanceNormalize(testingTB *testing.T) {
	Convey("Given resonance observable baselines", testingTB, func() {
		resonance := &Resonance{
			baselines: map[string]*adaptive.TimeElastic{},
		}

		Convey("When the first non-zero observable row arrives", func() {
			_, ready := resonance.normalize(
				[]float64{10, -2, 5, 0.5, 1000000},
				time.Unix(1, 0),
			)

			Convey("Then resonance waits for observed baselines", func() {
				So(ready, ShouldBeFalse)
			})
		})

		Convey("When equal values arrive after baselines are ready", func() {
			_, _ = resonance.normalize(
				[]float64{10, -2, 5, 0.5, 1000000},
				time.Unix(1, 0),
			)
			observables, ready := resonance.normalize(
				[]float64{10, -2, 5, 0.5, 1000000},
				time.Unix(2, 0),
			)

			Convey("Then the normalized row is centered on zero deviation", func() {
				So(ready, ShouldBeTrue)
				So(observables, ShouldHaveLength, resonanceObservables)

				for _, value := range observables {
					So(value, ShouldAlmostEqual, 0, 0.000001)
				}
			})
		})

		Convey("When a materially larger value arrives", func() {
			_, _ = resonance.normalize(
				[]float64{10, 2, 5, 0.5, 1000000},
				time.Unix(1, 0),
			)
			observables, ready := resonance.normalize(
				[]float64{20, 2, 5, 0.5, 1000000},
				time.Unix(2, 0),
			)

			Convey("Then the changed observable is positive deviation", func() {
				So(ready, ShouldBeTrue)
				So(observables[0], ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestResonancePrice(testingTB *testing.T) {
	Convey("Given resonance price evidence", testingTB, func() {
		thesis := strategy.NewThesis()
		at := time.Unix(10, 0)
		thesis.AddEvidence("price", 100.0)
		thesis.AddEvidence("price_at", at)
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			price, priceAt, ok := resonance.Price()

			Convey("Then the usable price and event time are returned", func() {
				So(ok, ShouldBeTrue)
				So(price, ShouldEqual, 100.0)
				So(priceAt, ShouldEqual, at)
			})
		})
	})

	Convey("Given resonance evidence without price", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price_at", time.Unix(10, 0))
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given resonance evidence with a non-float price", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", "100")
		thesis.AddEvidence("price_at", time.Unix(10, 0))
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given resonance evidence with a non-positive price", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", 0.0)
		thesis.AddEvidence("price_at", time.Unix(10, 0))
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given resonance evidence with a non-finite price", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", math.NaN())
		thesis.AddEvidence("price_at", time.Unix(10, 0))
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given resonance evidence without a price timestamp", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", 100.0)
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given resonance evidence with a non-time timestamp", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", 100.0)
		thesis.AddEvidence("price_at", "2026-07-09")
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given resonance evidence with a zero timestamp", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", 100.0)
		thesis.AddEvidence("price_at", time.Time{})
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})
}
