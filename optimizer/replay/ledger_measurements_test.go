package replay

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestReplayMeasurementsSnapshotOrdering(t *testing.T) {
	convey.Convey("Given replay measurements with global and symbol rows", t, func() {
		measurements := newReplayMeasurements()
		measurements.Add(perspectives.Measurement{Source: perspectives.SourceSentiment})
		measurements.Add(perspectives.Measurement{
			Symbol: "BTC/EUR",
			Source: perspectives.SourceHawkes,
		})

		convey.Convey("It should return the rows available to the symbol", func() {
			rows := measurements.Snapshot("BTC/EUR")

			convey.So(rows, convey.ShouldHaveLength, 2)
			convey.So(rows[0].Source, convey.ShouldEqual, perspectives.SourceSentiment)
			convey.So(rows[1].Source, convey.ShouldEqual, perspectives.SourceHawkes)
		})
	})
}
