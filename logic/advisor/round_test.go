package advisor

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

func TestNewArenaRound(t *testing.T) {
	Convey("Given a valid issued Perspective and its winning Class", t, func() {
		features := predictiveMidpointFeatures()
		perspective := arenaPerspective("BTC/USD", "recovery", 1, 3)
		round, err := newArenaRound(
			arenaEnvelope("BTC/USD", 1, 1, 1),
			perspective,
			features[0].Class,
		)

		Convey("the round owns an independent Perspective and seeded observations", func() {
			So(err, ShouldBeNil)
			So(round, ShouldNotBeNil)
			So(round.perspective.Key(), ShouldResemble, perspective.Key())
			So(round.baselines, ShouldHaveLength, 2)
		})
	})
}

func TestArenaRoundAdvance(t *testing.T) {
	Convey("Given a round at one completed market coordinate", t, func() {
		round := &arenaRound{
			clock:      Falsifiable{Label: "pumpdump/completed_volume_bar_ordinal", Type: METRIC},
			coordinate: 2,
		}

		Convey("a later completed coordinate advances it", func() {
			coordinate, advanced, err := round.advance(arenaEnvelope("BTC/USD", 1, 1, 3))
			So(err, ShouldBeNil)
			So(advanced, ShouldBeTrue)
			So(coordinate, ShouldEqual, uint64(3))
		})

		Convey("a backward coordinate returns a typed conflict", func() {
			_, _, err := round.advance(arenaEnvelope("BTC/USD", 1, 1, 1))
			So(errnie.IsConflict(err), ShouldBeTrue)
		})

		Convey("a missing clock leaves the round unchanged", func() {
			envelope := types.NewEnvelope(types.EnvelopeTicker)
			coordinate, advanced, err := round.advance(envelope)
			So(err, ShouldBeNil)
			So(advanced, ShouldBeFalse)
			So(coordinate, ShouldEqual, uint64(2))
		})
	})
}
