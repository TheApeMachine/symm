package trader

import (
	"strconv"
	"testing"

	"github.com/spf13/viper"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLevel3SchedulerMark(t *testing.T) {
	Convey("Given a hot symbol followed by two independently ready symbols", t, func() {
		scheduler := NewLevel3Scheduler()
		So(scheduler.Mark("A"), ShouldBeTrue)
		So(scheduler.Mark("A"), ShouldBeFalse)
		So(scheduler.Mark("A"), ShouldBeFalse)
		So(scheduler.Mark("B"), ShouldBeTrue)
		So(scheduler.Mark("C"), ShouldBeTrue)

		Convey("When A becomes ready again after its first turn", func() {
			order := make([]string, 0, 4)
			symbol, ok := scheduler.Next()
			So(ok, ShouldBeTrue)
			order = append(order, symbol)
			So(scheduler.Mark("A"), ShouldBeTrue)

			for scheduler.Len() > 0 {
				symbol, ok = scheduler.Next()
				So(ok, ShouldBeTrue)
				order = append(order, symbol)
			}

			Convey("It should schedule A, B, C, A without duplicate starvation", func() {
				So(order, ShouldResemble, []string{"A", "B", "C", "A"})
			})
		})
	})
}

func TestLevel3SchedulerRemove(t *testing.T) {
	Convey("Given a queued symbol whose newest observation is invalid", t, func() {
		scheduler := NewLevel3Scheduler()
		scheduler.Mark("A")
		scheduler.Mark("B")

		Convey("When the invalid symbol is removed", func() {
			So(scheduler.Remove("A"), ShouldBeTrue)

			Convey("It should not receive an advancement turn", func() {
				symbol, ok := scheduler.Next()
				So(ok, ShouldBeTrue)
				So(symbol, ShouldEqual, "B")
			})
		})
	})
}

func BenchmarkLevel3SchedulerMark(b *testing.B) {
	scheduler := NewLevel3Scheduler()
	b.ReportAllocs()

	for b.Loop() {
		scheduler.Mark("BTC/USD")
		scheduler.Next()
	}
}

func BenchmarkLevel3SchedulerNextTradingTier(b *testing.B) {
	symbols := make([]string, viper.GetInt("market.universe.trading_tier_size"))

	for index := range symbols {
		symbols[index] = strconv.Itoa(index)
	}

	scheduler := NewLevel3Scheduler()
	b.ReportAllocs()

	for b.Loop() {
		for _, symbol := range symbols {
			scheduler.Mark(symbol)
		}

		for scheduler.Len() > 0 {
			scheduler.Next()
		}
	}
}
