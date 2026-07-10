package logic

import (
	"testing"
	"time"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestManifoldNormalize(t *testing.T) {
	Convey("Given a manifold metric baseline", t, func() {
		manifold := &Manifold{
			halflife:  time.Second,
			baselines: map[string]*adaptive.TimeElastic{},
		}

		Convey("When the first value seeds the baseline", func() {
			value, ready := manifold.normalize("spread", 10, time.Unix(1, 0))

			Convey("Then it reports a defined zero-deviation baseline from the first reading", func() {
				So(ready, ShouldBeTrue)
				So(value, ShouldAlmostEqual, 0, 0.000001)
			})
		})

		Convey("When equal values arrive after the baseline is seeded", func() {
			_, _ = manifold.normalize("spread", 10, time.Unix(1, 0))
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

		Convey("When a stale timestamp arrives after a fresher event frontier", func() {
			manifold.lastEventAt = map[string]time.Time{
				"fluid/book": time.Unix(2, 0),
			}
			measurement := &types.Measurement{
				Source: types.SourceFluid,
				Stream: "book",
				At:     time.Unix(1, 0),
			}

			Convey("Then the measurement is rejected before baseline normalization", func() {
				So(manifold.eventStale(measurement), ShouldBeTrue)
			})
		})

		Convey("When another stream has a fresher frontier", func() {
			manifold.lastEventAt = map[string]time.Time{
				"fluid/book": time.Unix(2, 0),
			}
			measurement := &types.Measurement{
				Source: types.SourceLeadLag,
				Stream: "ticker",
				At:     time.Unix(1, 0),
			}

			Convey("Then it should not reject the independent stream", func() {
				So(manifold.eventStale(measurement), ShouldBeFalse)
			})
		})
	})
}

func TestSortMeasurementsByAt(t *testing.T) {
	Convey("Given out-of-order measurements for one symbol", t, func() {
		late := &types.Measurement{At: time.Unix(3, 0)}
		early := &types.Measurement{At: time.Unix(1, 0)}
		middle := &types.Measurement{At: time.Unix(2, 0)}
		measurements := []*types.Measurement{late, early, middle}

		sortMeasurementsByAt(measurements)

		Convey("Then they are ordered by event time", func() {
			So(measurements[0], ShouldEqual, early)
			So(measurements[1], ShouldEqual, middle)
			So(measurements[2], ShouldEqual, late)
		})
	})
}

func TestManifoldLearn(t *testing.T) {
	Convey("Given a manifold without matching attractor basins", t, func() {
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

	Convey("Given a manifold with a matching attractor basin", t, func() {
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
