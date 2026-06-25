package manifold

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/signal/compute"
)

func TestSignalIntegrateSerializesMeasureAndFeed(testingTB *testing.T) {
	Convey("Given concurrent measure and feed paths", testingTB, func() {
		viper.Set("market.book_depth_levels", 4)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		signal := NewSignal(ctx, dmt.NewTree(""))

		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		signal.field.RegisterSymbols([]string{"BTC/USD"})
		state := signal.field.universe.loadSymbol("BTC/USD")
		state.midPrice = 50000
		state.lastPrice = 50000
		state.tickSize = 0.01
		state.bookReady = true
		state.book = BookUpdate{
			Symbol: "BTC/USD",
			Bids:   []BookLevel{{Price: 49990, Qty: 1}},
			Asks:   []BookLevel{{Price: 50010, Qty: 1}},
		}

		at := time.Date(2026, 6, 25, 2, 0, 0, 0, time.UTC)
		var waitGroup sync.WaitGroup

		for range 32 {
			waitGroup.Add(2)

			go func() {
				defer waitGroup.Done()

				signal.integrateField(at)
			}()

			go func() {
				defer waitGroup.Done()

				_ = signal.field.enqueueTrade(&TradeUpdate{
					Symbol: "BTC/USD",
					Price:  50001,
					Qty:    0.5,
					Side:   "buy",
				}, at)
			}()
		}

		waitGroup.Wait()
		time.Sleep(200 * time.Millisecond)

		Convey("It should leave the solver initialized", func() {
			ready := compute.Run(signal.serial, func() bool {
				return signal.field.solver != nil
			})

			So(ready, ShouldBeTrue)
		})
	})
}
