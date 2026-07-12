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
		outcome := finiteResonanceOutcome()

		Convey("When the outcome is checked", func() {
			ok := outcome.IsFinite()

			Convey("Then it is usable causal evidence", func() {
				So(ok, ShouldBeTrue)
			})
		})
	})

	Convey("Given a resonance outcome with a non-finite latent value", t, func() {
		outcome := finiteResonanceOutcome()
		outcome.Latent[1] = math.NaN()

		Convey("When the outcome is checked", func() {
			ok := outcome.IsFinite()

			Convey("Then it is rejected before causal measurement", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given a resonance outcome with a non-finite scalar", t, func() {
		outcome := finiteResonanceOutcome()
		outcome.Energy = math.Inf(1)

		Convey("When the outcome is checked", func() {
			ok := outcome.IsFinite()

			Convey("Then it is rejected before causal measurement", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given a resonance outcome with the wrong latent width", t, func() {
		outcome := finiteResonanceOutcome()
		outcome.Latent = []float64{0.1, 0.2}

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
			observables, ready := resonance.normalize(
				[]float64{10, -2, 5, 0.5, 1000000},
				time.Unix(1, 0),
			)

			Convey("Then resonance reports a defined zero-deviation row from the first reading", func() {
				So(ready, ShouldBeTrue)
				So(observables, ShouldHaveLength, resonanceObservables)

				for _, value := range observables {
					So(value, ShouldAlmostEqual, 0, 0.000001)
				}
			})
		})

		Convey("When equal values arrive after baselines are seeded", func() {
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

func TestResonanceUpdate(t *testing.T) {
	Convey("Given a finite manifold observation", t, func() {
		thesis := strategy.NewThesis()
		resonance := NewResonance("BTC/USD", thesis, nil)
		thesis.AddEvidence("BTC/USD", "manifold", causalState(1, 100))

		Convey("When resonance settles and learns the observation online", func() {
			resonance.Update()
			snapshot, ok := thesis.Evidence("BTC/USD", "resonance")

			Convey("Then it emits a typed predictive-coding measurement", func() {
				So(ok, ShouldBeTrue)
				outcome := snapshot.(ResonanceOutcome)
				So(outcome.Source, ShouldEqual, "resonance")
				So(outcome.Symbol, ShouldEqual, "BTC/USD")
				So(outcome.Samples, ShouldEqual, uint64(1))
				So(outcome.IsFinite(), ShouldBeTrue)
			})
		})
	})
}

func BenchmarkResonanceUpdate(b *testing.B) {
	thesis := strategy.NewThesis()
	resonance := NewResonance("BTC/USD", thesis, nil)

	for index := 1; b.Loop(); index++ {
		state := causalState(uint64(index), 100+float64(index%19)/100)
		state.PressureGradNorm = float64(index%7) / 10
		state.Divergence = float64(index%11) / 100
		thesis.AddEvidence("BTC/USD", "manifold", state)
		resonance.Update()
	}
}

func finiteResonanceOutcome() ResonanceOutcome {
	return ResonanceOutcome{
		Source:      "resonance",
		Symbol:      "BTC/USD",
		At:          time.Unix(1, 0),
		Samples:     1,
		Observables: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		Latent:      []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		Energy:      0.6,
		Surprise:    0.7,
	}
}
