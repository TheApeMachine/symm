package trader_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
)

var symbols = []*testtypes.Symbol{
	testtypes.NewSymbol("SIM1/USD", 64000.0, 42),
	testtypes.NewSymbol("SIM2/USD", 5432.193, 1337),
	testtypes.NewSymbol("SIM3/USD", 103.01234, 90210),
}

func getAPI(ctx context.Context) *websocket.API {
	return websocket.NewAPI(
		ctx,
		websocket.NewWithClient(ctx, nil, false, "", nil),
		websocket.NewWithClient(ctx, nil, false, "", nil),
	)
}

func getInstance(ctx context.Context) *trader.Crypto {
	return trader.NewCrypto(
		ctx,
		getAPI(ctx),
		nil,
		nil,
		strategy.NewPlanner(
			ctx,
			nil,
			getAPI(ctx),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
		nil,
		nil,
	)
}

func TestStatus(t *testing.T) {
	Convey(
		"Setup",
		t, tests.WithMarket(t, symbols, func(market *tests.Market) {
			crypto := getInstance(t.Context())

			Convey("Then the status should be READY", func() {
				So(crypto.Status(), ShouldEqual, types.READY)
			})
		}),
	)
}

func TestSubscribe(t *testing.T) {
	Convey(
		"Setup",
		t, tests.WithMarket(t, symbols, func(market *tests.Market) {
			crypto := getInstance(t.Context())

			Convey("When subscribing to a channel", func() {
				subscription := crypto.Subscribe("test", nil)

				Convey("Then the subscription should be stored", func() {
					So(subscription, ShouldNotBeNil)
					So(subscription.Channel, ShouldNotBeNil)
					So(subscription.Channel, ShouldHaveLength, 0)
				})
			})
		}),
	)
}

func TestIntegration(t *testing.T) {
	Convey(
		"Setup",
		t, tests.WithMarket(t, symbols, func(market *tests.Market) {
			Convey("When the market is transitioned to a fast pump", func() {
				So(market.Transition("SIM1/USD", testtypes.FastPump), ShouldBeNil)

				Convey("Then the market should be in a fast pump state", func() {
					decisions := market.Decisions()

					for _, decision := range decisions {
						if decision.Symbol == "SIM1/USD" {
							So(decision.Action, ShouldEqual, "BUY")
							So(decision.Stoploss.Symbol, ShouldEqual, "SIM1/USD")
						}
					}
				})
			})
		}),
	)
}
