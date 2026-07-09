package trader

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"

	. "github.com/smartystreets/goconvey/convey"
)

// plannerSocket implements websocket.PublicSocket for test usage.
type plannerSocket struct {
	channels map[string]chan []byte
}

func (sock *plannerSocket) Observe(channel string) chan []byte {
	if sock.channels == nil {
		sock.channels = make(map[string]chan []byte)
	}

	if _, ok := sock.channels[channel]; !ok {
		sock.channels[channel] = make(chan []byte, 8)
	}

	return sock.channels[channel]
}

func (sock *plannerSocket) Ticker(_ []string) (kraken.TickerDataSlice, error) {
	return kraken.TickerDataSlice{}, nil
}

// plannerPrivate implements websocket.Private for test usage.
type plannerPrivate struct {
	channels map[string]chan []byte
}

func (priv *plannerPrivate) Observe(channel string) chan []byte {
	if priv.channels == nil {
		priv.channels = make(map[string]chan []byte)
	}

	if _, ok := priv.channels[channel]; !ok {
		priv.channels[channel] = make(chan []byte, 8)
	}

	return priv.channels[channel]
}

func (priv *plannerPrivate) Submit(_ *kraken.Order) error {
	return nil
}

func (priv *plannerPrivate) TradeVolume(_ []string) (websocket.FeeSchedule, error) {
	return websocket.FeeSchedule{
		Fallback: websocket.FeeRates{Taker: 0.002, Maker: 0.001},
		Pairs:    map[string]websocket.FeeRates{},
	}, nil
}

func (priv *plannerPrivate) Close() {}

func TestCryptoRun(testingTB *testing.T) {
	Convey("Given Crypto with a ready ticker measurement path", testingTB, func() {
		previousFraction := viper.GetFloat64("trading.sizing.base_fraction")
		viper.Set("trading.sizing.base_fraction", 0.05)
		defer viper.Set("trading.sizing.base_fraction", previousFraction)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		deskPublic := &plannerSocket{}
		deskPrivate := &plannerPrivate{}
		desk, err := broker.NewDesk(
			ctx,
			deskPublic,
			deskPrivate,
			make(chan []byte, 8),
		)
		So(err, ShouldBeNil)

		price := broker.NewPrice(ctx, &plannerSocket{}, &plannerPrivate{})
		defer price.Close()

		tickerChannel := make(chan []byte, 1)
		uiHub := &ui.Hub{Messages: make(chan []byte, 8)}
		crypto := &Crypto{
			ctx:    ctx,
			cancel: cancel,
			channels: map[string]chan []byte{
				channelTicker: tickerChannel,
			},
			uiHub:    uiHub,
			desk:     desk,
			ticker:   NewTicker(nil, nil),
			tick:     &atomic.Int64{},
			spreads:  &sync.Map{},
			planner:  NewPlanner(desk, price),
			analyzer: logic.NewAnalyzer(nil, nil, uiHub),
		}
		crypto.status.Store(types.READY)

		Convey("When a ticker event is measured", func() {
			err := crypto.Run()
			So(err, ShouldBeNil)

			tickerChannel <- []byte(`[{
				"symbol": "MANA/USD",
				"bid": "0.068",
				"ask": "0.069",
				"last": "0.0685"
			}]`)

			Convey("Then Crypto publishes the tick frame", func() {
				select {
				case msg := <-crypto.uiHub.Messages:
					So(bytes.Contains(msg, []byte(`"tick"`)), ShouldBeTrue)
					So(bytes.Contains(msg, []byte(`"count":1`)), ShouldBeTrue)
				case <-time.After(time.Second):
					testingTB.Fatal("crypto did not publish a tick frame")
				}
			})
		})
	})
}