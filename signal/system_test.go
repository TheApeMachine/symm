package signal

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/rawbus"
)

type bookRecordStub struct {
	symbol   string
	recorded chan any
}

func (stub *bookRecordStub) Measure(_ *market.Feedback, _ time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, nil
}

func (stub *bookRecordStub) Record(item any) bool {
	stub.recorded <- item
	return true
}

func (stub *bookRecordStub) Symbol() string {
	return stub.symbol
}

type publishableStub struct {
	symbol      string
	source      logic.SourceType
	measurement logic.Measurement
	recorded    chan any
	measureMu   sync.Mutex
	measureHits map[string]int
}

func (stub *publishableStub) Measure(_ *market.Feedback, at time.Time) (logic.Measurement, error) {
	stub.measureMu.Lock()
	defer stub.measureMu.Unlock()

	if stub.measureHits == nil {
		stub.measureHits = make(map[string]int)
	}

	stub.measureHits[stub.symbol]++

	measurement := stub.measurement
	measurement.ObservedAt = at

	return measurement, nil
}

func (stub *publishableStub) Record(item any) bool {
	if stub.recorded != nil {
		stub.recorded <- item
	}

	return true
}

func (stub *publishableStub) Symbol() string {
	return stub.symbol
}

func (stub *publishableStub) measureCount(symbol string) int {
	stub.measureMu.Lock()
	defer stub.measureMu.Unlock()

	return stub.measureHits[symbol]
}

func newSystemTestPool(test *testing.T) (context.Context, context.CancelFunc, *qpool.Q[any]) {
	test.Helper()

	viper.Set("system.queue.ttl", time.Second)
	viper.Set("system.queue.buffer", 16)
	viper.Set("telemetry.gauge.readings_capacity", 16)
	viper.Set("signals.fluid.measurements_capacity", 4)
	viper.Set("signals.hawkes.measurements_capacity", 4)
	viper.Set("signals.depthflow.measurements_capacity", 4)

	ctx, cancel := context.WithCancel(context.Background())
	pool := qpool.NewQ[any](ctx, 2, 16, nil)

	test.Cleanup(func() {
		cancel()
		pool.Close()
	})

	return ctx, cancel, pool
}

func publishableFixture(
	test *testing.T,
	symbol string,
	source logic.SourceType,
	at time.Time,
) logic.Measurement {
	test.Helper()

	row, err := krakenmarket.NewSymbolRow(symbol, 100, 0.01, 1000, 1, at)

	if err != nil {
		test.Fatal(err)
	}

	return logic.Measurement{
		Source:     source,
		Symbol:     symbol,
		Price:      100,
		Strength:   0.5,
		Volume:     1000,
		Spread:     1,
		Elapsed:    1,
		Category:   logic.CategoryOrganic,
		Confidence: 0.5,
		Surprise:   0.5,
		ObservedAt: at,
		Market:     *row,
	}
}

func TestSystemTickBookUpdates(t *testing.T) {
	Convey("Given a book update batch on the raw bus", t, func() {
		ctx, cancel, pool := newSystemTestPool(t)

		recorded := make(chan any, 2)
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

		So(rawbus.Send(system.bus, rawbus.TypeBook, &updates), ShouldBeNil)

		Convey("It should deliver each BookUpdate to Record", func() {
			first := readRecordedBook(recorded)
			second := readRecordedBook(recorded)

			So(first.Symbol, ShouldEqual, "BTC/USD")
			So(second.Symbol, ShouldEqual, "ETH/USD")
		})
	})
}

