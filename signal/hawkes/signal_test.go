package hawkes

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

type testMarketFeed struct {
	ticker *types.Subscription[*kraken.Ticker]
	book   *types.Subscription[*kraken.Book]
	trade  *types.Subscription[*kraken.Trade]
}

func (feed testMarketFeed) Ticker() *types.Subscription[*kraken.Ticker] { return feed.ticker }

func (feed testMarketFeed) Book() *types.Subscription[*kraken.Book] { return feed.book }

func (feed testMarketFeed) Trade() *types.Subscription[*kraken.Trade] { return feed.trade }

func (feed testMarketFeed) Instrument() *types.Subscription[*kraken.Instrument] {
	return types.NewSubscription[*kraken.Instrument]()
}

func newTestSignal() *Signal {
	thesis := types.NewThesis()

	return &Signal{
		ctx:     context.Background(),
		planner: &strategy.Planner{Thesis: thesis},
	}
}

func calc(signal *Signal, rows ...kraken.TradeData) []*types.Measurement {
	return signal.Calculate(nil, rows, nil)
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

func TestSignal_Calculate(t *testing.T) {
	Convey("Given a Hawkes signal driven by the central market cut", t, func() {
		signal := newTestSignal()
		at := time.Date(2023, 9, 25, 9, 4, 31, 0, time.UTC)
		row := tradeRow("BTC/USD", "buy", 100.5, 1.25, at)

		Convey("When an empty trade batch arrives", func() {
			measurements := calc(signal)

			Convey("Then nothing should be measured", func() {
				So(measurements, ShouldBeEmpty)
			})
		})

		Convey("When a malformed marked arrival is calculated", func() {
			invalid := row
			invalid.Side = "hold"
			measurements := calc(signal, invalid)

			Convey("Then the invalid input should produce no measurements", func() {
				So(measurements, ShouldBeEmpty)
			})
		})

		Convey("When ingress overlaps drains", func() {
			previous := viper.GetInt("system.actor.buffer")
			viper.Set("system.actor.buffer", 64)
			Reset(func() { viper.Set("system.actor.buffer", previous) })

			signal := NewSignal(
				context.Background(),
				nil,
				&strategy.Planner{Thesis: types.NewThesis()},
				nil,
			)
			signal.subscriptions["trade"] = types.NewSubscription[any]()
			signal.run()

			for range 32 {
				signal.subscriptions["trade"].Send(&kraken.Trade{Data: []kraken.TradeData{row}})
			}

			time.Sleep(50 * time.Millisecond)
			So(signal.Close(), ShouldBeNil)
		})
	})
}

func BenchmarkSignal_Calculate(benchmark *testing.B) {
	signal := newTestSignal()
	start := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	for index := range 16 {
		side := "buy"

		if index%2 == 0 {
			side = "sell"
		}

		calc(signal, tradeRow(
			"MATIC/USD",
			side,
			0.56+float64(index)*0.001,
			1+float64(index),
			start.Add(time.Duration(index)*100*time.Millisecond),
		))
	}

	benchmark.ReportAllocs()
	index := 16

	for benchmark.Loop() {
		side := "buy"

		if index%2 == 0 {
			side = "sell"
		}

		calc(signal, tradeRow(
			"MATIC/USD",
			side,
			0.56+float64(index)*0.001,
			1+float64(index),
			start.Add(time.Duration(index)*100*time.Millisecond),
		))

		index++
	}
}
