package strategy_test

import (
	"context"
	"iter"
	"math"
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
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
rotationMarketResult retains portfolio decisions, forecasts, lifecycle, and
transport effects from one two-symbol production-stack replay.
*/
type rotationMarketResult struct {
	decisions       []types.Decision
	forecasts       []types.Forecasts
	lifecycle       map[string]string
	privateRequests [][]byte
}

/*
TestArbiter_SelectFromMarket proves a stronger executable challenger displaces
a slower incumbent, while its buy waits for the incumbent's real sell fill.
*/
func TestArbiter_SelectFromMarket(t *testing.T) {
	result := playRotationMarket(t)

	Convey("Given a slow open incumbent and a stronger executable challenger", t, func() {
		entry, hasEntry := rotationDecision(
			result.decisions, types.ActionEnter, "BTC/USD",
		)
		So(hasEntry, ShouldBeTrue)
		So(entry.Cause, ShouldEqual, "rotation")
		So(entry.Displaces, ShouldEqual, conditions.Subject())
		So(entry.ReservationID, ShouldNotBeBlank)
		So(entry.Alternatives["rotate_surplus"], ShouldBeGreaterThan, 0)

		exit, hasExit := rotationDecision(
			result.decisions, types.ActionExit, conditions.Subject(),
		)
		So(hasExit, ShouldBeTrue)
		So(exit.Cause, ShouldEqual, "rotation")
		So(exit.ProposedQuantity, ShouldNotBeNil)
		So(exit.ProposedQuantity.Float64(), ShouldEqual, float64(100))

		challenger, hasChallenger := latestForecastFor(result.forecasts, "BTC/USD")
		incumbent, hasIncumbent := latestForecastFor(
			result.forecasts, conditions.Subject(),
		)
		So(hasChallenger, ShouldBeTrue)
		So(hasIncumbent, ShouldBeTrue)
		So(challenger.ExpectedReturn, ShouldBeGreaterThan, incumbent.ExpectedReturn)
		So(entry.ProposedNotional.Float64(), ShouldBeLessThanOrEqualTo,
			challenger.BuyCapacity)

		So(orderRequests(result.privateRequests, "sell"), ShouldEqual, 1)
		So(orderRequests(result.privateRequests, "buy"), ShouldEqual, 0)
		So(result.lifecycle[conditions.Subject()], ShouldEqual,
			types.LifecycleExitSubmitted)
		So(result.lifecycle["BTC/USD"], ShouldEqual,
			types.LifecycleEntrySelected)
	})
}

/*
playRotationMarket restores one open MATIC lot, advances aligned MATIC and BTC
facts in the same cuts, and captures the ordinary Decide and Trade outcome.
*/
func playRotationMarket(t *testing.T) *rotationMarketResult {
	t.Helper()
	const (
		incumbentEntry    = 0.65
		survivalDistance  = 0.20
		incumbentQuantity = 100
		takerFee          = 0.0026
	)
	dataPath := configureStrategyMarket(t)
	entryAt := time.Date(2023, 9, 25, 9, 0, 0, 0, time.UTC)
	stop := types.NewStoploss(context.Background())
	stop.Bind(incumbentEntry, survivalDistance)
	recovery := types.CaptureRecovery(0, map[string]types.Holding{
		conditions.Subject(): {
			Status:      types.OPEN,
			Symbol:      conditions.Subject(),
			Asset:       "MATIC",
			Qty:         decimal.NewFromFloat64(incumbentQuantity),
			SellableQty: decimal.NewFromFloat64(incumbentQuantity),
			EntryAt:     &entryAt,
			EntryPrice:  decimal.NewFromFloat64(incumbentEntry),
			EntryFee: decimal.NewFromFloat64(
				incumbentEntry * incumbentQuantity * takerFee,
			),
			Stoploss: stop,
		},
	}, nil, nil)

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
		"params":{"channel":"level3","symbol":["MATIC/USD","BTC/USD"],"depth":10}
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
			}
		},
		Tree: tree,
	})

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(wired.Close)
	wired.Desk.SetSlots(1, 0)

	select {
	case <-bootFrames:
	case <-ctx.Done():
		t.Fatal("rotation market boot frames timed out")
	}

	result := &rotationMarketResult{lifecycle: map[string]string{}}
	const (
		incumbentLogReturn  = 0.001
		challengerLogReturn = 0.01
	)
	playRotationFrames(
		t, wired.Crypto, mock, live, result,
		rotationFrames(
			conditions.Subject(), 0.0001, 0.5667, incumbentLogReturn,
		),
		rotationFrames("BTC/USD", 0.1, 50_000, challengerLogReturn),
	)
	result.privateRequests = mock.Private().Writes()

	if latest := wired.Crypto.LastThesis(); latest != nil {
		latest.Lifecycle.Range(func(key, value any) bool {
			symbol, symbolOK := key.(string)
			phase, phaseOK := value.(string)

			if symbolOK && phaseOK {
				result.lifecycle[symbol] = phase
			}

			return true
		})
	}

	return result
}

