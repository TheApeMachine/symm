package broker

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestStressSlotStore(t *testing.T) {
	convey.Convey("Given a stress cache", t, func() {
		cache := NewStressCache(context.Background(), nil)
		cache.ingestMeasurement(perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceHawkes,
			Category: perspectives.CategorySaturation,
			SNR:      2.5,
		})

		convey.Convey("It should snapshot Hawkes stress without a read lock", func() {
			stress := cache.Snapshot("BTC/EUR")

			convey.So(stress.HawkesCategory, convey.ShouldEqual, perspectives.CategorySaturation)
			convey.So(stress.HawkesSNR, convey.ShouldEqual, 2.5)
		})
	})
}
