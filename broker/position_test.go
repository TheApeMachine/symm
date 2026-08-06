package broker_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/tests"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

const fixtureEventTimeout = 5 * time.Second

/*
fillPrice reports what the simulated book is actually quoting.

The shared execution fixture reports an average price of 105 against a market
that opens at 100, which used to be invisible: the stop anchored its floor to
the live mark, so a fill five dollars above the book produced the same geometry
as one at the touch. It is anchored to the realized entry now, and a lot that
filled five percent above the market is underwater by five percent the moment
it opens — which is a hard-floor breach, correctly. These tests are about
execution correlation, so they fill at a price the book could have filled them
at.
*/
func fillPrice(system *cmd.System, symbol string) string {
	tick := system.Desk.Price().Tick(symbol)

	if tick == nil || tick.Ask == nil {
		return ""
	}

	return tick.Ask.String()
}

func awaitPosition(
	system *cmd.System,
	match func(*broker.Position) bool,
) *broker.Position {
	deadline := time.Now().Add(fixtureEventTimeout)
	var latest *broker.Position

	for time.Now().Before(deadline) {
		positions := slices.Collect(system.Desk.Positions())

		if len(positions) > 0 {
			latest = positions[0]

			if match(latest) {
				return latest
			}
		}

		runtime.Gosched()
	}

	return latest
}

func triggerStop(
	market *tests.Market,
	system *cmd.System,
	symbol string,
) *broker.Position {
	system.Desk.ApplyEvidence(types.StopEvidence{
		Symbol:     symbol,
		RegimeExit: types.TriggerPumpDumpSellIgnition,
		ObservedAt: time.Now().UTC(),
		Present:    true,
	})
	market.Tick()

	return awaitPosition(system, func(position *broker.Position) bool {
		return position.ExitOrder != nil && position.ExitOrder.ClOrdId != ""
	})
}

