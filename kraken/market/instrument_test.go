package market

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/public"
)

func TestInstrumentTick(t *testing.T) {
	Convey("Given an instrument snapshot on the shared raw bus as *SocketMessage", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		viper.Set("market.quote_currency", "EUR")
		viper.Set("market.max_scan_symbols", 8)
		viper.Set("market.book_depth_levels", 10)
		viper.Set("market.subscribe_pace", time.Millisecond)
		viper.Set("market.default_symbols", []string{"BTC/EUR"})
		viper.Set("market.anchor_symbol", "BTC/EUR")

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		defer pool.Close()

		raw := bus.Group(pool, "raw", 10*time.Millisecond)
		publicOut := bus.Group(pool, "kraken:public", 10*time.Millisecond)
		publicSubscriber := publicOut.Subscribe("test:instrument", 16)

		instrument := NewInstrument(ctx, pool)

		tickDone := make(chan struct{})

		go func() {
			_ = instrument.Tick()
			close(tickDone)
		}()

		payload, err := json.Marshal(InstrumentUpdate{
			Pairs: []InstrumentPair{
				{
					Symbol:         "BTC/EUR",
					Base:           "BTC",
					Quote:          "EUR",
					Status:         "online",
					QtyPrecision:   8,
					QtyIncrement:   0.00000001,
					PricePrecision: 1,
					CostPrecision:  5,
					PriceIncrement: 0.1,
					QtyMin:         0.00005,
				},
				{
					Symbol: "ETH/USD",
					Base:   "ETH",
					Quote:  "USD",
					Status: "online",
				},
				{
					Symbol: "XRP/EUR",
					Base:   "XRP",
					Quote:  "EUR",
					Status: "cancel_only",
				},
			},
		})

		So(err, ShouldBeNil)

		raw.Send(&qpool.QValue[any]{
			Value: &public.SocketMessage{
				Channel: "instrument",
				Type:    "snapshot",
				Data:    payload,
			},
		})

		Convey("It should emit book subscribe only for tradable EUR pairs", func() {
			waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
			defer waitCancel()

			bookSubscribe := false

			for !bookSubscribe {
				message, err := bus.PollFor(waitCtx, publicSubscriber)
				if err != nil {
					So("timeout waiting for book subscribe", ShouldBeEmpty)
					return
				}

				frame, ok := message.Value.(map[string]any)

				if !ok {
					continue
				}

				params, _ := frame["params"].(map[string]any)

				if params["channel"] == "book" {
					bookSubscribe = true
				}
			}

			So(bookSubscribe, ShouldBeTrue)
			time.Sleep(20 * time.Millisecond)
			So(instrument.Pairs, ShouldResemble, []string{"BTC/EUR"})
		})

		cancel()

		select {
		case <-tickDone:
		case <-time.After(2 * time.Second):
		}
	})
}

func TestInstrumentReplaySubscriptions(t *testing.T) {
	Convey("Given an instrument with active pairs", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		viper.Set("market.book_depth_levels", 10)
		viper.Set("market.subscribe_pace", time.Millisecond)

		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		defer pool.Close()

		publicOut := bus.Group(pool, "kraken:public", 10*time.Millisecond)
		publicSubscriber := publicOut.Subscribe("test:replay", 32)

		instrument := NewInstrument(ctx, pool)
		instrument.Pairs = []string{"BTC/EUR"}

		Convey("When replaySubscriptions runs after reconnect", func() {
			instrument.replaySubscriptions()

			waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
			defer waitCancel()

			channels := map[string]bool{}

			for len(channels) < 4 {
				message, err := bus.PollFor(waitCtx, publicSubscriber)
				if err != nil {
					So("timeout waiting for replay subscribe frames", ShouldBeEmpty)
					return
				}

				frame, ok := message.Value.(map[string]any)

				if !ok {
					continue
				}

				params, _ := frame["params"].(map[string]any)
				channel, _ := params["channel"].(string)

				if channel != "" {
					channels[channel] = true
				}
			}

			So(channels["ticker"], ShouldBeTrue)
			So(channels["book"], ShouldBeTrue)
			So(channels["ohlc"], ShouldBeTrue)
			So(channels["trade"], ShouldBeTrue)
		})
	})
}
