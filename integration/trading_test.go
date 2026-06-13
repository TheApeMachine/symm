package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	marketpkg "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/rawbus"
	"github.com/theapemachine/symm/trader"
)

func TestMain(m *testing.M) {
	if err := loadRepoConfig(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestTreeAllLeaves(t *testing.T) {
	tree, err := logic.NewTree(nil)

	if err != nil {
		t.Fatal(err)
	}

	scenarios := allTreeScenarios()

	if len(scenarios) != 15 {
		t.Fatalf("integration: expected 15 tree leaf scenarios, got %d", len(scenarios))
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			Convey("Given the embedded playbook tree", t, func() {
				evaluation, evalErr := evaluateScenario(tree, scenario)

				So(evalErr, ShouldBeNil)
				So(evaluation, ShouldNotBeNil)
				So(evaluation.Action, ShouldNotBeNil)
				So(evaluation.Action.Type, ShouldEqual, scenario.wantAction)
				So(evaluation.Action.Side, ShouldEqual, scenario.wantSide)
			})
		})
	}
}

func TestTradingStoryExit(t *testing.T) {
	Convey("Given story, crypto, and desk wired on a shared pool", t, func() {
		harness := newHarness(t, true)

		harness.setHolding(testSymbol, 1)
		harness.publishExitSpectrum(
			logic.SourceExhaustion,
			logic.CategoryMechanicalCollapse,
			0.6,
			1.2,
		)

		params, awaitErr := harness.awaitOrder(2 * time.Second)

		Convey("It should settle the position through the trading pipe", func() {
			So(awaitErr, ShouldBeNil)
			So(params.Side, ShouldEqual, trading.Sell)
			So(params.OrderType, ShouldEqual, trading.Market)
		})
	})
}

func TestTradingCryptoForwardsActions(t *testing.T) {
	Convey("Given crypto on the raw bus", t, func() {
		harness := newHarness(t, false)

		action := &logic.Action{
			Type:     logic.ActionMarket,
			Side:     trading.Buy,
			Symbol:   testSymbol,
			Quantity: 1,
		}

		So(rawbus.Send(harness.feed, rawbus.TypeActions, action), ShouldBeNil)

		params, awaitErr := harness.awaitOrder(2 * time.Second)

		Convey("It should forward actions to the desk as market orders", func() {
			So(awaitErr, ShouldBeNil)
			So(params.Side, ShouldEqual, trading.Buy)
			So(params.OrderType, ShouldEqual, trading.Market)
			So(params.Symbol, ShouldEqual, testSymbol)
		})
	})
}

func TestTradingDeskTrailingStop(t *testing.T) {
	Convey("Given a filled long position with a desk trailing stop", t, func() {
		harness := newHarness(t, false)
		fillPrice := 50000.0

		harness.submitEntry(testSymbol, 1, fillPrice)

		buyParams, buyErr := harness.awaitOrder(2 * time.Second)
		So(buyErr, ShouldBeNil)
		So(buyParams.Side, ShouldEqual, trading.Buy)

		harness.publishExecution(buyParams, fillPrice)
		time.Sleep(50 * time.Millisecond)

		harness.publishTicker(testSymbol, fillPrice, fillPrice+10, fillPrice+20)
		harness.publishTicker(testSymbol, 51000, 51010, 51020)

		harness.publishTicker(testSymbol, 49000, 49010, 49020)

		sellParams, sellErr := harness.awaitOrder(2 * time.Second)

		Convey("It should ratchet and exit on a trailing stop breach", func() {
			So(sellErr, ShouldBeNil)
			So(sellParams.Side, ShouldEqual, trading.Sell)
			So(sellParams.OrderType, ShouldEqual, trading.Market)
		})
	})
}

type harness struct {
	ctx    context.Context
	cancel context.CancelFunc
	pool   *qpool.Q[any]
	feed   *internal.Bus
	orders chan trading.AddParams
}

func newHarness(t *testing.T, withStory bool) *harness {
	ctx, cancel := context.WithCancel(context.Background())
	pool := qpool.NewQ[any](ctx, 4, 32, nil)

	feed := internal.NewBus(
		ctx,
		pool,
		[]internal.Channel{
			internal.ChannelMeasurements,
			internal.ChannelRaw,
			internal.ChannelKrakenPrivate,
			internal.ChannelUI,
		},
		[]internal.Subscription{
			internal.Subscribe(internal.ChannelKrakenPrivate, "integration:exchange"),
			internal.Subscribe(internal.ChannelUI, "integration:positions"),
		},
	)

	harness := &harness{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		feed:   feed,
		orders: make(chan trading.AddParams, 8),
	}

	crypto := trader.NewCrypto(ctx, pool)
	desk := broker.NewDesk(ctx, pool)

	harness.publishTicker(testSymbol, 50000, 49990, 50010)

	if withStory {
		story, storyErr := marketpkg.NewStory(ctx, pool)

		if storyErr != nil {
			t.Fatal(storyErr)
		}

		go story.Tick()
	}

	go crypto.Tick()
	go desk.Tick()
	go harness.runFakeExchange()

	return harness
}