func TestPositionOnExecution(t *testing.T) {
	Convey("Given a position entered through the production-wired desk", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 100.0, 42),
		}

		Convey("Execution updates should follow private transport correlation", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			market.Tick()
			decision := entryDecision(system, symbols[0].Pair)
			So(system.Desk.Execute(decision), ShouldBeNil)

			positions := slices.Collect(system.Desk.Positions())
			So(positions, ShouldHaveLength, 1)
			position := positions[0]

			Convey("A correlated order-open event without a fill should remain pending", func() {
				opened := executionfixture.BuyFill()
				opened.ClientOrderID = decision.ID
				opened.Symbol = decision.Symbol
				opened.ExecType = "new"
				opened.OrderStatus = "open"
				opened.LastQty = ""
				opened.CumQty = ""
				market.Private.Publish("executions", executionfixture.Frame(opened))
				market.Tick()

				position = slices.Collect(system.Desk.Positions())[0]
				So(position.Status, ShouldEqual, types.PENDING)
				So(position.Holding.Status, ShouldEqual, types.PENDING)
				So(system.Desk.OpenPositions(), ShouldEqual, 1)
			})

			Convey("A filled event with another client order ID should be ignored", func() {
				originalQuantity := position.Holding.Qty.String()
				openPositions := system.Desk.OpenPositions()
				uncorrelated := executionfixture.BuyFill()
				uncorrelated.ClientOrderID = uuid.NewString()
				uncorrelated.Symbol = decision.Symbol
				uncorrelated.CumQty = "1.5"
				market.Private.Publish("executions", executionfixture.Frame(uncorrelated))
				market.Tick()

				position = slices.Collect(system.Desk.Positions())[0]
				So(position.Status, ShouldEqual, types.PENDING)
				So(position.Holding.Status, ShouldEqual, types.PENDING)
				So(position.Holding.Qty.String(), ShouldEqual, originalQuantity)
				So(system.Desk.OpenPositions(), ShouldEqual, openPositions)
			})

			Convey("Correlated buy and sell fills should open and close inventory", func() {
				buy := executionfixture.BuyFill()
				buy.ClientOrderID = decision.ID
				buy.Symbol = decision.Symbol
				buy.AvgPrice = fillPrice(system, decision.Symbol)
				buy.CumQty = decision.ProposedQuantity.String()
				market.Private.Publish("executions", executionfixture.Frame(buy))
				market.Tick()

				position = slices.Collect(system.Desk.Positions())[0]
				So(position.Status, ShouldEqual, types.OPEN)
				So(position.Holding.Status, ShouldEqual, types.OPEN)
				So(position.Holding.SellableQty.Cmp(decimal.NewFromFloat64(0.25)), ShouldEqual, 0)
				So(position.Holding.EntryAt, ShouldNotBeNil)
				expectedEntryAt, err := time.Parse(time.RFC3339Nano, buy.Timestamp)
				So(err, ShouldBeNil)
				So(*position.Holding.EntryAt, ShouldResemble, expectedEntryAt)
				So(system.Desk.OpenPositions(), ShouldEqual, 1)

				position = triggerStop(market, system, decision.Symbol)
				exitID := position.ExitOrder.ClOrdId
				So(exitID, ShouldNotBeBlank)

				sell := executionfixture.ExitFill()
				sell.ClientOrderID = exitID
				sell.Symbol = decision.Symbol
				sell.CumQty = decision.ProposedQuantity.String()
				market.Private.Publish("executions", executionfixture.Frame(sell))
				market.Tick()

				position = awaitPosition(system, func(position *broker.Position) bool {
					return position.Status == types.CLOSED
				})
				So(position.Status, ShouldEqual, types.CLOSED)
				So(position.Holding.Status, ShouldEqual, types.CLOSED)
				So(position.Holding.SellableQty.Sign(), ShouldEqual, 0)
				So(position.Holding.ExitAt, ShouldNotBeNil)
				expectedExitAt, err := time.Parse(time.RFC3339Nano, sell.Timestamp)
				So(err, ShouldBeNil)
				So(*position.Holding.ExitAt, ShouldResemble, expectedExitAt)
				So(system.Desk.OpenPositions(), ShouldEqual, 0)
			})

			Convey("Split fills should accumulate each execution fee exactly once", func() {
				firstBuy := executionfixture.BuyFill()
				firstBuy.ClientOrderID = decision.ID
				firstBuy.Symbol = decision.Symbol
				firstBuy.ExecID = "entry-fill-one"
				firstBuy.OrderStatus = "partially_filled"
				firstBuy.LastQty = "0.10"
				firstBuy.CumQty = "0.10"
				firstBuy.AvgPrice = "100"
				firstBuy.FeeUsdEquiv = "0.026"
				market.Private.Publish("executions", executionfixture.Frame(firstBuy))
				market.Tick()

				secondBuy := firstBuy
				secondBuy.ExecID = "entry-fill-two"
				secondBuy.OrderStatus = "filled"
				secondBuy.LastQty = "0.15"
				secondBuy.CumQty = "0.25"
				secondBuy.FeeUsdEquiv = "0.039"
				market.Private.Publish("executions", executionfixture.Frame(secondBuy))
				market.Private.Publish("executions", executionfixture.Frame(secondBuy))
				market.Tick()

				expectedFee, err := decimal.NewFromString("0.065")
				So(err, ShouldBeNil)
				position = awaitPosition(system, func(position *broker.Position) bool {
					return position.Holding.EntryFee != nil &&
						position.Holding.EntryFee.Cmp(expectedFee) == 0
				})
				So(position.Holding.EntryFee.Cmp(expectedFee), ShouldEqual, 0)

				position = triggerStop(market, system, decision.Symbol)
				exitID := position.ExitOrder.ClOrdId
				So(exitID, ShouldNotBeBlank)

				firstSell := executionfixture.ExitFill()
				firstSell.ClientOrderID = exitID
				firstSell.Symbol = decision.Symbol
				firstSell.ExecID = "exit-fill-one"
				firstSell.OrderStatus = "partially_filled"
				firstSell.LastQty = "0.10"
				firstSell.CumQty = "0.10"
				firstSell.FeeUsdEquiv = "0.030"
				market.Private.Publish("executions", executionfixture.Frame(firstSell))
				market.Tick()
				firstExitFee, err := decimal.NewFromString("0.030")
				So(err, ShouldBeNil)
				awaitPosition(system, func(position *broker.Position) bool {
					return position.Holding.ExitFee != nil &&
						position.Holding.ExitFee.Cmp(firstExitFee) == 0
				})

				secondSell := firstSell
				secondSell.ExecID = "exit-fill-two"
				secondSell.OrderStatus = "filled"
				secondSell.LastQty = "0.15"
				secondSell.CumQty = "0.25"
				secondSell.FeeUsdEquiv = "0.035"
				market.Private.Publish("executions", executionfixture.Frame(secondSell))
				market.Private.Publish("executions", executionfixture.Frame(secondSell))
				market.Tick()

				position = awaitPosition(system, func(position *broker.Position) bool {
					return position.Holding.ExitFee != nil &&
						position.Holding.ExitFee.Cmp(expectedFee) == 0
				})
				So(position.Holding.ExitFee.Cmp(expectedFee), ShouldEqual, 0)
			})

			Convey("A stop during a partial entry should liquidate every late fill", func() {
				partial := executionfixture.BuyFill()
				partial.ClientOrderID = decision.ID
				partial.Symbol = decision.Symbol
				partial.ExecID = "partial-entry"
				partial.OrderStatus = "partially_filled"
				partial.LastQty = "0.10"
				partial.CumQty = "0.10"
				partial.AvgPrice = fillPrice(system, decision.Symbol)
				market.Private.Publish("executions", executionfixture.Frame(partial))
				market.Tick()

				position = triggerStop(market, system, decision.Symbol)
				firstExitID := position.ExitOrder.ClOrdId

				late := partial
				late.ExecID = "late-entry"
				late.OrderStatus = "filled"
				late.LastQty = "0.15"
				late.CumQty = "0.25"
				market.Private.Publish("executions", executionfixture.Frame(late))
				market.Tick()

				firstExit := executionfixture.ExitFill()
				firstExit.ClientOrderID = firstExitID
				firstExit.Symbol = decision.Symbol
				firstExit.LastQty = "0.10"
				firstExit.CumQty = "0.10"
				market.Private.Publish("executions", executionfixture.Frame(firstExit))
				market.Tick()
				awaitPosition(system, func(position *broker.Position) bool {
					return position.Holding.SellableQty.Cmp(decimal.NewFromFloat64(0.15)) == 0
				})
				market.Tick()

				position = awaitPosition(system, func(position *broker.Position) bool {
					return position.ExitOrder.ClOrdId != firstExitID
				})
				secondExitID := position.ExitOrder.ClOrdId
				So(secondExitID, ShouldNotEqual, firstExitID)
				So(position.Holding.SellableQty.Cmp(decimal.NewFromFloat64(0.15)), ShouldEqual, 0)

				secondExit := executionfixture.ExitFill()
				secondExit.ClientOrderID = secondExitID
				secondExit.Symbol = decision.Symbol
				secondExit.ExecID = "residual-exit"
				secondExit.LastQty = "0.15"
				secondExit.CumQty = "0.15"
				market.Private.Publish("executions", executionfixture.Frame(secondExit))
				market.Tick()

				position = awaitPosition(system, func(position *broker.Position) bool {
					return position.Status == types.CLOSED
				})
				So(position.Status, ShouldEqual, types.CLOSED)
				So(position.Holding.SellableQty.Sign(), ShouldEqual, 0)
			})

			Convey("A canceled stop order should retry only the unsold quantity", func() {
				buy := executionfixture.BuyFill()
				buy.ClientOrderID = decision.ID
				buy.Symbol = decision.Symbol
				buy.AvgPrice = fillPrice(system, decision.Symbol)
				buy.CumQty = decision.ProposedQuantity.String()
				market.Private.Publish("executions", executionfixture.Frame(buy))
				market.Tick()

				position = triggerStop(market, system, decision.Symbol)
				firstExitID := position.ExitOrder.ClOrdId

				canceled := executionfixture.ExitFill()
				canceled.ClientOrderID = firstExitID
				canceled.Symbol = decision.Symbol
				canceled.ExecType = "trade"
				canceled.OrderStatus = "canceled"
				canceled.LastQty = "0.10"
				canceled.CumQty = "0.10"
				market.Private.Publish("executions", executionfixture.Frame(canceled))
				market.Tick()
				awaitPosition(system, func(position *broker.Position) bool {
					return position.Holding.SellableQty.Cmp(decimal.NewFromFloat64(0.15)) == 0
				})
				market.Tick()

				position = awaitPosition(system, func(position *broker.Position) bool {
					return position.ExitOrder.ClOrdId != firstExitID
				})
				So(position.ExitOrder.ClOrdId, ShouldNotEqual, firstExitID)
				So(position.Holding.SellableQty.Cmp(decimal.NewFromFloat64(0.15)), ShouldEqual, 0)
				retryQuantity, err := decimal.NewFromString(position.ExitOrder.Volume)
				So(err, ShouldBeNil)
				So(retryQuantity.Cmp(decimal.NewFromFloat64(0.15)), ShouldEqual, 0)
				So(position.Status, ShouldEqual, types.PENDING)
			})
		}))
	})
}

