package trader

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
)

func TestNewCryptoRegistersFuturesChannel(t *testing.T) {
	convey.Convey("Given a crypto trader bus", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 8, nil)
		crypto := NewCrypto(ctx, pool)

		defer crypto.cancel()

		convey.Convey("It should accept kraken:futures publish", func() {
			err := crypto.bus.Send(internal.ChannelKrakenFutures, "book", map[string]string{"event": "subscribe"})
			convey.So(err, convey.ShouldBeNil)
		})
	})
}

func TestCryptoResetSubscriptions(t *testing.T) {
	convey.Convey("Given a crypto trader with active subscriptions", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 8, nil)
		crypto := NewCrypto(ctx, pool)

		defer crypto.cancel()

		crypto.instrument.pairs.Store("BTC/USD", true)
		crypto.instrument.candles.Store("BTC/USD", true)
		crypto.instrument.anchorSubscribed.Store(true)

		convey.Convey("It should clear pair and anchor state on reset", func() {
			crypto.reset()

			_, subscribed := crypto.instrument.pairs.Load("BTC/USD")
			_, candleSubscribed := crypto.instrument.candles.Load("BTC/USD")
			convey.So(subscribed, convey.ShouldBeFalse)
			convey.So(candleSubscribed, convey.ShouldBeFalse)
			convey.So(crypto.instrument.anchorSubscribed.Load(), convey.ShouldBeFalse)
		})
	})
}

func TestInstrumentSubscribeMarketFeeds(t *testing.T) {
	convey.Convey("Given a new online pair", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 16, nil)
		subscriber := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelKrakenPublic, internal.ChannelRaw},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelKrakenPublic, "market-feed-test"),
				internal.Subscribe(internal.ChannelRaw, "market-feed-raw"),
			},
		)
		instrument := NewInstrument(ctx, internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelKrakenPublic, internal.ChannelRaw},
			nil,
		))

		defer instrument.cancel()

		update := &market.InstrumentUpdate{
			Pairs: []market.InstrumentPair{
				{Symbol: "CELO/USD", Quote: "USD", Status: "online"},
			},
		}

		convey.Convey("It should subscribe ticker for bbo and trades plus book and trade", func() {
			convey.So(
				instrument.Tick(&qpool.QValue[any]{
					Type:  "instrument",
					Value: update,
				}),
				convey.ShouldBeNil,
			)

			_, _ = subscriber.Receive(internal.ChannelRaw)

			tickerTriggers := make([]string, 0, 2)
			sawBook := false
			sawTrade := false

			for attempt := 0; attempt < 8 && (len(tickerTriggers) < 2 || !sawBook || !sawTrade); attempt++ {
				frame, receiveErr := subscriber.Receive(internal.ChannelKrakenPublic)
				convey.So(receiveErr, convey.ShouldBeNil)

				switch frame.Type {
				case "ohlc":
					continue
				case "ticker":
					message, ok := frame.Value.(types.KrakenMessage)
					convey.So(ok, convey.ShouldBeTrue)

					var params market.TickerParams
					convey.So(json.Unmarshal(message.Params.(json.RawMessage), &params), convey.ShouldBeNil)
					convey.So(params.Symbol, convey.ShouldResemble, []string{"CELO/USD"})
					convey.So(params.Snapshot, convey.ShouldBeTrue)
					tickerTriggers = append(tickerTriggers, params.EventTrigger)
				case "book":
					sawBook = true
				case "trade":
					sawTrade = true
				default:
					convey.So(frame.Type, convey.ShouldEqual, "ticker")
				}
			}

			convey.So(tickerTriggers, convey.ShouldContain, market.TickerTriggerBBO)
			convey.So(tickerTriggers, convey.ShouldContain, market.TickerTriggerTrades)
			convey.So(sawBook, convey.ShouldBeTrue)
			convey.So(sawTrade, convey.ShouldBeTrue)
		})
	})
}

func TestInstrumentSubscribePositionCandles(t *testing.T) {
	convey.Convey("Given a balance with an open position", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 8, nil)
		subscriber := internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelKrakenPublic},
			[]internal.Subscription{
				internal.Subscribe(
					internal.ChannelKrakenPublic,
					"position-candle-test",
				),
			},
		)
		instrument := NewInstrument(ctx, internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelKrakenPublic},
			nil,
		))

		defer instrument.cancel()

		balances := user.Balances{
			Currency: "USD",
			Inventory: map[string]float64{
				"LTC": 0.5,
			},
		}

		convey.Convey("It should subscribe the position symbol to OHLC", func() {
			convey.So(
				instrument.SubscribePositionCandles(balances),
				convey.ShouldBeNil,
			)

			frame, receiveErr := subscriber.Receive(internal.ChannelKrakenPublic)
			convey.So(receiveErr, convey.ShouldBeNil)
			convey.So(frame.Type, convey.ShouldEqual, "ohlc")

			message, ok := frame.Value.(types.KrakenMessage)
			convey.So(ok, convey.ShouldBeTrue)
			params, paramsOK := message.Params.(market.CandleParams)
			convey.So(paramsOK, convey.ShouldBeTrue)
			convey.So(params.Symbol, convey.ShouldResemble, []string{"LTC/USD"})
			convey.So(params.Interval, convey.ShouldEqual, 1)
			convey.So(params.Snapshot, convey.ShouldBeTrue)
		})
	})
}

func BenchmarkInstrumentPositionSymbols(benchmark *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 2, 8, nil)
	instrument := NewInstrument(ctx, internal.NewBus(
		ctx,
		pool,
		[]internal.Channel{internal.ChannelKrakenPublic},
		nil,
	))
	balances := user.Balances{
		Currency: "USD",
		Inventory: map[string]float64{
			"CELO": 660,
			"LTC":  0.9,
			"RIZE": 11743,
			"XLM":  105,
		},
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_ = instrument.positionSymbols(balances)
	}
}