func TestSystemPublishKnownSymbolsOnSymbolAnnounce(t *testing.T) {
	Convey("Given a signal system subscribed to measurements", t, func() {
		ctx, cancel, pool := newSystemTestPool(t)
		at := time.Unix(100, 0)
		stub := &publishableStub{
			source:      logic.SourceHawkes,
			measurement: publishableFixture(t, "BTC/USD", logic.SourceHawkes, at),
		}

		subscriber := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelMeasurements},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelMeasurements, "system-test"),
			},
		)

		system := NewSystem(
			ctx,
			pool,
			logic.SourceHawkes,
			func(symbol string, entity *logic.Entity) market.Signal {
				stub.symbol = symbol
				return stub
			},
			logic.EntityTrade,
		)

		So(system, ShouldNotBeNil)
		So(subscriber, ShouldNotBeNil)

		t.Cleanup(func() {
			cancel()
			_ = system.Close()
			_ = subscriber.Close()
		})

		go system.Tick()

		So(rawbus.Send(system.bus, rawbus.TypeSymbols, []string{"BTC/USD", "ETH/USD"}), ShouldBeNil)

		Convey("It should measure every announced symbol without waiting for market data", func() {
			time.Sleep(300 * time.Millisecond)

			So(stub.measureCount("BTC/USD"), ShouldBeGreaterThan, 0)
			So(stub.measureCount("ETH/USD"), ShouldBeGreaterThan, 0)

			received := 0

			for range 2 {
				frame, receiveErr := subscriber.Receive(internal.ChannelMeasurements)

				So(receiveErr, ShouldBeNil)
				So(frame, ShouldNotBeNil)

				measurement, ok := frame.Value.(logic.Measurement)

				So(ok, ShouldBeTrue)
				So(measurement.Publishable(), ShouldBeTrue)
				received++
			}

			So(received, ShouldEqual, 2)
		})
	})
}