func (harness *harness) runFakeExchange() {
	for {
		if harness.ctx.Err() != nil {
			return
		}

		message, err := harness.feed.Receive(internal.ChannelKrakenPrivate)

		if internal.IsShutdown(err) {
			return
		}

		if err != nil || message == nil {
			continue
		}

		frame, ok := message.Value.(types.KrakenMessage)

		if !ok || frame.Method != trading.MethodAddOrder {
			continue
		}

		params, paramsOK := frame.Params.(trading.AddParams)

		if !paramsOK {
			continue
		}

		harness.orders <- params
	}
}

func (harness *harness) publishExecution(params trading.AddParams, fillPrice float64) {
	execution := user.Execution{
		OrderID:     params.ClOrdID,
		ClOrdID:     params.ClOrdID,
		Symbol:      params.Symbol,
		Side:        string(params.Side),
		OrderType:   string(params.OrderType),
		OrderQty:    params.OrderQty,
		ExecType:    "trade",
		OrderStatus: "filled",
		LastQty:     params.OrderQty,
		LastPrice:   fillPrice,
		AvgPrice:    fillPrice,
		CumQty:      params.OrderQty,
	}

	So(rawbus.Send(harness.feed, rawbus.TypeExecutions, []user.Execution{execution}), ShouldBeNil)
}

func (harness *harness) setHolding(symbol string, quantity float64) {
	base, quote, splitErr := market.SplitPairSymbol(symbol)

	So(splitErr, ShouldBeNil)

	balances := user.Balances{
		Currency: quote,
		Balance:  200,
		Inventory: map[string]float64{
			base: quantity,
		},
		AvgEntry: map[string]float64{
			base: 50000,
		},
	}

	So(rawbus.Send(harness.feed, rawbus.TypeBalances, balances), ShouldBeNil)
	So(harness.awaitPositionsUpdate(2*time.Second), ShouldBeNil)
}

func (harness *harness) awaitPositionsUpdate(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		message, err := harness.feed.Receive(internal.ChannelUI)

		if internal.IsShutdown(err) {
			return err
		}

		if err != nil {
			continue
		}

		if message != nil && message.Type == "positions" {
			return nil
		}
	}

	return fmt.Errorf("integration: timed out waiting for positions update")
}

func (harness *harness) publishMeasurement(measurement logic.Measurement) {
	So(measurement.Publish(harness.feed), ShouldBeNil)
	So(harness.feed.Send(
		internal.ChannelMeasurements,
		rawbus.TypeMeasurements.String(),
		measurement,
	), ShouldBeNil)
}

func (harness *harness) publishExitSpectrum(
	triggerSource logic.SourceType,
	triggerCategory logic.CategoryType,
	confidence float64,
	surprise float64,
) {
	at := time.Now()

	for _, source := range logic.SpectrumSources {
		category := logic.CategoryOrganicTrend
		sourceConfidence := 0.25
		sourceSurprise := 0.25

		if source == triggerSource {
			category = triggerCategory
			sourceConfidence = confidence
			sourceSurprise = surprise
		}

		measurement, measurementErr := synthMeasurement(
			source,
			category,
			sourceConfidence,
			sourceSurprise,
			at,
		)

		So(measurementErr, ShouldBeNil)
		harness.publishMeasurement(measurement)
	}
}

func (harness *harness) publishTicker(symbol string, last, bid, ask float64) {
	tickers := market.TickerUpdates{
		&market.TickerUpdate{
			Symbol:    symbol,
			Last:      last,
			Bid:       bid,
			Ask:       ask,
			Timestamp: time.Now(),
		},
	}

	So(rawbus.Send(harness.feed, rawbus.TypeTicker, &tickers), ShouldBeNil)
}

func (harness *harness) submitEntry(
	symbol string, quantity float64, fillPrice float64,
) {
	action := &logic.Action{
		Type:     logic.ActionMarket,
		Side:     trading.Buy,
		Symbol:   symbol,
		Quantity: quantity,
		Price:    fillPrice,
	}

	So(rawbus.Send(harness.feed, rawbus.TypeOrder, action), ShouldBeNil)
}

func (harness *harness) awaitOrder(timeout time.Duration) (trading.AddParams, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case params := <-harness.orders:
			return params, nil
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	return trading.AddParams{}, fmt.Errorf("integration: timed out awaiting order")
}
