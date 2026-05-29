package leadlag

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestSymbolStateApplyFeedback(t *testing.T) {
	Convey("Given a symbol forecast learner", t, func() {
		state := newSymbolState(testPair())

		err := state.applyFeedback(0.02, -0.01)

		Convey("It should fold feedback into the forecast scale", func() {
			So(err, ShouldBeNil)
			So(state.forecastScale(), ShouldBeLessThan, 1)
		})
	})
}

func TestSymbolStateChange(t *testing.T) {
	Convey("Given an observed ticker", t, func() {
		state := newSymbolState(testPair())
		at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		state.observeTicker(0.05, 100, 99.9, 100.1, at)

		Convey("It should expose the latest change percent", func() {
			So(state.change(), ShouldAlmostEqual, 0.05, 1e-12)
		})
	})
}

func testPair() asset.Pair {
	return asset.Pair{Wsname: "ALT/EUR", Quote: "EUR"}
}
