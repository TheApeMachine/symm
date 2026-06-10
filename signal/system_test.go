package signal

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type bookRecordStub struct {
	symbol   string
	recorded chan any
}

func (stub *bookRecordStub) Measure(*market.Feedback, time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, nil
}

func (stub *bookRecordStub) Record(item any) bool {
	stub.recorded <- item
	return false
}

func (stub *bookRecordStub) Symbol() string {
	return stub.symbol
}

func TestSystemTickBookUpdates(t *testing.T) {
	Convey("Given a book update batch on the raw bus", t, func() {
		viper.Set("system.queue.ttl", time.Second)
		viper.Set("system.queue.buffer", 8)
		viper.Set("telemetry.gauge.readings_capacity", 8)
		viper.Set("signals.fluid.measurements_capacity", 4)

		recorded := make(chan any, 2)
		ctx, cancel := context.WithCancel(context.Background())
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		defer pool.Close()

		var system *System

		t.Cleanup(func() {
			cancel()

			if system != nil {
				_ = system.Close()
			}
		})

		system = NewSystem(ctx, pool, logic.SourceFluid, func(symbol string, entity *logic.Entity) market.Signal {
			return &bookRecordStub{
				symbol:   symbol,
				recorded: recorded,
			}
		})

		So(system, ShouldNotBeNil)

		go system.Tick()

		updates := krakenmarket.BookUpdates{
			{Symbol: "BTC/USD"},
			{Symbol: "ETH/USD"},
		}

		So(system.bus.Send("raw", "book", &updates), ShouldBeNil)

		Convey("It should deliver each BookUpdate to Record", func() {
			first := readRecordedBook(recorded)
			second := readRecordedBook(recorded)

			So(first.Symbol, ShouldEqual, "BTC/USD")
			So(second.Symbol, ShouldEqual, "ETH/USD")
		})
	})
}

func readRecordedBook(recorded chan any) *krakenmarket.BookUpdate {
	var item any

	select {
	case item = <-recorded:
	case <-time.After(2 * time.Second):
		So("book update", ShouldEqual, "received")
	}

	book, ok := item.(*krakenmarket.BookUpdate)

	So(ok, ShouldBeTrue)

	return book
}
