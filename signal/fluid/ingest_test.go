package fluid

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestPublishIngestAppliesTicker(t *testing.T) {
	Convey("Given a fluid system with disruptor ingest", t, func() {
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		viper.Set("signals.fluid.measurements_capacity", 64)
		viper.Set("signals.fluid.tick_size", 0.01)
		viper.Set("signals.fluid.grid_half_width", 10)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		system := NewSystem(ctx, pool)

		So(system, ShouldNotBeNil)
		defer system.Close()

		ticker := &krakenmarket.TickerUpdate{
			Symbol:    "BTC/EUR",
			Last:      100,
			Volume:    288,
			ChangePct: 1.2,
			Bid:       99,
			Ask:       101,
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		}

		tickerAt, parseErr := krakenmarket.EventTimeFromTicker(ticker)
		So(parseErr, ShouldBeNil)

		feedErr := system.feedTicker(ticker)

		Convey("It should publish through the ingest queue without error", func() {
			So(feedErr, ShouldBeNil)
		})

		Convey("It should apply ticker fields through ingest", func() {
			applyErr := system.applyIngest(ingestEvent{
				symbol:   "BTC/EUR",
				kind:     ingestTicker,
				ticker:   *ticker,
				tickerAt: tickerAt,
			})

			So(applyErr, ShouldBeNil)

			state := system.loadSymbol("BTC/EUR")

			So(state, ShouldNotBeNil)
			So(state.last, ShouldEqual, 100)
			So(state.changePct, ShouldEqual, 1.2)
		})
	})
}

func BenchmarkPublishIngestTicker(b *testing.B) {
	viper.Set("market.book_depth_levels", 10)
	viper.Set("signals.volume_clock_bars_per_day", 288)
	viper.Set("signals.fluid.measurements_capacity", 64)
	viper.Set("signals.fluid.tick_size", 0.01)
	viper.Set("signals.fluid.grid_half_width", 10)
	viper.Set("telemetry.gauge.readings_capacity", 1024)

	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 4, nil)
	system := NewSystem(ctx, pool)

	if system == nil {
		b.Fatal("fluid system unavailable")
	}

	defer system.Close()

	ticker := &krakenmarket.TickerUpdate{
		Symbol:    "BTC/EUR",
		Last:      100,
		Volume:    288,
		ChangePct: 1.2,
		Bid:       99,
		Ask:       101,
		Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := system.feedTicker(ticker); err != nil {
			b.Fatal(err)
		}
	}
}
