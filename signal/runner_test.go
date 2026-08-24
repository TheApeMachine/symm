package signal

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestRunner(t *testing.T) {
	Convey("Given a Workspace and a Signal Runner", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		bus := runtime.NewWorkspace(nil)
		defer bus.Close()

		runner := NewRunner(ctx, bus)
		defer runner.Close()

		var measurementCount atomic.Int64
		measurements := runtime.ChannelOf[*nmtypes.Measurement](
			bus, types.ChannelMeasurements,
			func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
		)

		measurements.Subscribe("test-collector", func(measurement *nmtypes.Measurement) error {
			measurementCount.Add(1)
			return nil
		})

		Convey("When tickers and trades are published to the bus", func() {
			tickers := runtime.ChannelOf[kraken.TickerData](
				bus, types.ChannelTickers,
				func(ticker kraken.TickerData) string { return ticker.Symbol },
			)

			trades := runtime.ChannelOf[kraken.TradeData](
				bus, types.ChannelTrades,
				func(trade kraken.TradeData) string { return trade.Symbol },
			)

			bid := decimal.NewFromFloat64(65000.0)
			ask := decimal.NewFromFloat64(65001.0)
			last := decimal.NewFromFloat64(65000.5)

			tickers.Publish(kraken.TickerData{
				Symbol:    "BTC/USD",
				Bid:       bid,
				Ask:       ask,
				BidQty:    5.0,
				AskQty:    5.0,
				Last:      last,
				Timestamp: time.Now(),
			})

			trades.Publish(kraken.TradeData{
				Symbol:    "BTC/USD",
				Side:      "buy",
				Price:     *decimal.NewFromFloat64(65000.5),
				Qty:       1.5,
				Timestamp: time.Now(),
			})

			deadline := time.Now().Add(2 * time.Second)
			for measurementCount.Load() == 0 && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}

			Convey("Signals should process concurrently and emit measurements", func() {
				So(measurementCount.Load(), ShouldBeGreaterThan, 0)
			})
		})
	})
}

func BenchmarkRunnerTickerStep(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := runtime.NewWorkspace(nil)
	defer bus.Close()

	runner := NewRunner(ctx, bus)
	defer runner.Close()

	tickers := runtime.ChannelOf[kraken.TickerData](
		bus, types.ChannelTickers,
		func(ticker kraken.TickerData) string { return ticker.Symbol },
	)

	bid := decimal.NewFromFloat64(65000.0)
	ask := decimal.NewFromFloat64(65001.0)
	last := decimal.NewFromFloat64(65000.5)

	tickerData := kraken.TickerData{
		Symbol:    "BTC/USD",
		Bid:       bid,
		Ask:       ask,
		BidQty:    5.0,
		AskQty:    5.0,
		Last:      last,
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tickers.Publish(tickerData)
	}
}
