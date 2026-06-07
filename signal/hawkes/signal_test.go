package hawkes

import (
	"context"

	"github.com/theapemachine/symm/bus"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func tradeBurst(symbol string, base time.Time, count int) []market.TradeUpdate {
	trades := make([]market.TradeUpdate, count)

	for index := range count {
		side := "buy"

		if index%2 == 0 {
			side = "sell"
		}

		trades[index] = market.TradeUpdate{
			Symbol:    symbol,
			Side:      side,
			Price:     100 + float64(index)*0.01,
			Qty:       1.5 + float64(index%5)*0.1,
			Timestamp: base.Add(time.Duration(index) * 100 * time.Millisecond),
		}
	}

	return trades
}

func TestNewSignal(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		Convey("It should expose a measurements broadcast", func() {
			So(signal.broadcasts["measurements"], ShouldNotBeNil)
		})
	})
}

func TestMeasure(t *testing.T) {
	Convey("Given a Hawkes symbol with a clustered buy burst", t, func() {
		symbol := NewHawkesSymbol(nil, hawkesTestCategories())
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		ticks := tradeBurst("ALT/EUR", base, 128)
		now := base.Add(128 * 100 * time.Millisecond)

		Convey("When enough arrivals exist to fit", func() {
			measurement, _, err := symbol.Measure(ticks, now)

			Convey("It should publish a thermal perspective reading", func() {
				So(err, ShouldBeNil)
				So(measurement.Source, ShouldEqual, types.SourceHawkes)
				So(measurement.Strength, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestObserveTrades(t *testing.T) {
	Convey("Given a hawkes signal with a measurements subscriber", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:hawkes", 64)
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		trades := tradeBurst("ALT/EUR", base, 128)

		Convey("When a multi-print burst is observed", func() {
			err := signal.observeTrades(trades)

			So(err, ShouldBeNil)

			waitCtx, waitCancel := context.WithTimeout(ctx, time.Second)
			defer waitCancel()

			value, err := bus.PollFor(waitCtx, measurements)
			if err != nil {
				t.Fatal("timed out waiting for hawkes measurement")
			}

			measurement, _ := value.Value.(types.Measurement)

			Convey("It publishes one thermal reading for the symbol", func() {
				So(measurement.Source, ShouldEqual, types.SourceHawkes)
				So(measurement.Symbol, ShouldEqual, "ALT/EUR")
				So(measurement.Strength, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func BenchmarkObserveTrades(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer signal.Close()

	signal.broadcasts["measurements"].Subscribe("bench:hawkes", 1024)

	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	trades := tradeBurst("ALT/EUR", base, 128)

	if err := signal.observeTrades(trades); err != nil {
		b.Fatal(err)
	}

	touches := append([]tradeTouch(nil), signal.tradeScratch...)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := signal.publishTouches(touches); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMeasure(b *testing.B) {
	symbol := NewHawkesSymbol(nil, hawkesTestCategories())
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	ticks := tradeBurst("ALT/EUR", base, 128)
	now := base.Add(128 * 100 * time.Millisecond)

	b.ReportAllocs()

	for b.Loop() {
		_, _, _ = symbol.Measure(ticks, now)
	}
}
