package toxicity

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestTrackerMeasureToxicBluff(t *testing.T) {
	convey.Convey("Given a near-touch toxic cancel flag", t, func() {
		tracker := NewTracker()
		now := time.Now()
		symbol := "ETH/EUR"

		tracker.ObserveMid(symbol, market.Pair{}, 100)
		state := tracker.stateLocked(symbol, market.Pair{})
		state.toxic[100] = now.Add(time.Minute)

		measurement, ok := tracker.Measure(symbol, now)

		convey.Convey("It should publish toxic bluff with SNR", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(measurement.Category, convey.ShouldEqual, perspectives.CategoryToxicBluff)
			convey.So(measurement.SNR, convey.ShouldBeGreaterThan, 0)
		})
	})
}
