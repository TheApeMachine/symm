package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChartWatchSubscribe(t *testing.T) {
	Convey("Given a chart watch set", t, func() {
		chartWatch := NewChartWatch("BTC/EUR")

		chartWatch.Subscribe([]string{"ETH/EUR"})

		Convey("It should retain existing symbols and add subscribed symbols", func() {
			So(chartWatch.Has("BTC/EUR"), ShouldBeTrue)
			So(chartWatch.Has("ETH/EUR"), ShouldBeTrue)
		})
	})
}

func TestChartWatchReplace(t *testing.T) {
	Convey("Given a chart watch set with existing symbols", t, func() {
		chartWatch := NewChartWatch("BTC/EUR", "ETH/EUR")

		chartWatch.Replace([]string{"SOL/EUR"})

		Convey("It should keep only the replacement symbols", func() {
			So(chartWatch.Has("BTC/EUR"), ShouldBeFalse)
			So(chartWatch.Has("ETH/EUR"), ShouldBeFalse)
			So(chartWatch.Has("SOL/EUR"), ShouldBeTrue)
		})
	})
}

func BenchmarkChartWatchHas(benchmark *testing.B) {
	chartWatch := NewChartWatch("BTC/EUR", "ETH/EUR", "SOL/EUR")

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if !chartWatch.Has("BTC/EUR") {
			benchmark.Fatal("expected watched symbol")
		}
	}
}