/*
stopRows reads back every stop row the recorder wrote for one position.

The caller points the data path at a directory of its own first. The default
audit file is shared across every process that boots the system, and one of them
rotating it on boot while this test is reading is a race that has nothing to do
with what is being asserted. Rows are still matched on the position identifier,
since a single run writes rows for more than one lot.

The recorder flushes on Close, which is why the caller closes the market before
reading.
*/
func stopRows(t *testing.T, positionID string) []map[string]any {
	t.Helper()

	file, err := os.Open(filepath.Join(
		utils.ResolveDataPath(), "runtime-audit.jsonl",
	))

	if err != nil {
		return nil
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	rows := make([]map[string]any, 0)

	for scanner.Scan() {
		var row map[string]any

		if json.Unmarshal(scanner.Bytes(), &row) != nil ||
			row["type"] != string(audit.CategoryExecutionLifecycle) {
			continue
		}

		value, ok := row["value"].(map[string]any)

		if !ok || value["position_id"] != positionID {
			continue
		}

		rows = append(rows, value)
	}

	So(scanner.Err(), ShouldBeNil)

	return rows
}

func TestPositionStopAudit(t *testing.T) {
	Convey("Given a position whose regulator has drawn its geometry", t, func() {
		dataPath := viper.GetString("system.data_path")
		viper.Set("system.data_path", t.TempDir())

		defer viper.Set("system.data_path", dataPath)

		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("Every change should leave a row that can label the outcome later", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			market.Tick()
			decision := entryDecision(system, symbols[0].Pair)
			So(system.Desk.Execute(decision), ShouldBeNil)

			position := slices.Collect(system.Desk.Positions())[0]
			buy := executionfixture.BuyFill()
			buy.ClientOrderID = decision.ID
			buy.Symbol = decision.Symbol
			buy.AvgPrice = fillPrice(system, decision.Symbol)
			buy.CumQty = decision.ProposedQuantity.String()
			market.Private.Publish("executions", executionfixture.Frame(buy))
			market.Tick()
			market.Tick()

			// The recorder drains on its own goroutine and flushes when it is
			// closed, so the rows are read back from a settled file.
			market.Close()

			rows := stopRows(t, position.ID)
			So(rows, ShouldNotBeEmpty)

			reasons := make([]string, 0, len(rows))

			for _, row := range rows {
				transition, ok := row["transition"].(map[string]any)
				So(ok, ShouldBeTrue)
				reason, _ := transition["reason"].(string)
				reasons = append(reasons, reason)

				/*
					Both boundaries and the mark between them travel on every
					row. Without them a row says a floor moved but not what it
					moved relative to, and the first-passage question — profit
					before loss — cannot be answered from the file.
				*/
				So(row["symbol"], ShouldEqual, symbols[0].Pair)
				So(transition["hard_floor"], ShouldNotBeNil)
				So(transition["profit_line"], ShouldNotBeNil)
				So(transition["mark"], ShouldNotBeNil)
				So(transition["phase"], ShouldNotBeNil)
			}

			/*
				The realized fill is what promotes the regulator from a
				provisional basis to a confirmed one, and the row proving it
				happened is the one that shows the audit trail follows what was
				paid rather than what was quoted.
			*/
			So(reasons, ShouldContain, "bound_on_fill")
		}))
	})
}