/*
rotationFrames defines one symbol's market facts; direction is the per-event
log return, with mixed aggressors preventing a one-sided synthetic shortcut.
The horizon leaves both adaptive forecast heads enough observations to mature.
*/
func rotationFrames(
	symbol string,
	priceIncrement float64,
	openingPrice float64,
	direction float64,
) iter.Seq[tests.Frame] {
	const horizon = 64
	startedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	prices := make([]float64, horizon)
	quantities := make([]float64, horizon)
	spreads := make([]float64, horizon)
	depths := make([]float64, horizon)
	sides := make([]string, horizon)
	bids := make([][]float64, horizon)
	asks := make([][]float64, horizon)
	stamps := make([]time.Time, horizon)

	for index := range horizon {
		prices[index] = openingPrice * math.Exp(direction*float64(index))
		quantities[index] = 10
		spreads[index] = priceIncrement * 2
		depths[index] = 500
		stamps[index] = startedAt.Add(time.Duration(index) * time.Second)
		bids[index] = []float64{500, 200}
		asks[index] = []float64{500, 200}
		sides[index] = "buy"

		if index%4 == 3 {
			sides[index] = "sell"
		}

		if symbol == "BTC/USD" {
			quantities[index] = 0.01
			depths[index] = 0.01
			bids[index] = []float64{0.01, 0.004}
			asks[index] = []float64{0.01, 0.004}
		}
	}

	level3 := conditions.Level3PathFor(
		symbol, priceIncrement, prices, bids, asks, stamps,
	)
	market := conditions.MarketPathWithSidesFor(
		symbol, priceIncrement, prices, quantities, sides, spreads, depths,
	)

	return tests.RoundRobin(level3.Frames(), market.Frames())
}

/*
playRotationFrames emits each aligned symbol pair before one market cut so both
fresh forecasts are available to the same portfolio decision.
*/
func playRotationFrames(
	t *testing.T,
	crypto interface {
		Tick(time.Time) (*types.Thesis, error)
	},
	mock *mockapi.MockAPI,
	live *websocket.Live,
	result *rotationMarketResult,
	incumbent iter.Seq[tests.Frame],
	challenger iter.Seq[tests.Frame],
) {
	t.Helper()
	nextIncumbent, stopIncumbent := iter.Pull(incumbent)
	nextChallenger, stopChallenger := iter.Pull(challenger)
	defer stopIncumbent()
	defer stopChallenger()
	cutAt := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)

	for {
		incumbentFrame, incumbentOK := nextIncumbent()
		challengerFrame, challengerOK := nextChallenger()

		if incumbentOK != challengerOK {
			t.Fatal("rotation market streams are not aligned")
		}

		if !incumbentOK {
			return
		}

		if incumbentFrame.Channel == "level3" {
			if err := live.ApplyLevel3(incumbentFrame.Payload); err != nil {
				t.Fatal(err)
			}

			if err := live.ApplyLevel3(challengerFrame.Payload); err != nil {
				t.Fatal(err)
			}

			continue
		}

		mock.Emit(incumbentFrame.Channel, incumbentFrame.Payload)
		mock.Emit(challengerFrame.Channel, challengerFrame.Payload)
		thesis, err := crypto.Tick(cutAt)

		if err != nil {
			t.Fatal(err)
		}

		cutAt = cutAt.Add(time.Second)

		if thesis == nil {
			continue
		}

		result.decisions = append(result.decisions, thesis.Decisions...)
		result.forecasts = append(result.forecasts, thesis.Forecasts...)

		for _, decision := range thesis.Decisions {
			if decision.Action == types.ActionEnter && decision.Cause == "rotation" {
				return
			}
		}
	}
}

/*
rotationDecision returns the first action for one symbol.
*/
func rotationDecision(
	decisions []types.Decision,
	action types.Action,
	symbol string,
) (types.Decision, bool) {
	for _, decision := range decisions {
		if decision.Action == action && decision.Symbol == symbol {
			return decision, true
		}
	}

	return types.Decision{}, false
}

/*
latestForecastFor returns the latest forecast for symbol.
*/
func latestForecastFor(
	forecasts []types.Forecasts,
	symbol string,
) (types.Forecasts, bool) {
	for index := len(forecasts) - 1; index >= 0; index-- {
		if forecasts[index].Symbol == symbol {
			return forecasts[index], true
		}
	}

	return types.Forecasts{}, false
}
