package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/spot"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestExecutionModelReport(t *testing.T) {
	Convey("Given explicit rejected, canceled, no-fill, and expired outcomes", t, func() {
		symbol := testtypes.NewSymbol("SIM3/USD", 100, 73)
		market := NewMarket(context.Background(), []*testtypes.Symbol{symbol})
		defer market.Close()
		config := testtypes.DefaultExecutionConfig()
		config.EmitAcknowledgements = true
		config.ExpireAfter = 100 * time.Millisecond
		config.Outcomes = []testtypes.OrderOutcome{
			testtypes.OrderReject, testtypes.OrderCancel,
			testtypes.OrderNoFill, testtypes.OrderExpire,
		}
		market.WithAutoFill(config)

		for index := range config.Outcomes {
			_, err := market.Private.transport.addOrder(spot.AddOrderRequest{
				ClOrdId:   fmt.Sprintf("outcome-%d", index),
				OrderType: "market", Type: "buy", Volume: "0.1", Pair: symbol.Pair,
			})
			So(err, ShouldBeNil)
		}

		market.Tick()
		market.Tick()
		mechanics, economics := market.execution.Report()

		Convey("Lifecycle mechanics should remain separate from economics", func() {
			So(mechanics.Submitted, ShouldEqual, 4)
			So(mechanics.Rejected, ShouldEqual, 1)
			So(mechanics.Canceled, ShouldEqual, 1)
			So(mechanics.Expired, ShouldEqual, 2)
			So(mechanics.Filled, ShouldEqual, 0)
			So(economics.ExecutedQuantity, ShouldEqual, 0.0)
			So(mechanics.Orders[0].State, ShouldEqual, "rejected")
			So(mechanics.Orders[1].State, ShouldEqual, "canceled")
			So(mechanics.Orders[2].State, ShouldEqual, "expired")
			So(mechanics.Orders[3].State, ShouldEqual, "expired")
		})
	})

	Convey("Given a buy limit below the current ask", t, func() {
		symbol := testtypes.NewSymbol("LIMIT/USD", 100, 74)
		market := NewMarket(context.Background(), []*testtypes.Symbol{symbol})
		defer market.Close()
		config := testtypes.DefaultExecutionConfig()
		config.EmitAcknowledgements = true
		market.WithAutoFill(config)
		market.Tick()
		_, err := market.Private.transport.addOrder(spot.AddOrderRequest{
			ClOrdId: "resting-limit", OrderType: "limit", Type: "buy",
			Volume: "0.25", Pair: symbol.Pair,
			Price: format(market.latest[symbol.Pair].Bid),
		})
		So(err, ShouldBeNil)
		market.Tick()
		mechanics, economics := market.execution.Report()

		Convey("It should be acknowledged and remain open without a fill", func() {
			So(mechanics.Acknowledged, ShouldEqual, 1)
			So(mechanics.Orders[0].State, ShouldEqual, "open")
			So(economics.ExecutedQuantity, ShouldEqual, 0.0)
		})
	})

	Convey("Given a duplicated private execution frame", t, func() {
		symbol := testtypes.NewSymbol("DUP/USD", 100, 75)
		config := testtypes.NewScenarioConfig([]*testtypes.Symbol{symbol})
		config.Faults.Rules = []testtypes.FaultRule{{
			Channel: "executions", Occurrence: 1, Action: testtypes.FaultDuplicate,
		}}
		market, err := NewMarketWithScenario(context.Background(), config)
		So(err, ShouldBeNil)
		defer market.Close()
		market.WithAutoFill()
		_, err = market.Private.transport.addOrder(spot.AddOrderRequest{
			ClOrdId: "duplicate-delivery", OrderType: "market", Type: "buy",
			Volume: "0.1", Pair: symbol.Pair,
		})
		So(err, ShouldBeNil)
		market.Tick()
		report := market.Report()

		Convey("Venue economics should apply once despite two deliveries", func() {
			So(report.PrivateTransport.Duplicated, ShouldEqual, 1)
			So(report.Mechanics.Filled, ShouldEqual, 1)
			So(report.Economics.ExecutedQuantity, ShouldEqual, 0.1)
			So(market.Validate(), ShouldBeNil)
		})
	})

	Convey("Given insufficient exchange quote inventory", t, func() {
		symbol := testtypes.NewSymbol("POOR/USD", 100, 76)
		config := testtypes.NewScenarioConfig([]*testtypes.Symbol{symbol})
		config.InitialBalances = map[string]float64{"USD": 10}
		market, err := NewMarketWithScenario(context.Background(), config)
		So(err, ShouldBeNil)
		defer market.Close()
		market.WithAutoFill()
		_, err = market.Private.transport.addOrder(spot.AddOrderRequest{
			ClOrdId: "unaffordable", OrderType: "market", Type: "buy",
			Volume: "1", Pair: symbol.Pair,
		})
		So(err, ShouldBeNil)
		market.Tick()
		mechanics, economics := market.execution.Report()

		Convey("The fill should reject without moving balances or economics", func() {
			So(mechanics.Rejected, ShouldEqual, 1)
			So(economics.ExecutedQuantity, ShouldEqual, 0.0)
			So(market.Private.transport.accountBalances()["USD"], ShouldEqual, 10.0)
		})
	})

	Convey("Given otherwise identical low- and high-friction scenarios", t, func() {
		run := func(slippage float64) EconomicsReport {
			symbol := testtypes.NewSymbol("META/USD", 100, 81)
			market := NewMarket(context.Background(), []*testtypes.Symbol{symbol})
			defer market.Close()
			config := testtypes.DefaultExecutionConfig()
			config.DepthLevels = 3
			config.SlippageBasisPoints = slippage
			market.WithAutoFill(config)
			_, err := market.Private.transport.addOrder(spot.AddOrderRequest{
				ClOrdId: "metamorphic", OrderType: "market", Type: "buy",
				Volume: "200", Pair: symbol.Pair,
			})
			So(err, ShouldBeNil)
			market.Tick()
			_, economics := market.execution.Report()

			return economics
		}

		baseline := run(0)
		degraded := run(25)

		Convey("Added slippage must never improve reported economics", func() {
			So(degraded.ExecutedQuantity, ShouldEqual, baseline.ExecutedQuantity)
			So(degraded.Slippage, ShouldBeGreaterThan, baseline.Slippage)
			So(degraded.Fees, ShouldBeGreaterThan, baseline.Fees)
			So(degraded.NetPnL, ShouldBeLessThan, baseline.NetPnL)
		})
	})
}

