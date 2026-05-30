package pumpdump

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
)

func startPumpSignalTick(t *testing.T, signal *Signal) {
	t.Helper()

	done := make(chan struct{})

	go func() {
		defer close(done)

		if err := signal.Tick(); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("pumpdump tick: %v", err)
		}
	}()

	t.Cleanup(func() {
		_ = signal.Close()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for pumpdump tick to close")
		}
	})
}

func loadPumpClassed(signal *Signal, symbol string) *numeric.Classed {
	raw, ok := signal.symbols.Load(symbol)

	if !ok {
		return nil
	}

	return raw.(*numeric.Classed)
}

func pumpCategorySet() map[perspectives.CategoryType]struct{} {
	return map[perspectives.CategoryType]struct{}{
		perspectives.CategoryVerticalIgnition:  {},
		perspectives.CategoryCoiledCompression: {},
		perspectives.CategoryOrganicTrend:      {},
		perspectives.CategoryFadedExhaustion:   {},
	}
}

func tradeBatch(
	symbol string,
	base time.Time,
	price float64,
	qty float64,
	count int,
) []market.TradeUpdate {
	trades := make([]market.TradeUpdate, count)

	for index := range count {
		trades[index] = market.TradeUpdate{
			Symbol:    symbol,
			Side:      "buy",
			Price:     price + float64(index)*0.01,
			Qty:       qty,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		}
	}

	return trades
}

func TestNewSignal(t *testing.T) {
	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		Convey("It should wire pumpdump categories", func() {
			So(signal.categories["vertical_ignition"], ShouldEqual, perspectives.CategoryVerticalIgnition)
			So(signal.categories["coiled_compression"], ShouldEqual, perspectives.CategoryCoiledCompression)
			So(signal.categories["organic_trend"], ShouldEqual, perspectives.CategoryOrganicTrend)
			So(signal.categories["faded_exhaustion"], ShouldEqual, perspectives.CategoryFadedExhaustion)
		})

		Convey("It should expose a measurements broadcast", func() {
			So(signal.broadcasts["measurements"], ShouldNotBeNil)
			So(signal.subscribers["trades"], ShouldNotBeNil)
		})
	})
}

func TestSignalTick(t *testing.T) {
	Convey("Given a running pumpdump signal", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:pumpdump", 8)
		startPumpSignalTick(t, signal)

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		Convey("When a trade batch arrives", func() {
			pool.CreateBroadcastGroup("trades", 0).Send(&qpool.QValue[any]{
				Value: tradeBatch("ALT/EUR", base, 10, 1.5, 12),
			})

			var measurement perspectives.Measurement

			select {
			case value := <-measurements.Incoming:
				var ok bool

				measurement, ok = value.Value.(perspectives.Measurement)

				So(ok, ShouldBeTrue)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for pumpdump measurement")
			}

			Convey("It should publish a pumpdump measurement", func() {
				So(measurement.Source, ShouldEqual, perspectives.SourcePumpDump)
				_, ok := pumpCategorySet()[measurement.Category]
				So(ok, ShouldBeTrue)
				So(measurement.Confidence, ShouldBeGreaterThan, 0)
				So(measurement.SNR, ShouldBeGreaterThan, 0)
			})

			Convey("It should create per-symbol pipeline state", func() {
				So(loadPumpClassed(signal, "ALT/EUR"), ShouldNotBeNil)
			})
		})

		Convey("When the payload is not a trade batch", func() {
			pool.CreateBroadcastGroup("trades", 0).Send(&qpool.QValue[any]{
				Value: "not-trades",
			})

			select {
			case value := <-measurements.Incoming:
				t.Fatalf("expected no measurement, got %v", value.Value)
			case <-time.After(100 * time.Millisecond):
			}
		})
	})
}

func BenchmarkSignalTick(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = signal.Tick()
	}()

	defer func() {
		_ = signal.Close()

		select {
		case <-done:
		case <-time.After(time.Second):
			b.Fatal("timed out waiting for pumpdump tick to close")
		}
	}()

	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	payload := &qpool.QValue[any]{
		Value: tradeBatch("ALT/EUR", base, 10, 1.5, 16),
	}

	b.ReportAllocs()

	for b.Loop() {
		signal.subscribers["trades"].Incoming <- payload
	}
}
