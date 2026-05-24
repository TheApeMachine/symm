package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestCombinePerspectives(t *testing.T) {
	Convey("Given two strong perspectives and one weak", t, func() {
		combined, active := combinePerspectives([]PerspectiveScore{
			{Perspective: "microstructure", Score: 0.8, Sources: 2},
			{Perspective: "flow", Score: 0.5, Sources: 1},
			{Perspective: "sentiment", Score: 0.05, Sources: 1},
		})

		Convey("It should blend the top angles without summing everything", func() {
			So(active, ShouldEqual, 2)
			So(combined, ShouldBeGreaterThan, 0.9)
			So(combined, ShouldBeLessThan, 1.5)
		})
	})
}

func TestScorePerspectivesFiltersRegime(t *testing.T) {
	Convey("Given a dead regime and a muted hawkes source", t, func() {
		scores := scorePerspectives(
			map[string]SignalCandidate{
				"hawkes":   {Source: "hawkes", Confidence: 0.8, ExpectedReturn: 0.01},
				"pumpdump": {Source: "pumpdump", Confidence: 0.6, ExpectedReturn: 0.01},
			},
			EnsembleContext{Regime: RegimeDead, Trust: NewSourceTrustStore()},
		)

		Convey("It should keep pumpdump and drop hawkes", func() {
			So(len(scores), ShouldEqual, 1)
			So(scores[0].Perspective, ShouldEqual, engine.PerspectiveMicrostructure)
		})
	})
}
