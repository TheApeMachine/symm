package trader

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeSocket struct {
	channels map[string]chan []byte
}

func newFakeSocket() *fakeSocket {
	return &fakeSocket{
		channels: map[string]chan []byte{},
	}
}

func (socket *fakeSocket) Observe(channel string) chan []byte {
	out := make(chan []byte, 4)
	socket.channels[channel] = out
	return out
}

func TestCryptoRun(t *testing.T) {
	Convey("Given a Crypto runtime observing market frames", t, func() {
		previousDepth := viper.GetInt("signals.feed_ring_capacity")
		previousFraction := viper.GetFloat64("trading.sizing.base_fraction")
		viper.Set("signals.feed_ring_capacity", 8)
		viper.Set("trading.sizing.base_fraction", 0.1)
		defer viper.Set("signals.feed_ring_capacity", previousDepth)
		defer viper.Set("trading.sizing.base_fraction", previousFraction)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 1, nil)
		defer pool.Close()

		publicSocket := newFakeSocket()
		level3Socket := newFakeSocket()
		crypto, err := NewCrypto(ctx, pool, dmt.NewTree(""), publicSocket, level3Socket)
		So(err, ShouldBeNil)
		defer crypto.Close()

		runErr := make(chan error, 1)
		go func() {
			runErr <- crypto.Run()
		}()

		Convey("When public and level3 frames arrive", func() {
			publicSocket.channels[channelTicker] <- []byte(`[{
				"symbol": "BTC/USD",
				"bid": 99,
				"ask": 101,
				"last": 100,
				"timestamp": "2026-07-04T12:00:00Z"
			}]`)
			publicSocket.channels[channelTrade] <- []byte(`[{
				"symbol": "MATIC/USD",
				"side": "buy",
				"price": 0.5147,
				"qty": 6423.46326,
				"ord_type": "limit",
				"trade_id": 4665846,
				"timestamp": "2026-07-04T12:00:01Z"
			}]`)
			publicSocket.channels[channelOHLC] <- []byte(`[{
				"symbol": "ALGO/USD",
				"open": 0.09875,
				"high": 0.0988,
				"low": 0.09875,
				"close": 0.09875,
				"trades": 13,
				"volume": 16255.46368,
				"vwap": 0.09879,
				"interval_begin": "2026-07-04T11:55:00Z",
				"interval": 5,
				"timestamp": "2026-07-04T12:00:00Z"
			}]`)
			publicSocket.channels[channelBook] <- []byte(`[{
				"symbol": "MATIC/USD",
				"bids": [{"price": 0.5666, "qty": 4831.75496356}],
				"asks": [{"price": 0.5668, "qty": 4410.79769741}],
				"checksum": 2439117997,
				"timestamp": "2026-07-04T12:00:02Z"
			}]`)
			level3Socket.channels[channelLevel3] <- []byte(`[{
				"symbol": "BTC/USD",
				"timestamp": "2026-07-04T12:00:03Z",
				"checksum": 291736120,
				"bids": [{
					"event": "add",
					"order_id": "OQCLML-BW3P3-BUCMWZ",
					"limit_price": 43125.3,
					"order_qty": 0.15,
					"timestamp": "2026-07-04T12:00:03Z"
				}],
				"asks": []
			}]`)

			observed := false
			timedOut := false
			deadline := time.After(time.Second)
			for !observed && !timedOut {
				_, tickerOK := crypto.ticker.history.cache.Load("BTC/USD")
				_, tradeOK := crypto.trade.history.cache.Load("MATIC/USD")
				_, ohlcOK := crypto.ohlc.history.cache.Load("ALGO/USD")
				_, bookOK := crypto.book.history.cache.Load("MATIC/USD")
				_, level3OK := crypto.level3.history.cache.Load("BTC/USD")

				if tickerOK && tradeOK && ohlcOK && bookOK && level3OK {
					observed = true
					break
				}

				select {
				case <-deadline:
					timedOut = true
				default:
					time.Sleep(time.Millisecond)
				}
			}

			Convey("It should measure each entity history and stop on context cancellation", func() {
				So(observed, ShouldBeTrue)

				cancel()

				select {
				case err := <-runErr:
					So(err, ShouldBeNil)
				case <-time.After(time.Second):
					t.Fatal("crypto did not stop")
				}
			})
		})
	})
}