func TestSystemPublishKnownSymbolsSweep(t *testing.T) {
	Convey("Given two known symbols and a book update for one symbol", t, func() {
		ctx, cancel, pool := newSystemTestPool(t)
		at := time.Unix(100, 0)
		stub := &publishableStub{
			source:      logic.SourceDepthFlow,
			measurement: publishableFixture(t, "BTC/USD", logic.SourceDepthFlow, at),
		}

		system := NewSystem(
			ctx,
			pool,
			logic.SourceDepthFlow,
			func(symbol string, entity *logic.Entity) market.Signal {
				stub.symbol = symbol
				stub.measurement = publishableFixture(t, symbol, logic.SourceDepthFlow, at)
				return stub
			},
			logic.EntityBook,
		)

		So(system, ShouldNotBeNil)

		t.Cleanup(func() {
			cancel()
			_ = system.Close()
		})

		go system.Tick()

		So(rawbus.Send(system.bus, rawbus.TypeSymbols, []string{"BTC/USD", "ETH/USD"}), ShouldBeNil)

		updates := krakenmarket.BookUpdates{
			{
				Symbol:    "BTC/USD",
				Timestamp: at,
				Bids:      []krakenmarket.BookLevel{{Price: 99, Qty: 1}},
				Asks:      []krakenmarket.BookLevel{{Price: 101, Qty: 1}},
			},
		}

		So(rawbus.Send(system.bus, rawbus.TypeBook, &updates), ShouldBeNil)

		Convey("It should measure both symbols on the sweep", func() {
			time.Sleep(300 * time.Millisecond)

			So(stub.measureCount("BTC/USD"), ShouldBeGreaterThan, 0)
			So(stub.measureCount("ETH/USD"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSystemAlwaysMeasuresWithoutWarmupGate(t *testing.T) {
	Convey("Given a signal whose Record still reports warmup", t, func() {
		ctx, cancel, pool := newSystemTestPool(t)
		at := time.Unix(100, 0)

		subscriber := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelMeasurements},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelMeasurements, "system-test"),
			},
		)

		system := NewSystem(
			ctx,
			pool,
			logic.SourceFluid,
			func(symbol string, entity *logic.Entity) market.Signal {
				return &warmupRecordStub{
					symbol:      symbol,
					measurement: publishableFixture(t, symbol, logic.SourceFluid, at),
				}
			},
			logic.EntityBook,
		)

		So(system, ShouldNotBeNil)

		t.Cleanup(func() {
			cancel()
			_ = system.Close()
			_ = subscriber.Close()
		})

		go system.Tick()

		updates := krakenmarket.BookUpdates{{Symbol: "BTC/USD", Timestamp: at}}
		So(rawbus.Send(system.bus, rawbus.TypeBook, &updates), ShouldBeNil)

		Convey("It should still publish measurements on the first event", func() {
			frame, receiveErr := subscriber.Receive(internal.ChannelMeasurements)

			So(receiveErr, ShouldBeNil)

			measurement, ok := frame.Value.(logic.Measurement)

			So(ok, ShouldBeTrue)
			So(measurement.Publishable(), ShouldBeTrue)
			So(measurement.Symbol, ShouldEqual, "BTC/USD")
		})
	})
}

func TestSystemTickHaltsOnMeasureError(t *testing.T) {
	Convey("Given a signal that fails Measure", t, func() {
		ctx, cancel, pool := newSystemTestPool(t)

		measureErr := errors.New("fluid: instrument broken")
		system := NewSystem(ctx, pool, logic.SourceFluid, func(symbol string, entity *logic.Entity) market.Signal {
			return &measureErrorStub{symbol: symbol, err: measureErr}
		}, logic.EntityBook)

		So(system, ShouldNotBeNil)

		t.Cleanup(func() {
			cancel()
			_ = system.Close()
		})

		tickErr := make(chan error, 1)

		go func() {
			tickErr <- system.Tick()
		}()

		updates := krakenmarket.BookUpdates{{Symbol: "BTC/USD"}}
		So(rawbus.Send(system.bus, rawbus.TypeBook, &updates), ShouldBeNil)

		Convey("Tick should halt with the measure error", func() {
			select {
			case err := <-tickErr:
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "fluid: instrument broken")
			case <-time.After(2 * time.Second):
				So("measure error", ShouldEqual, "received")
			}
		})
	})
}

func TestSystemTickContinuesDuringDeferredMeasure(t *testing.T) {
	Convey("Given a signal that is still accumulating", t, func() {
		ctx, cancel, pool := newSystemTestPool(t)

		system := NewSystem(ctx, pool, logic.SourceFluid, func(symbol string, entity *logic.Entity) market.Signal {
			return &emptyMeasurementStub{symbol: symbol}
		}, logic.EntityBook)

		So(system, ShouldNotBeNil)

		t.Cleanup(func() {
			cancel()
			_ = system.Close()
		})

		tickErr := make(chan error, 1)

		go func() {
			tickErr <- system.Tick()
		}()

		updates := krakenmarket.BookUpdates{{Symbol: "BTC/USD"}}
		So(rawbus.Send(system.bus, rawbus.TypeBook, &updates), ShouldBeNil)

		Convey("Tick should keep running while the signal defers", func() {
			time.Sleep(200 * time.Millisecond)

			select {
			case err := <-tickErr:
				So(err, ShouldBeNil)
			default:
			}
		})
	})
}

func TestSystemIgnoresUnsupportedEntity(t *testing.T) {
	Convey("Given a trade-only signal system", t, func() {
		ctx, cancel, pool := newSystemTestPool(t)
		viper.Set("signals.cvd.measurements_capacity", 4)

		recorded := make(chan any, 1)
		system := NewSystem(ctx, pool, logic.SourceCVD, func(symbol string, entity *logic.Entity) market.Signal {
			return &bookRecordStub{symbol: symbol, recorded: recorded}
		}, logic.EntityTrade)

		So(system, ShouldNotBeNil)

		t.Cleanup(func() {
			cancel()
			_ = system.Close()
		})

		go system.Tick()

		updates := krakenmarket.BookUpdates{{Symbol: "BTC/USD"}}
		So(rawbus.Send(system.bus, rawbus.TypeBook, &updates), ShouldBeNil)

		Convey("It should not record book updates", func() {
			time.Sleep(200 * time.Millisecond)

			select {
			case <-recorded:
				So("book update", ShouldEqual, "ignored")
			default:
			}
		})
	})
}

type emptyMeasurementStub struct {
	symbol string
}

func (stub *emptyMeasurementStub) Measure(*market.Feedback, time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, nil
}

func (stub *emptyMeasurementStub) Record(any) bool {
	return false
}

func (stub *emptyMeasurementStub) Symbol() string {
	return stub.symbol
}

type warmupRecordStub struct {
	symbol      string
	measurement logic.Measurement
}

func (stub *warmupRecordStub) Measure(*market.Feedback, time.Time) (logic.Measurement, error) {
	return stub.measurement, nil
}

func (stub *warmupRecordStub) Record(any) bool {
	return true
}

func (stub *warmupRecordStub) Symbol() string {
	return stub.symbol
}

type measureErrorStub struct {
	symbol string
	err    error
}

func (stub *measureErrorStub) Measure(*market.Feedback, time.Time) (logic.Measurement, error) {
	return logic.Measurement{}, stub.err
}

func (stub *measureErrorStub) Record(any) bool {
	return false
}

func (stub *measureErrorStub) Symbol() string {
	return stub.symbol
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
