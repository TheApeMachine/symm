package strategy_test

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
continuationMarketResult retains the open-lot decision, fill, wallet, and
lifecycle outcome from one controlled production-stack replay.
*/
type continuationMarketResult struct {
	decisions       []types.Decision
	exitThesisAt    time.Time
	retreatPressure float64
	holding         types.Holding
	availableCash   *decimal.Decimal
	expectedExitFee *decimal.Decimal
	openPositions   int
	lifecycle       string
	privateRequests [][]byte
}

/*
TestContinuity_ManageFromMarket proves quote retreat holds, sincere drawdown
stops, and a profitable peak with weakening forward support takes profit.
*/
func TestContinuity_ManageFromMarket(t *testing.T) {
	phantom := playOpenStrategyMarket(
		t, phantomStrategyFrames(),
	)
	drawdown := playOpenStrategyMarket(
		t, conditions.Drawdown(32, 0.08, 8).Frames(),
	)
	profit := playOpenStrategyMarket(t, conditions.TapeTakeProfit())

	Convey("Given phantom retreat, sincere loss, and profitable weakening", t, func() {
		_, phantomExit := decisionFor(phantom.decisions, types.ActionExit)
		So(phantomExit, ShouldBeFalse)
		So(phantom.retreatPressure, ShouldBeGreaterThan, 0)
		So(orderRequests(phantom.privateRequests, "sell"), ShouldEqual, 0)
		So(phantom.holding.Status, ShouldEqual, types.OPEN)
		So(phantom.holding.Stoploss, ShouldNotBeNil)
		So(phantom.holding.Stoploss.Action, ShouldEqual, "hold")

		exit, hasExit := decisionFor(drawdown.decisions, types.ActionExit)
		So(hasExit, ShouldBeTrue)
		So(exit.Symbol, ShouldEqual, conditions.Subject())
		So(exit.Cause, ShouldEqual, "stop")
		So(exit.ProposedQuantity, ShouldNotBeNil)
		So(exit.ProposedQuantity.Cmp(decimal.NewFromInt64(100)), ShouldEqual, 0)
		So(exit.At, ShouldEqual, drawdown.exitThesisAt)
		So(orderRequests(drawdown.privateRequests, "sell"), ShouldEqual, 1)
		So(drawdown.holding.Status, ShouldEqual, types.CLOSED)
		So(drawdown.holding.Qty.Sign(), ShouldEqual, 0)
		So(drawdown.holding.SellableQty.Sign(), ShouldEqual, 0)
		So(drawdown.holding.ExitPrice, ShouldNotBeNil)
		So(drawdown.holding.ExitFee, ShouldNotBeNil)
		So(drawdown.holding.ExitPrice.Cmp(exit.ReferencePrice), ShouldEqual, 0)
		So(drawdown.holding.ExitFee.Cmp(drawdown.expectedExitFee), ShouldEqual, 0)
		expectedCash := decimal.NewFromInt64(1000).
			Add(exit.ReferencePrice.Mul(exit.ProposedQuantity)).
			Sub(drawdown.expectedExitFee)
		So(drawdown.availableCash.Cmp(expectedCash), ShouldEqual, 0)
		So(drawdown.openPositions, ShouldEqual, 0)
		So(drawdown.lifecycle, ShouldEqual, types.LifecycleEvaluated)

		takeProfit, hasTakeProfit := decisionFor(profit.decisions, types.ActionExit)
		So(hasTakeProfit, ShouldBeTrue)
		So(takeProfit.Cause, ShouldEqual, "take_profit")
		So(takeProfit.ProposedQuantity.Cmp(decimal.NewFromInt64(100)), ShouldEqual, 0)
		So(orderRequests(profit.privateRequests, "sell"), ShouldEqual, 1)
		So(profit.holding.Status, ShouldEqual, types.CLOSED)
		So(profit.holding.Stoploss, ShouldNotBeNil)
		So(profit.holding.Stoploss.LockedFloor, ShouldBeGreaterThan, 0)
		So(profit.holding.ExitPrice, ShouldNotBeNil)
		So(profit.holding.ExitFee, ShouldNotBeNil)
		So(profit.holding.ExitPrice.Cmp(takeProfit.ReferencePrice), ShouldEqual, 0)
		So(profit.holding.ExitFee.Cmp(profit.expectedExitFee), ShouldEqual, 0)
		So(profit.holding.ExitPrice.Cmp(profit.holding.EntryPrice), ShouldBeGreaterThan, 0)
		So(profit.holding.Qty.Sign(), ShouldEqual, 0)
		expectedProfitCash := decimal.NewFromInt64(1000).
			Add(takeProfit.ReferencePrice.Mul(takeProfit.ProposedQuantity)).
			Sub(profit.expectedExitFee)
		So(profit.availableCash.Cmp(expectedProfitCash), ShouldEqual, 0)
		So(profit.openPositions, ShouldEqual, 0)
		So(profit.lifecycle, ShouldEqual, types.LifecycleEvaluated)
	})
}

