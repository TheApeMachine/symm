package fluid

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestSystemPublishFieldSnapshot(t *testing.T) {
	Convey("Given a fluid system with one symbol field row", t, func() {
		viper.Set("market.book_depth_levels", 10)
		viper.Set("signals.volume_clock_bars_per_day", 288)
		viper.Set("signals.fluid.measurements_capacity", 16)
		viper.Set("signals.fluid.tick_size", 0.01)
		viper.Set("signals.fluid.grid_half_width", 10)
		viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
		feedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		defer pool.Close()
		system := NewSystem(ctx, pool)

		So(system, ShouldNotBeNil)

		uiBus := internal.NewBus(ctx, pool, nil, []string{"ui"})
		state := system.loadSymbol("ETH/EUR")
		fixture := symbolBookFixture{symbol: "ETH/EUR"}

		So(state.FeedTicker(krakenmarket.TickerUpdate{
			Symbol: "ETH/EUR",
			Last:   100,
			Bid:    99,
			Ask:    101,
			Volume: 1000,
		}, feedAt), ShouldBeNil)
		So(advanceFluidGrid(state, fixture, feedAt, 99, 10, 101, 6), ShouldBeNil)

		Convey("It should publish a fluid field snapshot on the ui bus", func() {
			received := make(chan map[string]any, 1)

			go func() {
				for {
					row, receiveErr := uiBus.Receive("ui")

					if receiveErr != nil {
						return
					}

					if row == nil {
						continue
					}

					frame, ok := row.Value.(map[string]any)

					if !ok || frame["type"] != "fluid" {
						continue
					}

					received <- frame
					return
				}
			}()

			err := system.publishFieldSnapshot(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
			So(err, ShouldBeNil)

			var frame map[string]any

			select {
			case frame = <-received:
			case <-time.After(2 * time.Second):
				So("ui field_snapshot", ShouldEqual, "received")
			}

			So(frame["type"], ShouldEqual, "fluid")
			So(frame["symbol_count"], ShouldEqual, 1)

			rows, ok := frame["symbols"].([]map[string]any)

			if !ok {
				rawRows, rawOk := frame["symbols"].([]any)

				So(rawOk, ShouldBeTrue)
				So(len(rawRows), ShouldEqual, 1)

				first, firstOk := rawRows[0].(map[string]any)
				So(firstOk, ShouldBeTrue)
				So(first["symbol"], ShouldEqual, "ETH/EUR")

				return
			}

			So(len(rows), ShouldEqual, 1)
			So(rows[0]["symbol"], ShouldEqual, "ETH/EUR")
		})
	})
}
