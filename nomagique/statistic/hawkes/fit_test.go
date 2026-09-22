package hawkes_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHawkesFit(t *testing.T) {
	Convey("Given a path drawn from a known process", t, func() {
		baseline := []float64{0.5, 0.3}
		excitation := []float64{0.9, 0.25, 0.35, 0.8}
		decay := 1.5
		path := simulate(2, baseline, excitation, decay, 6000, 20260922)

		So(len(path.times), ShouldBeGreaterThan, 4000)

		Convey("When the estimation loop is run to convergence", func() {
			settled := fit(t, path, 4000, 1)

			Convey("Then it recovers the parameters the path was drawn from", func() {
				// Nothing about the truth is given to the loop: the search
				// region is derived from the window, the start is the
				// process with no excitation at all, and every step is
				// taken on the measured likelihood. Landing near the
				// generating parameters is the only evidence that the
				// likelihood, its gradient, the bounded map and the stepper
				// are all correct together.
				So(math.Abs(settled.decay-decay), ShouldBeLessThan, 0.25*decay)

				for index := range baseline {
					So(math.Abs(settled.baseline[index]-baseline[index]), ShouldBeLessThan, 0.30*baseline[index])
				}

				for index := range excitation {
					So(math.Abs(settled.excitation[index]-excitation[index]), ShouldBeLessThan, 0.30*excitation[index])
				}
			})

			Convey("Then it explains the path better than the process it started from", func() {
				derived := domainOf(t, path)
				start := mapCoordinates(t, derived.seed, derived.lower, derived.upper, 2)
				opening, _, openingOK := likelihood(t, path, start.baseline, start.excitation, start.decay)
				final, _, finalOK := likelihood(t, path, settled.baseline, settled.excitation, settled.decay)

				So(openingOK, ShouldBeTrue)
				So(finalOK, ShouldBeTrue)
				So(final, ShouldBeGreaterThan, opening)
			})

			Convey("Then the process it found is a stable one", func() {
				branching := branchingOf(t, settled.excitation, settled.decay, 2)
				So(radiusOf(t, branching, 2), ShouldBeLessThan, 1)
			})

			Convey("Then restricting the components from exciting each other explains it less well", func() {
				// The restricted process is the same model with the
				// cross-excitation held at zero, so it cannot fit better.
				// That the unrestricted fit does better is what makes the
				// difference between them evidence of coupling rather than
				// of one search having gone further than the other.
				restricted := fit(t, path, 4000, 0)
				So(restricted.excitation[1], ShouldBeLessThan, settled.excitation[1])
				So(restricted.excitation[2], ShouldBeLessThan, settled.excitation[2])

				free, _, freeOK := likelihood(t, path, settled.baseline, settled.excitation, settled.decay)
				held, _, heldOK := likelihood(t, path, restricted.baseline, restricted.excitation, restricted.decay)

				So(freeOK, ShouldBeTrue)
				So(heldOK, ShouldBeTrue)
				So(free, ShouldBeGreaterThan, held)
			})
		})
	})
}
