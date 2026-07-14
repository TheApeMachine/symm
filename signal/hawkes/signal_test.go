package hawkes

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func measureField(
	measurements []*types.Measurement, symbol string, metric types.MetricType,
) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == metric {
			return measurement, true
		}
	}

	return nil, false
}

func tradeRow(symbol, side string, price float64, quantity float64, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Side:      side,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       quantity,
		Timestamp: at,
	}
}

func TestSignalOnTrade(testingTB *testing.T) {
	Convey("Given a Hawkes signal wired to the trade channel", testingTB, func() {
		signal := &Signal{tradeCache: []kraken.TradeData{}}
		payload := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":100.5,"qty":1.25,"ord_type":"market","trade_id":1,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)

		Convey("When a trade frame arrives over the wire", func() {
			signal.onTrade(payload)

			Convey("Then the row should accumulate in the trade cache", func() {
				So(len(signal.tradeCache), ShouldEqual, 1)
				So(signal.tradeCache[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})

		Convey("When an empty frame arrives", func() {
			signal.onTrade(nil)

			Convey("Then nothing should be cached", func() {
				So(len(signal.tradeCache), ShouldEqual, 0)
			})
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a Hawkes signal with enough marked arrivals to identify a stream", testingTB, func() {
		signal := &Signal{
			ctx:   context.Background(),
			trade: NewTrade(),
		}
		start := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		sides := []string{"buy", "sell", "buy", "sell", "buy", "sell"}
		var result *types.Thesis

		Convey("When each trade arrives on its own tick", func() {
			for index, side := range sides {
				signal.tradeCache = []kraken.TradeData{
					tradeRow("BTC/USD", side, 100+float64(index), 1, start.Add(time.Duration(index)*time.Second)),
				}
				result = signal.Measure(types.NewThesis(nil))
			}

			Convey("Then event-count observation measurements should be emitted", func() {
				count, ok := measureField(result.Measurements, "BTC/USD", types.MetricEventCount)
				So(ok, ShouldBeTrue)
				So(count.Raw, ShouldBeGreaterThan, 0)

				So(len(signal.tradeCache), ShouldEqual, 0)
			})
		})
	})
}
