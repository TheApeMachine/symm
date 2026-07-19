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
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
continuationMarketResult retains the open-lot decision and transport outcome
from one controlled production-stack replay.
*/
type continuationMarketResult struct {
	decisions       []types.Decision
	exitThesisAt    time.Time
	retreatPressure float64
	holding         types.Holding
	lifecycle       string
	privateRequests [][]byte
}

/*
TestContinuity_ManageFromMarket proves quote-only adversity is held while a
sincere traded drawdown through an armed floor selects and submits one exit.
*/
func TestContinuity_ManageFromMarket(t *testing.T) {
	phantom := playOpenStrategyMarket(
		t, phantomStrategyFrames(),
	)
	drawdown := playOpenStrategyMarket(
		t, conditions.Drawdown(32, 0.08, 8).Frames(),
	)

	Convey("Given adverse quote retreat and a sincere traded drawdown", t, func() {
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
		So(exit.ProposedQuantity.Float64(), ShouldEqual, float64(100))
		So(exit.At, ShouldEqual, drawdown.exitThesisAt)
		So(orderRequests(drawdown.privateRequests, "sell"), ShouldEqual, 1)
		So(drawdown.lifecycle, ShouldEqual, types.LifecycleExitSubmitted)
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
	wired, err := stack.Boot(ctx, api, stack.Options{
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

	for frame := range frames {
		if frame.Channel == "level3" {
			if err := live.ApplyLevel3(frame.Payload); err != nil {
				t.Fatal(err)
			}

			continue
		}

		mock.Emit(frame.Channel, frame.Payload)
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

	result.holding, err = wired.Balance.Holding(conditions.Subject())

	if err != nil {
		t.Fatal(err)
	}

	if latest := wired.Crypto.LastThesis(); latest != nil {
		if value, found := latest.Lifecycle.Load(conditions.Subject()); found {
			result.lifecycle, _ = value.(string)
		}
	}

	result.privateRequests = mock.Private().Writes()

	return result
}
