//go:build !race

package tests

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
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

					system.Thesis.Symbol("SIM1/USD").Positions.Range(func(_, value any) bool {
						position, ok := value.(*broker.Position)

						if !ok {
							return true
						}

						if position.Holding == nil ||
							position.Holding.Symbol != "SIM1/USD" ||
							position.Holding.Status != types.CLOSED {
							return true
						}

						closed++
						So(position.Holding.EntryAt, ShouldNotBeNil)
						So(position.Holding.EntryPrice, ShouldNotBeNil)
						So(position.Holding.ExitAt, ShouldNotBeNil)
						So(position.Holding.ExitPrice, ShouldNotBeNil)
						So(position.Holding.PnL, ShouldNotBeNil)
						So(position.Holding.PnL.Sign(), ShouldEqual, 1)
						So(position.Holding.ReturnPct, ShouldBeGreaterThan, 0.0)
						return true
					})

					So(closed, ShouldBeGreaterThan, 0)
				})
			})
		}),
	)
}

func TestMarketReplayEntryAndExit(t *testing.T) {
	previousDepth := viper.GetInt("market.l3_depth")
	viper.Set("market.l3_depth", 10)
	defer viper.Set("market.l3_depth", previousDepth)
	symbol := testtypes.NewSymbol("IDOS/USD", 0.00455, 13)
	symbol.PriceIncrement = 0.00001
	symbol.PricePrecision = 5
	symbol.QuantityPrecision = 5
	symbol.TakerFeePercent = 0.4
	symbol.MakerFeePercent = 0.23
	symbol.BookDepthLevels = 10

	Convey("Given an exact profitable IDOS/USD Kraken tape", t,
		WithOrders(t, []*testtypes.Symbol{symbol}, cmd.Boot,
			func(market *Market, system *cmd.System) {
				execution := market.Config.Execution
				execution.DepthLevels = 10
				market.WithAutoFill(execution)
				capture, err := os.Open(
					"/Users/theapemachine/.symm/data/backtests/kraken/" +
						"2026-08-13-live-exact-v2/slices/IDOSUSD.jsonl",
				)
				So(err, ShouldBeNil)
				defer capture.Close()

				So(market.Replay(capture), ShouldBeNil)
				position := waitForClosedPosition(
					system.Thesis.Symbol("IDOS/USD"),
					market.Config.BookApplyTimeout,
				)

				Convey("It should retain a profitable completed position on the Thesis", func() {
					So(position, ShouldNotBeNil)
					So(position.Decision.Action, ShouldEqual, types.ActionEnter)
					So(position.Holding.PnL.Sign(), ShouldEqual, 1)
					So(position.Holding.ReturnPct, ShouldBeGreaterThan, 0.0)
					So(market.Report().Economics.NetPnL, ShouldBeGreaterThan, 0.0)
					So(market.Validate(), ShouldBeNil)
				})
			}),
	)
}

func waitForClosedPosition(
	symbol *types.Symbol,
	timeout time.Duration,
) *broker.Position {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		var closed *broker.Position
		symbol.Positions.Range(func(_, value any) bool {
			position, ok := value.(*broker.Position)

			if ok && position.Holding != nil &&
				position.Holding.Status == types.CLOSED {
				closed = position
				return false
			}

			return true
		})

		if closed != nil {
			return closed
		}

		runtime.Gosched()
	}

	return nil
}
