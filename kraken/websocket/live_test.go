package websocket

import (
	"context"
	"sync"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLiveOn(t *testing.T) {
	Convey("Given a live private transport with embedded paper", t, func() {
		previousModel := viper.GetString("trading.model")
		viper.Set("trading.model", "paper")
		defer viper.Set("trading.model", previousModel)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		paper := paperFixture(t)
		live := &Live{
			ctx:    ctx,
			cancel: cancel,
			sync:   &sync.Map{},
			paper:  paper,
		}

		balances := &frameCapture{}
		live.paper.On("balances", balances.record)

		Convey("Then wallet channels are served by paper", func() {
			So(balances.count(), ShouldBeGreaterThan, 0)
		})
	})
}

func TestLiveWrite(t *testing.T) {
	Convey("Given a live private transport with embedded paper", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		paper := paperFixture(t)
		live := &Live{
			ctx:    ctx,
			cancel: cancel,
			sync:   &sync.Map{},
			paper:  paper,
		}

		orderResponses := &frameCapture{}
		live.paper.On("add_order", orderResponses.record)

		payload, marshalErr := sonic.Marshal(&kraken.Order{
			Method: "add_order",
			Params: kraken.LimitOrderParams{
				OrderType: "market",
				Side:      "buy",
				OrderQty:  0.0001,
				Symbol:    "BTC/USD",
			},
		})
		So(marshalErr, ShouldBeNil)

		Convey("When an order is submitted", func() {
			err := live.Write(jsonPayload(payload))

			Convey("Then paper handles the wallet write path", func() {
				So(err, ShouldBeNil)

				waitCaptureCount(t, orderResponses, 1)
			})
		})
	})

	Convey("Given the remediation lock on a private live transport", t, func() {
		live := &Live{privateLock: true}
		order := &kraken.Order{
			Method: "add_order",
			Params: kraken.LimitOrderParams{
				OrderType: "market",
				Side:      "buy",
				OrderQty:  1,
				Symbol:    "BTC/USD",
			},
		}

		Convey("Then an order is rejected before transport access", func() {
			So(live.Write(order), ShouldNotBeNil)
		})
	})
}

func TestLivePost(t *testing.T) {
	Convey("Given a live transport with embedded paper", t, func() {
		previousTaker := viper.GetFloat64("trading.paper.taker_fee_bps")
		previousMaker := viper.GetFloat64("trading.paper.maker_fee_bps")
		viper.Set("trading.paper.taker_fee_bps", 26)
		viper.Set("trading.paper.maker_fee_bps", 16)
		defer func() {
			viper.Set("trading.paper.taker_fee_bps", previousTaker)
			viper.Set("trading.paper.maker_fee_bps", previousMaker)
		}()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		live := &Live{
			ctx:    ctx,
			cancel: cancel,
			sync:   &sync.Map{},
			paper:  paperFixture(t),
		}

		Convey("When TradeVolume is posted", func() {
			body, err := live.paper.Post(
				TradeVolumeEndpoint,
				kraken.NewTradeVolumeRequest([]string{"BTC/USD"}),
			)

			Convey("Then embedded paper serves the REST path", func() {
				So(err, ShouldBeNil)

				schedule := kraken.FeeSchedule{}
				So(sonic.Unmarshal(body, &schedule), ShouldBeNil)
				So(schedule.Pairs["BTC/USD"].Taker, ShouldAlmostEqual, 0.0026, 1e-12)
			})
		})
	})
}
