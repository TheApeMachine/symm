package fluid

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	. "github.com/theapemachine/symm/signal"
)

func TestSignalFieldSnapshot(testingTB *testing.T) {
	Convey("Given a fluid signal with one symbol field row", testingTB, func() {
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		viper.Set("signals.fluid.tick_size", 0.01)
		viper.Set("signals.fluid.grid_half_width", 10)
		viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
		symbolConfigValue.Store(nil)

		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)

		signal := NewSignal(ctx, pool, NewTestTree())
		defer func() {
			_ = signal.Close()
		}()
		So(signal, ShouldNotBeNil)

		state := signal.registry.loadSymbol("ETH/EUR")
		fixture := symbolBookFixture{symbol: "ETH/EUR"}

		So(state.FeedTicker(TickerUpdate{
			Symbol: "ETH/EUR", Last: 100, Bid: 99.99, Ask: 100.01, Volume: 1000,
		}, feedAt), ShouldBeNil)

		So(state.FeedBook(fixture.snapshot(99.99, 5, 100.01, 5), feedAt), ShouldBeNil)
		So(state.FeedBook(
			fixture.snapshot(99.99, 8, 100.01, 8),
			feedAt.Add(100*time.Millisecond),
		), ShouldBeNil)
		So(state.FeedBook(
			fixture.snapshot(100.01, 8, 100.03, 8),
			feedAt.Add(200*time.Millisecond),
		), ShouldBeNil)

		Convey("It should build a fluid field snapshot payload", func() {
			So(state.Row(), ShouldNotBeNil)

			payload, err := signal.FieldSnapshot(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
			So(err, ShouldBeNil)
			So(payload["type"], ShouldEqual, "fluid")
			So(payload["symbol_count"], ShouldEqual, 1)

			rows, ok := payload["symbols"].([]map[string]any)

			if !ok {
				rawRows, rawOk := payload["symbols"].([]any)

				So(rawOk, ShouldBeTrue)
				So(len(rawRows), ShouldEqual, 1)

				first, firstOk := rawRows[0].(map[string]any)
				So(firstOk, ShouldBeTrue)
				So(first["symbol"], ShouldEqual, "ETH/EUR")
				So(first["vort"], ShouldNotBeNil)
				So(first["turb"], ShouldNotBeNil)

				return
			}

			So(len(rows), ShouldEqual, 1)
			So(rows[0]["symbol"], ShouldEqual, "ETH/EUR")
			So(rows[0]["vort"], ShouldNotBeNil)
			So(rows[0]["turb"], ShouldNotBeNil)
		})
	})
}
