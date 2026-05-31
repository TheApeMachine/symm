package exhaust

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func thinningBook(symbol string, bidDepth float64, askPrice float64) market.BookUpdate {
	update := market.BookUpdate{
		Symbol: symbol,
		Bids: []market.BookLevel{{
			Price: 100, Qty: bidDepth,
			PriceRaw: "100", QtyRaw: fmt.Sprintf("%g", bidDepth),
		}},
		Asks: []market.BookLevel{{
			Price: askPrice, Qty: bidDepth * 0.5,
			PriceRaw: fmt.Sprintf("%g", askPrice), QtyRaw: fmt.Sprintf("%g", bidDepth*0.5),
		}},
	}
	update.SetEnvelopeType(market.BookSnapshot)

	return update
}

func TestNewSignal(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		Convey("It should expose a measurements broadcast", func() {
			So(signal.broadcasts["measurements"], ShouldNotBeNil)
		})
	})
}

func TestObserveBook(t *testing.T) {
	Convey("Given an exhaust signal with a measurements subscriber", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:exhaust", 64)
		symbol := "ETH/EUR"

		Convey("When bid depth thins and spreads widen over successive frames", func() {
			for index := range 8 {
				depth := 20.0 - float64(index)*2
				askPrice := 101.0 + float64(index)*0.5
				signal.observeBook(thinningBook(symbol, depth, askPrice))
				signal.emit(symbol)
			}

			var measurement perspectives.Measurement
			received := false
			deadline := time.After(time.Second)

			for !received {
				select {
				case value := <-measurements.Incoming:
					reading, ok := value.Value.(perspectives.Measurement)

					if ok {
						measurement = reading
						received = true
					}
				case <-deadline:
					t.Fatal("timed out waiting for exhaust measurement")
				}
			}

			Convey("It publishes an exhaustion reading", func() {
				So(measurement.Source, ShouldEqual, perspectives.SourceExhaustion)
				So(measurement.Symbol, ShouldEqual, symbol)
				So(measurement.Strength, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func BenchmarkObserveBook(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer signal.Close()

	signal.broadcasts["measurements"].Subscribe("bench:exhaust", 1024)

	symbol := "ETH/EUR"
	delta := thinningBook(symbol, 12, 103)

	b.ReportAllocs()

	for b.Loop() {
		signal.observeBook(delta)
		signal.emit(symbol)
	}
}
