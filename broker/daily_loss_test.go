package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDailyLossRecord(t *testing.T) {
	Convey("Given a fresh DailyLoss tracker", t, func() {
		dailyLoss := NewDailyLoss()

		Convey("A winning trade never registers as a loss", func() {
			dailyLoss.Record(*decimal.NewFromFloat64(12.5))

			So(dailyLoss.Exceeds(0.01), ShouldBeFalse)
		})

		Convey("A losing trade accumulates its magnitude", func() {
			dailyLoss.Record(*decimal.NewFromFloat64(-3))

			So(dailyLoss.Exceeds(2.99), ShouldBeTrue)
			So(dailyLoss.Exceeds(3.01), ShouldBeFalse)
		})

		Convey("Multiple losing trades accumulate across the same day", func() {
			dailyLoss.Record(*decimal.NewFromFloat64(-3))
			dailyLoss.Record(*decimal.NewFromFloat64(-4))

			So(dailyLoss.Exceeds(6.99), ShouldBeTrue)
			So(dailyLoss.Exceeds(7.01), ShouldBeFalse)
		})

		Convey("A winning trade does not offset an earlier loss", func() {
			dailyLoss.Record(*decimal.NewFromFloat64(-5))
			dailyLoss.Record(*decimal.NewFromFloat64(10))

			So(dailyLoss.Exceeds(4.99), ShouldBeTrue)
		})

		Convey("A non-positive limit never blocks", func() {
			dailyLoss.Record(*decimal.NewFromFloat64(-1000))

			So(dailyLoss.Exceeds(0), ShouldBeFalse)
			So(dailyLoss.Exceeds(-1), ShouldBeFalse)
		})
	})

	Convey("Given a nil DailyLoss tracker", t, func() {
		var dailyLoss *DailyLoss

		Convey("Record and Exceeds are safe no-ops", func() {
			So(func() { dailyLoss.Record(*decimal.NewFromFloat64(-100)) }, ShouldNotPanic)
			So(dailyLoss.Exceeds(1), ShouldBeFalse)
		})
	})
}

func BenchmarkDailyLossRecord(b *testing.B) {
	dailyLoss := NewDailyLoss()
	loss := *decimal.NewFromFloat64(-1.5)

	b.ReportAllocs()
	for b.Loop() {
		dailyLoss.Record(loss)
	}
}
