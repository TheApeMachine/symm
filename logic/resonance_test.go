package logic

import (
	"math"
	"testing"
	"time"

	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/symm/strategy"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResonanceOutcome(t *testing.T) {
	Convey("Given a finite resonance outcome", t, func() {
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

	Convey("Given a resonance outcome with a non-finite latent value", t, func() {
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

	Convey("Given a resonance outcome with a non-finite scalar", t, func() {
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

	Convey("Given a resonance outcome with the wrong latent width", t, func() {
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

func TestResonanceNormalize(t *testing.T) {
	Convey("Given resonance observable baselines", t, func() {
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

		Convey("When a stale event time arrives after the resonance frontier", func() {
			resonance.lastEventAt = time.Unix(2, 0)

			Convey("Then the event is rejected before baseline normalization", func() {
				So(resonance.eventStale(time.Unix(1, 0)), ShouldBeTrue)
			})
		})
	})
}

func TestResonancePrice(t *testing.T) {
	Convey("Given resonance price evidence", t, func() {
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

	Convey("Given resonance evidence without price", t, func() {
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

	Convey("Given resonance evidence with a non-float price", t, func() {
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

	Convey("Given resonance evidence with a non-positive price", t, func() {
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

	Convey("Given resonance evidence with a non-finite price", t, func() {
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

	Convey("Given resonance evidence without a price timestamp", t, func() {
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

	Convey("Given resonance evidence with a non-time timestamp", t, func() {
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

	Convey("Given resonance evidence with a zero timestamp", t, func() {
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
