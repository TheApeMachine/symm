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
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"

	. "github.com/smartystreets/goconvey/convey"
)

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
