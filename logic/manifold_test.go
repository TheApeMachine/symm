package logic

import (
	"testing"
	"time"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/nomagique/adaptive"

	. "github.com/smartystreets/goconvey/convey"
)

func TestManifoldNormalize(testingTB *testing.T) {
	Convey("Given a manifold metric baseline", testingTB, func() {
		manifold := &Manifold{
			halflife:  time.Second,
			baselines: map[string]*adaptive.TimeElastic{},
		}

		Convey("When equal values arrive after the baseline is ready", func() {
			_, ready := manifold.normalize("spread", 10, time.Unix(1, 0))
			So(ready, ShouldBeFalse)

			value, ready := manifold.normalize("spread", 10, time.Unix(2, 0))

			Convey("Then the normalized value is centered on zero deviation", func() {
				So(ready, ShouldBeTrue)
				So(value, ShouldAlmostEqual, 0, 0.000001)
			})
		})

		Convey("When a materially larger value arrives", func() {
			_, _ = manifold.normalize("volume", 10, time.Unix(1, 0))
			value, ready := manifold.normalize("volume", 20, time.Unix(2, 0))

			Convey("Then the normalized value is positive deviation", func() {
				So(ready, ShouldBeTrue)
				So(value, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestManifoldLearn(testingTB *testing.T) {
	Convey("Given a manifold without matching attractor basins", testingTB, func() {
		manifold := &Manifold{
			tree: dmt.NewTree(""),
		}

		Convey("When the manifold learns a novel category sequence", func() {
			learned := manifold.learn([]byte("1_2"))

			Convey("Then the missing attractor is treated as an unreadiness state", func() {
				So(learned, ShouldBeFalse)
			})
		})
	})

	Convey("Given a manifold with a matching attractor basin", testingTB, func() {
		tree := dmt.NewTree("")
		_, _, err := tree.InsertAttractorBasin(
			[]byte("movement"),
			[]byte("1"),
			dmt.CognitiveState{Count: 4, Probability: 0.6},
		)
		So(err, ShouldBeNil)

		manifold := &Manifold{
			tree: tree,
		}

		Convey("When the manifold learns the category sequence", func() {
			learned := manifold.learn([]byte("1_2"))

			Convey("Then it updates sensory evidence through DMT", func() {
				state := tree.GetSensoryWeight([]byte("1"))

				So(learned, ShouldBeTrue)
				So(state.Count, ShouldBeGreaterThan, 0)
			})
		})
	})
}