/*
phantomStrategyFrames couples an adverse ticker bid flicker to a real Level-3
best-bid cancellation while leaving the trade tape unchanged. The independent
cancellation is the evidence that makes the apparent mark loss insincere.
*/
func phantomStrategyFrames() iter.Seq[tests.Frame] {
	const (
		horizon  = 32
		cancelAt = 8
	)
	startedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mids := make([]float64, horizon)
	bids := make([][]float64, horizon)
	asks := make([][]float64, horizon)
	stamps := make([]time.Time, horizon)

	for index := range horizon {
		mids[index] = 0.10035
		bids[index] = []float64{1000, 300}
		asks[index] = []float64{1000, 300}
		stamps[index] = startedAt.Add(time.Duration(index) * time.Second)

		if index >= cancelAt {
			bids[index] = []float64{1, 300}
		}
	}

	level3 := conditions.Level3Path(mids, bids, asks, stamps)
	quotes := conditions.PhantomDrawdown(horizon, cancelAt, 0.08)

	return tests.RoundRobin(level3.Frames(), quotes.Frames())
}

/*
playOpenStrategyMarket restores one armed wallet lot through the production
recovery path, replays market frames, and captures Continuity and trade output.
*/
func playOpenStrategyMarket(
	t *testing.T,
	frames iter.Seq[tests.Frame],
) *continuationMarketResult {
	t.Helper()
	dataPath := configureStrategyMarket(t)
	entryAt := time.Date(2023, 9, 25, 9, 0, 0, 0, time.UTC)
	stop := types.NewStoploss(context.Background())
	stop.Bind(0.10035, 0.02)
	lot := types.Holding{
		Status:      types.OPEN,
		Symbol:      conditions.Subject(),
		Asset:       "MATIC",
		Qty:         decimal.NewFromFloat64(100),
		SellableQty: decimal.NewFromFloat64(100),
		EntryAt:     &entryAt,
		EntryPrice:  decimal.NewFromFloat64(0.10035),
		EntryFee:    decimal.NewFromFloat64(0.026091),
		Stoploss:    stop,
	}
	recovery := types.CaptureRecovery(
		0,
		map[string]types.Holding{conditions.Subject(): lot},
		nil,
		nil,
	)

	if err := types.SaveRecovery(dataPath, recovery); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	mock := mockapi.NewMockAPI()

	if err := mock.SetTradeVolumeResponse(&kraken.TradeVolume{
		Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFee{
			"MATICUSD": {Fee: decimal.NewFromFloat64(0.26)},
			"BTCUSD":   {Fee: decimal.NewFromFloat64(0.26)},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), nil)
	live := websocket.New(ctx, nil, true, websocket.Level3WebSocketURL)
	t.Cleanup(live.Close)
	api.AttachLevel3(live)

	if err := live.ApplyLevel3([]byte(`{
		"method":"subscribe",
		"params":{"channel":"level3","symbol":["MATIC/USD"],"depth":10}
	}`)); err != nil {
		t.Fatal(err)
	}

	tree := dmt.NewTree("")
	t.Cleanup(func() {
		if err := tree.Close(); err != nil {
			t.Error(err)
		}
	})
	bootFrames := serveStrategyBoot(ctx, mock, []byte(`{
		"channel":"balances","type":"snapshot","sequence":1,"data":[
			{"asset":"USD","balance":"1000","available":"1000","reserved":"0"},
			{"asset":"MATIC","balance":"100","available":"100","reserved":"0"}
		]}`))
	channel := make(chan []byte, 64)
	wired, err := stack.Boot(ctx, api, stack.Options{
		Booter:  system.NewBooter(ctx, channel),
		Channel: channel,
		Thesis:  types.NewThesis(channel, nil),
		Signals: func(
			ctx context.Context,
			api *websocket.API,
			_ *broker.Instrument,
			channel chan []byte,
		) []types.Signal {
			return []types.Signal{
				cvd.NewSignal(ctx, api, channel),
				hawkes.NewSignal(ctx, api, channel),
				toxicity.NewSignal(ctx, api, channel),
			}
		},
		Tree: tree,
	})

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(wired.Close)

	select {
	case <-bootFrames:
	case <-ctx.Done():
		t.Fatal("open strategy market boot frames timed out")
	}

	result := &continuationMarketResult{}
	cutAt := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)
	var latestTicker []byte

	for frame := range frames {
		if frame.Channel == "level3" {
			if err := live.ApplyLevel3(frame.Payload); err != nil {
				t.Fatal(err)
			}

			continue
		}

		mock.Emit(frame.Channel, frame.Payload)

		if frame.Channel == "ticker" {
			latestTicker = frame.Payload
		}

		thesis, tickErr := wired.Crypto.Tick(cutAt)

		if tickErr != nil {
			t.Fatal(tickErr)
		}

		cutAt = cutAt.Add(time.Second)

		if thesis == nil {
			continue
		}

		for _, decision := range thesis.Decisions {
			result.decisions = append(result.decisions, decision)

			if decision.Action == types.ActionExit {
				result.exitThesisAt = thesis.At
			}
		}

		for _, measurement := range thesis.Measurements {
			if measurement == nil ||
				measurement.Metric != types.MetricRetreatingQuantity ||
				measurement.Normalized == nil {
				continue
			}

			if *measurement.Normalized > result.retreatPressure {
				result.retreatPressure = *measurement.Normalized
			}
		}
	}

	result.privateRequests = mock.Private().Writes()
	exit, hasExit := decisionFor(result.decisions, types.ActionExit)

	if hasExit {
		pair, pairErr := wired.Instrument.Pair(exit.Symbol)

		if pairErr != nil {
			t.Fatal(pairErr)
		}

		proceeds := exit.ReferencePrice.Mul(exit.ProposedQuantity)
		fee := wired.Price.Fee(pair, proceeds)

		if fee == nil {
			t.Fatal("exit fee unavailable")
		}

		result.expectedExitFee = fee

		for _, request := range result.privateRequests {
			if orderRequests([][]byte{request}, "sell") == 0 {
				continue
			}

			for _, frame := range conditions.ExitFill(
				request, exit, decimal.NewFromInt64(1000), fee,
			) {
				mock.Private().Emit(frame.Channel, frame.Payload)
			}

			mock.Emit("ticker", latestTicker)
			_, err = wired.Crypto.Tick(cutAt)
			break
		}
	}

	result.holding, err = wired.Balance.Holding(conditions.Subject())

	if err != nil {
		t.Fatal(err)
	}

	if latest := wired.Crypto.LastThesis(); latest != nil {
		if value, found := latest.Lifecycle.Load(conditions.Subject()); found {
			result.lifecycle, _ = value.(string)
		}
	}

	result.availableCash, err = wired.Balance.AvailableCash()

	if err != nil {
		t.Fatal(err)
	}

	result.openPositions = wired.Desk.OpenPositions()

	return result
}
