package trader

import (
	"context"
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
