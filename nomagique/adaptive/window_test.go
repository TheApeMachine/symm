package adaptive

import (
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestAdwinWindow pins the two properties an ADWIN window must have, and which a
per-sample outlier test cannot: it holds its horizon open on a stationary
stream, and it contracts on a sustained level shift.

The first is the one worth guarding. A rule that compares each new observation
against the running mean contracts on roughly a third of stationary samples,
because that is how often one draw sits beyond one standard deviation. Such a
window reports a horizon of a handful of samples no matter how long the stream
has been quiet, and every estimator built on it loses its support.
*/
func TestAdwinWindow(t *testing.T) {
	Convey("Given an ADWIN window on a stationary stream", t, func() {
		rng := rand.New(rand.NewSource(11))
		window := &Window{Type: ADWIN}

		for range 500 {
			window.Step(1.0 + rng.NormFloat64()*0.1)
		}

		Convey("it accumulates its horizon rather than contracting on noise", func() {
			So(window.Capacity(), ShouldBeGreaterThan, 400)
		})

		Convey("it reports no shedding while the stream is stationary", func() {
			So(window.ShedRatio(), ShouldEqual, 1.0)
		})

		Convey("a sustained level shift contracts the horizon", func() {
			before := window.Capacity()

			for range 500 {
				window.Step(5.0 + rng.NormFloat64()*0.1)
			}

			So(window.Capacity(), ShouldBeLessThan, before)
		})
	})
}

/*
TestWelfordShed proves shedding preserves the estimate while collapsing the
support behind it: the level and the spread are unchanged at the moment of the
call, so nothing is discarded discontinuously, but subsequent samples carry
proportionally more weight.
*/
func TestWelfordShed(t *testing.T) {
	Convey("Given accumulated moments", t, func() {
		var welford WelfordEngine

		for index := range 100 {
			welford.Update(float64(index%10) + 1)
		}

		meanBefore := welford.Mean()
		dispersionBefore := welford.Dispersion()

		welford.Shed(0.5)

		Convey("the mean and dispersion survive the shed", func() {
			So(welford.Mean(), ShouldAlmostEqual, meanBefore, 1e-9)
			So(welford.Dispersion(), ShouldAlmostEqual, dispersionBefore, 1e-9)
		})

		Convey("the support behind them halves", func() {
			So(welford.Count(), ShouldEqual, 50.0)
		})

		Convey("a new sample moves the mean further than it would have", func() {
			shedMean := welford.Mean()
			welford.Update(100)
			shedMove := welford.Mean() - shedMean

			var unshed WelfordEngine
			for index := range 100 {
				unshed.Update(float64(index%10) + 1)
			}
			unshedMean := unshed.Mean()
			unshed.Update(100)

			So(shedMove, ShouldBeGreaterThan, unshed.Mean()-unshedMean)
		})
	})
}
