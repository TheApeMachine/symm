package vector

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
A class must not be silenced by a sibling's missing evidence.

An advisor declares several classes and mixes evidence from several venues and
clocks. Requiring every metric of every class meant one metric an instrument
never produces — a derivatives reading on a spot-only market, an arrival model
on an instrument that trades five times an hour — silenced the whole advisor
permanently. Measured over one run, that held five of seven advisors below 3%
of the instrument universe and two at zero.

What must still hold is the comparison itself: a class scored on present
evidence can never be ranked against one scored on absent evidence. Restricting
the distribution to classes whose OWN evidence is complete keeps that intact.
*/
func TestPartialClassReadiness(t *testing.T) {
	Convey("Given classes drawn from different evidence sets", t, func() {
		classifier, err := NewClassifier(
			NewGroup("Building", "hawkes/branching", "pumpdump/rate"),
			NewGroup("Stalling", "pumpdump/rate", "pumpdump/spread"),
			NewGroup("Reversing", "pumpdump/spread", "cvd/flow"),
		)

		So(err, ShouldBeNil)

		partial := map[string]float64{
			"pumpdump/rate":   1.5,
			"pumpdump/spread": 0.2,
			"cvd/flow":        -0.7,
		}

		Convey("an observation missing one class's evidence still classifies", func() {
			So(classifier.Complete(partial), ShouldBeTrue)
		})

		Convey("it names which classes it could and could not score", func() {
			ready, unscored := classifier.ReadyClasses(partial)

			So(ready, ShouldResemble, []string{"Stalling", "Reversing"})
			So(unscored, ShouldResemble, []string{"Building"})
		})

		Convey("the unscored class takes no probability", func() {
			So(classifier.Observe(partial), ShouldBeTrue)

			reading := classifier.Read()

			So(reading.Ready, ShouldBeTrue)
			So(reading.Probabilities, ShouldContainKey, "Stalling")
			So(reading.Probabilities, ShouldContainKey, "Reversing")
			So(reading.Probabilities, ShouldNotContainKey, "Building")
		})

		Convey("the winner is a class that was actually scored", func() {
			So(classifier.Observe(partial), ShouldBeTrue)

			reading := classifier.Read()

			So(reading.WinnerLabel, ShouldBeIn, []string{"Stalling", "Reversing"})
		})
	})

	Convey("Given only one class with complete evidence", t, func() {
		classifier, err := NewClassifier(
			NewGroup("Building", "hawkes/branching"),
			NewGroup("Stalling", "pumpdump/rate"),
		)

		So(err, ShouldBeNil)

		lone := map[string]float64{"pumpdump/rate": 1.0}

		Convey("nothing is stated, because one class is not a comparison", func() {
			So(classifier.Complete(lone), ShouldBeFalse)
			So(classifier.Observe(lone), ShouldBeFalse)
		})
	})
}
