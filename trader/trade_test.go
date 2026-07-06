package trader

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTradeMeasure(testingTB *testing.T) {
	Convey("Given a trade with a typed signal", testingTB, func() {
		recording := &recordingSignal{}
		trade := NewTrade([]types.Signal[any]{recording})
		message := kraken.TradeDataSlice{{
			Symbol:    "MATIC/USD",
			Side:      "buy",
			Price:     0.5147,
			Qty:       6423.46326,
			OrderType: "limit",
			TradeID:   4665846,
			Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		}}

		Convey("When trade data is measured", func() {
			measurements, err := trade.Measure(message)

			Convey("It should measure each row through the signal", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldHaveLength, 1)
				So(recording.rows, ShouldHaveLength, 1)
				row := recording.rows[0].(kraken.TradeData)
				So(row.Symbol, ShouldEqual, "MATIC/USD")
			})
		})
	})
}

func BenchmarkTradeMeasure(benchmarkTB *testing.B) {
	trade := NewTrade([]types.Signal[any]{
		&benchmarkSignal{},
	})
	message := kraken.TradeDataSlice{{
		Symbol:    "MATIC/USD",
		Side:      "buy",
		Price:     0.5147,
		Qty:       6423.46326,
		OrderType: "limit",
		TradeID:   4665846,
		Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
	}}

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		if _, err := trade.Measure(message); err != nil {
			benchmarkTB.Fatal(err)
		}
	}
}
