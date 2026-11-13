package types

import (
	"testing"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPrediction(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When a Prediction is constructed", func() {
			pred := Prediction{
				Symbol:    "BTC/USD",
				Timestamp: 1700000000,
				Price:     decimal.RequireFromString("42150.25"),
			}

			Convey("symbol, timestamp, and price are exact", func() {
				So(pred.Symbol, ShouldEqual, "BTC/USD")
				So(pred.Timestamp, ShouldEqual, uint64(1700000000))
				So(pred.Price.String(), ShouldEqual, "42150.25")
			})
		})
	})
}

func BenchmarkPrediction(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		pred := Prediction{
			Symbol: "BTC/USD", Timestamp: 1700000000,
			Price: decimal.RequireFromString("42150.25"),
		}
		_ = pred.Price.String()
	}
}
