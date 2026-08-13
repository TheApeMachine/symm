//go:build !race

package tests

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/kraken"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

func runAutoFillStackTest(t *testing.T, symbols []*testtypes.Symbol) {
	Convey("Given an executable production-stack position lifecycle", t,
		WithOrders(t, symbols, cmd.Boot, func(market *Market, _ *cmd.System) {
			market.WithAutoFill()
			market.Tick()
			_, private := market.Feeds()
			executions := make(chan *kraken.Execution, 1)
			handler := market.Private.Client().OnReceived.Recurring(
				func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
					execution := kraken.NewExecution(event.Data.Bytes())

					if execution.Channel == "executions" {
						executions <- execution
					}
				},
			)
			defer market.Private.Client().OnReceived.Deregister(handler)
			result, err := private.AddOrder(&spot.AddOrderRequest{
				ClOrdId: "entry-1", OrderType: "market", Type: "buy",
				Volume: "0.25", Pair: symbols[0].Pair,
			})
			So(err, ShouldBeNil)
			So(result.ID, ShouldHaveLength, 1)
			market.Tick()
			var fill *kraken.Execution

			select {
			case fill = <-executions:
			default:
			}

			So(fill, ShouldNotBeNil)
			So(fill.Data, ShouldHaveLength, 1)
			So(fill.Data[0].OrderID, ShouldEqual, result.ID[0])
			So(fill.Data[0].ClientOrderID, ShouldEqual, "entry-1")
			So(fill.Data[0].Symbol, ShouldEqual, symbols[0].Pair)
			So(fill.Data[0].Side, ShouldEqual, "buy")
			So(fill.Data[0].AvgPrice.Float64(),
				ShouldEqual, market.latest[symbols[0].Pair].Ask)
			expectedFee := fill.Data[0].Cost.Float64() *
				simulatedTakerFeePercent / percentDenominator
			So(fill.Data[0].FeeUsdEquiv.Float64(),
				ShouldAlmostEqual, expectedFee, 1e-12)
		}),
	)
}

func TestMarketStackEntryAndExit(t *testing.T) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 64_000, 42),
	}

	Convey("Given the full system driven only by simulated venue data", t,
		WithOrders(t, symbols, cmd.Boot, func(market *Market, system *cmd.System) {
			market.WithAutoFill()

			Convey("When a pump develops into a reversal", func() {
				So(market.Transition("SIM1/USD", testtypes.FastPump), ShouldBeNil)
				So(market.Express("SIM1/USD"), ShouldBeNil)
				So(system.Desk.Holding("SIM1/USD"), ShouldBeGreaterThan, 0)

				So(market.Transition("SIM1/USD", testtypes.FastDump), ShouldBeNil)
				So(market.Flatten("SIM1/USD"), ShouldBeNil)

				Convey("Then the system should have entered and exited an actual lot", func() {
					So(system.Desk.Holding("SIM1/USD"), ShouldEqual, 0)
					closed := 0

					for position := range system.Desk.Positions() {
						if position.Holding == nil ||
							position.Holding.Symbol != "SIM1/USD" ||
							position.Holding.Status != types.CLOSED {
							continue
						}

						closed++
						So(position.Holding.EntryAt, ShouldNotBeNil)
						So(position.Holding.EntryPrice, ShouldNotBeNil)
						So(position.Holding.ExitAt, ShouldNotBeNil)
						So(position.Holding.ExitPrice, ShouldNotBeNil)
						So(position.Holding.PnL, ShouldNotBeNil)
						So(position.Holding.PnL.Sign(), ShouldEqual, 1)
						So(position.Holding.ReturnPct, ShouldBeGreaterThan, 0.0)
					}

					So(closed, ShouldBeGreaterThan, 0)
				})
			})
		}),
	)
}
