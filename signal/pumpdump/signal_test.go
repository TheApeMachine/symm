package pumpdump

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func withPumpdumpConfig(t *testing.T) {
	t.Helper()
	viper.Set("signals.pumpdump.window", time.Minute)
}

func loadPumpState(signal *Signal, symbol string) *pumpState {
	raw, ok := signal.symbols.Load(symbol)

	if !ok {
		return nil
	}

	return raw.(*pumpState)
}

func pumpCategorySet() map[types.CategoryType]struct{} {
	return map[types.CategoryType]struct{}{
		types.CategoryVerticalIgnition:  {},
		types.CategoryCoiledCompression: {},
		types.CategoryOrganicTrend:      {},
		types.CategoryFadedExhaustion:   {},
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
			Qty:       qty + float64(index)*0.25,
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
		}
	}

	return trades
}

func drainMeasurements(measurements *qpool.Subscriber) []types.Measurement {
	published := make([]types.Measurement, 0)

	for {
		select {
		case value := <-measurements.Incoming:
			reading, ok := value.Value.(types.Measurement)

			if ok {
				published = append(published, reading)
			}
		default:
			return published
		}
	}
}

func TestNewSignal(t *testing.T) {
	withPumpdumpConfig(t)

	Convey("Given a qpool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		Convey("It should wire pumpdump categories", func() {
			So(signal.categories["vertical_ignition"], ShouldEqual, types.CategoryVerticalIgnition)
			So(signal.categories["coiled_compression"], ShouldEqual, types.CategoryCoiledCompression)
			So(signal.categories["organic_trend"], ShouldEqual, types.CategoryOrganicTrend)
			So(signal.categories["faded_exhaustion"], ShouldEqual, types.CategoryFadedExhaustion)
		})

		Convey("It should expose a measurements broadcast", func() {
			So(signal.broadcasts["measurements"], ShouldNotBeNil)
		})
	})
}

func TestObserve(t *testing.T) {
	withPumpdumpConfig(t)

	Convey("Given a pumpdump signal with a measurements subscriber", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		signal := NewSignal(ctx, pool)
		defer signal.Close()

		measurements := signal.broadcasts["measurements"].Subscribe("test:pumpdump", 64)
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		Convey("When baselines are unobserved", func() {
			err := signal.observe(market.TradeUpdate{
				Symbol:    "ALT/EUR",
				Side:      "buy",
				Price:     10,
				Qty:       1.5,
				Timestamp: base,
			})

			Convey("It should fail without publishing", func() {
				So(errors.Is(err, errBaselineUnobserved), ShouldBeTrue)

				select {
				case <-measurements.Incoming:
					t.Fatal("unexpected measurement during warmup")
				default:
				}
			})
		})

		Convey("When a developing volume lift is folded in trade by trade", func() {
			for _, trade := range tradeBatch("ALT/EUR", base, 10, 1.5, 24) {
				err := signal.observe(trade)

				if err != nil && isWarmup(err) {
					continue
				}

				So(err, ShouldBeNil)
			}

			published := drainMeasurements(measurements)

			Convey("It publishes strength as the fused observation", func() {
				So(published, ShouldNotBeEmpty)

				measurement := published[len(published)-1]
				So(measurement.Source, ShouldEqual, types.SourcePumpDump)
				So(measurement.Symbol, ShouldEqual, "ALT/EUR")
				So(measurement.Last, ShouldBeGreaterThan, 0)
				So(measurement.At, ShouldEqual, base.Add(23*time.Millisecond))
				So(measurement.Strength, ShouldNotEqual, 0)
				So(measurement.Confidence, ShouldBeGreaterThanOrEqualTo, 0)
				So(measurement.Confidence, ShouldBeLessThanOrEqualTo, 1)
				So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)

				state := loadPumpState(signal, "ALT/EUR")
				So(measurement.Strength, ShouldEqual, state.pipe.Observation())

				_, known := pumpCategorySet()[measurement.Category]
				So(known, ShouldBeTrue)
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

		Convey("When lift fades after an impulse", func() {
			fadeSignal := NewSignal(ctx, pool)
			defer fadeSignal.Close()

			fadeMeasurements := fadeSignal.broadcasts["measurements"].Subscribe("test:pumpdump-fade", 64)

			for _, trade := range tradeBatch("ALT/EUR", base, 10, 2, 24) {
				_ = fadeSignal.observe(trade)
			}

			_ = fadeSignal.observe(market.TradeUpdate{
				Symbol:    "ALT/EUR",
				Side:      "buy",
				Price:     14,
				Qty:       40,
				Timestamp: base.Add(30 * time.Millisecond),
			})
			_ = fadeSignal.observe(market.TradeUpdate{
				Symbol:    "ALT/EUR",
				Side:      "buy",
				Price:     15,
				Qty:       60,
				Timestamp: base.Add(31 * time.Millisecond),
			})

			for index := range 16 {
				err := fadeSignal.observe(market.TradeUpdate{
					Symbol:    "ALT/EUR",
					Side:      "sell",
					Price:     15 - float64(index)*0.05,
					Qty:       1.2,
					Timestamp: base.Add(time.Duration(40+index) * time.Millisecond),
				})

				if err != nil && isWarmup(err) {
					continue
				}

				So(err, ShouldBeNil)
			}

			published := drainMeasurements(fadeMeasurements)
			state := loadPumpState(fadeSignal, "ALT/EUR")
			code, err := fadeSignal.classifier.Code(state.pipe.Observation())

			Convey("It should publish faded exhaustion end to end", func() {
				So(published, ShouldNotBeEmpty)
				So(err, ShouldBeNil)
				So(
					fadeSignal.categories[fadeSignal.classifier.Label(code)],
					ShouldEqual,
					types.CategoryFadedExhaustion,
				)

				faded := false

				for _, measurement := range published {
					if measurement.Category == types.CategoryFadedExhaustion {
						faded = true
					}
				}

				So(faded, ShouldBeTrue)
				So(published[len(published)-1].Strength, ShouldEqual, state.pipe.Observation())
			})
		})
	})
}

func BenchmarkObserve(b *testing.B) {
	withPumpdumpConfig(&testing.T{})

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	defer pool.Close()

	signal := NewSignal(ctx, pool)
	defer signal.Close()

	signal.broadcasts["measurements"].Subscribe("bench:pumpdump", 1024)
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	trade := market.TradeUpdate{Symbol: "ALT/EUR", Side: "buy", Price: 10, Qty: 1.5, Timestamp: base}

	for b.Loop() {
		_ = signal.observe(trade)
	}
}
