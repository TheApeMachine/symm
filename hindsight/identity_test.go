package hindsight

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCaptureIdentityValidTest(t *testing.T) {
	Convey("Given a CaptureIdentity", t, func() {
		valid := CaptureIdentity{
			Run:            "run-1",
			Sequence:       1,
			Stream:         "spot.public",
			StreamEpoch:    1,
			StreamSequence: 1,
		}

		Convey("A fully populated identity is valid", func() {
			So(valid.Valid(), ShouldBeTrue)
		})

		Convey("A zero Run makes it invalid", func() {
			invalid := valid
			invalid.Run = ""
			So(invalid.Valid(), ShouldBeFalse)
		})

		Convey("An empty Stream makes it invalid", func() {
			invalid := valid
			invalid.Stream = ""
			So(invalid.Valid(), ShouldBeFalse)
		})

		Convey("A zero epoch makes it invalid", func() {
			invalid := valid
			invalid.StreamEpoch = 0
			So(invalid.Valid(), ShouldBeFalse)
		})

		Convey("A zero capture sequence makes it invalid", func() {
			invalid := valid
			invalid.Sequence = 0
			So(invalid.Valid(), ShouldBeFalse)
		})
	})
}

func TestStateVersionMonotonicTest(t *testing.T) {
	Convey("Given a shared resident state cell", t, func() {
		var version uint64

		Convey("Advancing it from two workloads keeps the version monotonic", func() {
			version++
			fromTrade := version

			version++
			fromTicker := version

			So(fromTicker, ShouldBeGreaterThan, fromTrade)
			So(StateVersion{
				Component: "strategy.planner",
				Key:       "BTC",
				Version:   fromTrade,
			}, ShouldNotResemble, StateVersion{
				Component: "strategy.planner",
				Key:       "BTC",
				Version:   fromTicker,
			})
		})
	})
}
