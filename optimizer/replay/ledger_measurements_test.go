package replay

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestReplayMeasurementsSnapshotOrdering(t *testing.T) {
	convey.Convey("Given replay measurements with global and symbol rows", t, func() {
		measurements := newReplayMeasurements()
		measurements.Add(types.Measurement{Source: types.SourceSentiment})
		measurements.Add(types.Measurement{
			Symbol: "BTC/EUR",
			Source: types.SourceHawkes,
		})

		convey.Convey("It should return the rows available to the symbol", func() {
			rows := measurements.Snapshot("BTC/EUR")

			convey.So(rows, convey.ShouldHaveLength, 2)
			convey.So(rows[0].Source, convey.ShouldEqual, types.SourceSentiment)
			convey.So(rows[1].Source, convey.ShouldEqual, types.SourceHawkes)
		})
	})
}