func TestPositionStopSnapshot(t *testing.T) {
	Convey("Given a filled position that has seen a tick", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("Its geometry should be readable from outside the desk goroutine", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			market.Tick()
			decision := entryDecision(system, symbols[0].Pair)
			So(system.Desk.Execute(decision), ShouldBeNil)

			position := slices.Collect(system.Desk.Positions())[0]
			buy := executionfixture.BuyFill()
			buy.ClientOrderID = decision.ID
			buy.Symbol = decision.Symbol
			buy.AvgPrice = fillPrice(system, decision.Symbol)
			buy.CumQty = decision.ProposedQuantity.String()
			market.Private.Publish("executions", executionfixture.Frame(buy))
			market.Tick()
			market.Tick()

			snapshot := position.StopSnapshot()

			/*
				The strategy prices a lot's remaining upside and downside against
				these. An absent snapshot is not a harmless gap — it silently
				disables the whole first-passage path, the way the unread
				StopEvidence struct used to.
			*/
			So(snapshot.Present, ShouldBeTrue)
			So(snapshot.Entry, ShouldNotBeNil)
			So(snapshot.Mark, ShouldNotBeNil)
			So(snapshot.HardFloor, ShouldNotBeNil)
			So(snapshot.ProfitFloor, ShouldNotBeNil)
			So(snapshot.RiskDistance.Sign(), ShouldEqual, 1)
			So(snapshot.HardFloor.Cmp(snapshot.Entry), ShouldEqual, -1)
			So(snapshot.ProfitFloor.Cmp(snapshot.Entry), ShouldEqual, 1)
		}))
	})
}

func TestPositionOnTicker(t *testing.T) {
	Convey("Given a triggered stoploss with an exit already pending", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("BTC/USD", 100.0, 42),
		}

		Convey("Ticker updates should not submit the sell again", tests.WithOrders(t, symbols, cmd.Boot, func(market *tests.Market, system *cmd.System) {
			market.Tick()
			decision := entryDecision(system, symbols[0].Pair)
			So(system.Desk.Execute(decision), ShouldBeNil)

			buy := executionfixture.BuyFill()
			buy.ClientOrderID = decision.ID
			buy.Symbol = decision.Symbol
			buy.AvgPrice = fillPrice(system, decision.Symbol)
			buy.CumQty = decision.ProposedQuantity.String()
			market.Private.Publish("executions", executionfixture.Frame(buy))
			market.Tick()

			position := triggerStop(market, system, decision.Symbol)
			So(position.Status, ShouldEqual, types.PENDING)
			So(position.ExitOrderResult, ShouldNotBeNil)
			firstOrderID := position.ExitOrderResult.ID[0]

			market.Tick()
			position = slices.Collect(system.Desk.Positions())[0]

			So(position.ExitOrderResult.ID[0], ShouldEqual, firstOrderID)
		}))
	})
}
