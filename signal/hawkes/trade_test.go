package hawkes

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestTrade_Measure(testingTB *testing.T) {
	Convey("Given a chronological alternating trade stream", testingTB, func() {
		trade := NewTrade()
		start := time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)
		var measurements []*types.Measurement
		var err error

		for index := range 128 {
			side := "buy"

			if index%2 == 1 {
				side = "sell"
			}

			at := start.Add(time.Duration(index) * time.Millisecond)
			measurements, err = trade.Measure(kraken.TradeData{
				Symbol:    "BTC/USD",
				Side:      side,
				Price:     *decimal.NewFromFloat64(float64(index + 1)),
				Qty:       1,
				OrderType: "market",
				TradeID:   int64(index + 1),
				Timestamp: at,
			})
		}

		Convey("It should publish the fitted typed-arrival measurement", func() {
			So(err, ShouldBeNil)
			So(measurements, ShouldHaveLength, 1)
			So(measurements[0].Source, ShouldEqual, types.SourceHawkes)
			So(measurements[0].Symbol, ShouldEqual, "BTC/USD")
			So(measurements[0].Categories, ShouldHaveLength, 4)
			So(measurements[0].Maturity, ShouldEqual, 1.0)
		})
	})
}

func BenchmarkTrade_Measure(testingTB *testing.B) {
	const eventCount = 1024

	rows := make([]kraken.TradeData, eventCount)
	start := time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)

	for index := range rows {
		side := "buy"

		if index%2 == 1 {
			side = "sell"
		}

		rows[index] = kraken.TradeData{
			Symbol:    "BTC/USD",
			Side:      side,
			Price:     *decimal.NewFromFloat64(float64(index + 1)),
			Qty:       1,
			OrderType: "market",
			TradeID:   int64(index + 1),
			Timestamp: start.Add(time.Duration(index) * time.Millisecond),
		}
	}

	testingTB.ReportAllocs()
	testingTB.ResetTimer()

	for testingTB.Loop() {
		trade := NewTrade()

		for _, row := range rows {
			measurements, err := trade.Measure(row)

			if err != nil {
				testingTB.Fatal(err)
			}

			_ = measurements
		}
	}
}
