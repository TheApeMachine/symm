package tests

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestExecutionModelProcess(t *testing.T) {
	Convey("Given a finite one-level simulated book", t, func() {
		symbol := testtypes.NewSymbol("SIM1/USD", 100, 71)
		market := NewMarket(context.Background(), []*testtypes.Symbol{symbol})
		defer market.Close()
		market.WithAutoFill()
		_, err := market.Private.transport.addOrder(spot.AddOrderRequest{
			ClOrdId: "finite-depth", OrderType: "market", Type: "buy",
			Volume: "250", Pair: symbol.Pair,
		})
		So(err, ShouldBeNil)

		market.Tick()
		mechanics, economics := market.execution.Report()
		So(mechanics.PartiallyFilled, ShouldEqual, 1)
		So(economics.ExecutedQuantity, ShouldBeGreaterThan, 0)
		So(economics.ExecutedQuantity, ShouldBeLessThan, 250.0)

		minimumLevelQuantity := testtypes.DefaultProfiles[testtypes.Baseline].BaseQty *
			testtypes.QuantityJitterMinimum
		completionTickLimit := int(math.Ceil(250 / minimumLevelQuantity))

		for range completionTickLimit {
			mechanics, _ = market.execution.Report()

			if mechanics.Filled == 1 {
				break
			}

			market.Tick()
		}

		Convey("Replenished books should complete without overfilling", func() {
			mechanics, economics = market.execution.Report()
			So(mechanics.Filled, ShouldEqual, 1)
			So(economics.ExecutedQuantity, ShouldEqual, 250.0)
			So(economics.FillRatio, ShouldEqual, 1.0)
			So(market.execution.Validate(), ShouldBeNil)
		})
	})

	Convey("Given three depth levels and explicit slippage", t, func() {
		symbol := testtypes.NewSymbol("SIM2/USD", 100, 72)
		market := NewMarket(context.Background(), []*testtypes.Symbol{symbol})
		defer market.Close()
		config := testtypes.DefaultExecutionConfig()
		config.DepthLevels = 3
		config.SlippageBasisPoints = 5
		market.WithAutoFill(config)
		_, err := market.Private.transport.addOrder(spot.AddOrderRequest{
			ClOrdId: "walk-depth", OrderType: "market", Type: "buy",
			Volume: "200", Pair: symbol.Pair,
		})
		So(err, ShouldBeNil)
		market.Tick()
		mechanics, economics := market.execution.Report()

		Convey("The fill should walk finite tiers and report displacement", func() {
			So(market.latest[symbol.Pair].Asks, ShouldHaveLength, 3)
			So(mechanics.Filled, ShouldEqual, 1)
			So(economics.Slippage, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given independent execution and REST balance delays", t, func() {
		symbol := testtypes.NewSymbol("SIM7/USD", 100, 77)
		market := NewMarket(context.Background(), []*testtypes.Symbol{symbol})
		defer market.Close()
		config := testtypes.DefaultExecutionConfig()
		config.ExecutionDelay = 200 * time.Millisecond
		config.RESTBalanceDelay = 200 * time.Millisecond
		market.WithAutoFill(config)
		initialUSD := market.Private.transport.accountBalances()["USD"]
		_, err := market.Private.transport.addOrder(spot.AddOrderRequest{
			ClOrdId: "delayed", OrderType: "market", Type: "buy",
			Volume: "0.1", Pair: symbol.Pair,
		})
		So(err, ShouldBeNil)
		market.Tick()
		market.Tick()
		mechanics, _ := market.execution.Report()
		So(mechanics.Filled, ShouldEqual, 0)
		market.Tick()
		mechanics, _ = market.execution.Report()

		Convey("Execution and REST truth should advance on separate clocks", func() {
			So(mechanics.Filled, ShouldEqual, 1)
			So(market.Private.transport.accountBalances()["USD"],
				ShouldEqual, initialUSD)
			market.Tick()
			market.Tick()
			So(market.Private.transport.accountBalances()["USD"],
				ShouldBeLessThan, initialUSD)
		})
	})
}

func TestExecutionModelExecutionBeforeAcknowledgment(t *testing.T) {
	Convey("Given execution latency shorter than acknowledgement latency", t, func() {
		symbol := testtypes.NewSymbol("SIM6/USD", 100, 76)
		market := NewMarket(context.Background(), []*testtypes.Symbol{symbol})
		defer market.Close()
		config := testtypes.DefaultExecutionConfig()
		config.EmitAcknowledgements = true
		config.ExecutionBeforeAcknowledgment = true
		config.AcknowledgementDelay = 300 * time.Millisecond
		market.WithAutoFill(config)
		_, err := market.Private.transport.addOrder(spot.AddOrderRequest{
			ClOrdId: "execution-first", OrderType: "market", Type: "buy",
			Volume: "0.1", Pair: symbol.Pair,
		})
		So(err, ShouldBeNil)
		market.Tick()
		mechanics, _ := market.execution.Report()
		So(mechanics.Filled, ShouldEqual, 1)
		So(mechanics.Acknowledged, ShouldEqual, 0)

		for range 3 {
			market.Tick()
		}

		Convey("The later acknowledgement should remain independently visible", func() {
			mechanics, _ = market.execution.Report()
			So(mechanics.Filled, ShouldEqual, 1)
			So(mechanics.Acknowledged, ShouldEqual, 1)
			So(mechanics.Orders[0].State, ShouldEqual, "filled")
		})
	})
}

func BenchmarkExecutionModelProcess(b *testing.B) {
	symbol := testtypes.NewSymbol("BENCH/USD", 100, 101)
	private := NewConn(context.Background())
	private.Configure([]*testtypes.Symbol{symbol})
	defer private.Close()
	config := testtypes.DefaultExecutionConfig()
	config.EnforceBalances = false
	model := newExecutionModel(
		config, testtypes.DefaultProfiles,
		[]*testtypes.Symbol{symbol}, private, 101,
	)
	sample := testtypes.Sample{
		Symbol: symbol.Pair, Bid: 99.99, BidQty: 100,
		Ask: 100.01, AskQty: 100, Last: 100.01,
		Volume: 100, StepVolume: 100, Timestamp: time.Unix(1, 0),
		Bids: []testtypes.DepthLevel{{Price: 99.99, Quantity: 100}},
		Asks: []testtypes.DepthLevel{{Price: 100.01, Quantity: 100}},
	}
	iteration := int64(0)

	for b.Loop() {
		iteration++
		_, err := private.transport.addOrder(spot.AddOrderRequest{
			ClOrdId:   fmt.Sprintf("bench-%d", iteration),
			OrderType: "market", Type: "buy", Volume: "0.1", Pair: symbol.Pair,
		})

		if err != nil {
			b.Fatal(err)
		}

		sample.Timestamp = sample.Timestamp.Add(time.Millisecond)
		model.Process(sample, testtypes.Baseline)
		model.orders = model.orders[:0]
	}
}
