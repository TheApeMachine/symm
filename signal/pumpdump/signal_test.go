package pumpdump

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func loadPumpState(signal *Signal, symbol string) *pumpState {
	raw, ok := signal.symbols.Load(symbol)

	if !ok {
		return nil
	}

	return raw.(*pumpState)
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
			Qty:       qty + float64(index)*0.25, // rising size: a developing lift
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
		})
	})
}

func TestObserve(t *testing.T) {
	Convey("Given a pumpdump signal with a measurements subscriber", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:pumpdump", 64)
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		Convey("When a developing volume lift is folded in trade by trade", func() {
			for _, trade := range tradeBatch("ALT/EUR", base, 10, 1.5, 24) {
				signal.observe(trade)
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
					t.Fatal("timed out waiting for pumpdump measurement")
				}
			}

			Convey("It publishes an ignition measurement carrying symbol and price", func() {
				So(measurement.Source, ShouldEqual, perspectives.SourcePumpDump)
				So(measurement.Symbol, ShouldEqual, "ALT/EUR")
				So(measurement.Last, ShouldBeGreaterThan, 0)

				_, known := pumpCategorySet()[measurement.Category]
				So(known, ShouldBeTrue)
				So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
			})

			Convey("It creates per-symbol pipeline state", func() {
				So(loadPumpState(signal, "ALT/EUR"), ShouldNotBeNil)
			})
		})

		Convey("When a trade has no price or size", func() {
			signal.observe(market.TradeUpdate{Symbol: "ALT/EUR", Side: "buy", Timestamp: base})

			Convey("It is ignored and creates no state", func() {
				So(loadPumpState(signal, "ALT/EUR"), ShouldBeNil)
			})
		})
	})
}

func BenchmarkObserve(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer signal.Close()

	signal.broadcasts["measurements"].Subscribe("bench:pumpdump", 1024)
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	trade := market.TradeUpdate{Symbol: "ALT/EUR", Side: "buy", Price: 10, Qty: 1.5, Timestamp: base}

	b.ReportAllocs()

	for b.Loop() {
		signal.observe(trade)
	}
}