func TestExecutionModelValidate(t *testing.T) {
	Convey("Given a filled buy and a gap-through sell stop", t, func() {
		symbol := testtypes.NewSymbol("SIM5/USD", 100, 75)
		market := NewMarket(context.Background(), []*testtypes.Symbol{symbol})
		defer market.Close()
		market.WithAutoFill()
		_, err := market.Private.transport.addOrder(spot.AddOrderRequest{
			ClOrdId: "entry", OrderType: "market", Type: "buy",
			Volume: "1", Pair: symbol.Pair,
		})
		So(err, ShouldBeNil)
		market.Tick()
		trigger := market.latest[symbol.Pair].Ask + symbol.PriceIncrement
		_, err = market.Private.transport.addOrder(spot.AddOrderRequest{
			ClOrdId: "stop", OrderType: "stop-loss", Type: "sell",
			Volume: "1", Pair: symbol.Pair, Price: format(trigger),
		})
		So(err, ShouldBeNil)
		market.Tick()
		mechanics, economics := market.execution.Report()

		Convey("Balances, IDs, quantities, fees, and PnL should reconcile", func() {
			So(mechanics.Filled, ShouldEqual, 2)
			So(economics.Fees, ShouldBeGreaterThan, 0)
			So(economics.NetPnL, ShouldBeLessThan, economics.GrossPnL)
			So(market.execution.Validate(), ShouldBeNil)
		})
	})
}
