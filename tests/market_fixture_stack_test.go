//go:build !race

package tests

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/callback"
	sdkkraken "github.com/theapemachine/api-go/v2/pkg/kraken"
	"github.com/theapemachine/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/kraken"
	testtypes "github.com/theapemachine/symm/tests/types"
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
