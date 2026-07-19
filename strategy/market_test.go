package strategy_test

import (
	"bytes"
	"context"
	"iter"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
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
strategyMarketResult retains decisions and wallet effects observed from one
production-stack replay. It performs no alternate strategy calculation.
*/
type strategyMarketResult struct {
	decisions       []types.Decision
	forecasts       []types.Forecasts
	availableQuote  float64
	privateRequests [][]byte
}

/*
TestPlanner_DecideFromMarket proves admission and sizing against independently
defined executable, opposing, and capacity-trapped market regimes.
*/
func TestPlanner_DecideFromMarket(t *testing.T) {
	buy := playStrategyMarket(t, strategyFrames(1, 60))
	sell := playStrategyMarket(t, strategyFrames(-1, 240))
	thin := playStrategyMarket(t, strategyFrames(1, 0.5))

	Convey("Given strong buy, mirrored sell, and thin-ask markets", t, func() {
		buyEnter, hasBuyEnter := decisionFor(buy.decisions, types.ActionEnter)
		So(hasBuyEnter, ShouldBeTrue)
		So(buyEnter.Symbol, ShouldEqual, conditions.Subject())
		So(buyEnter.ProposedQuantity, ShouldNotBeNil)
		So(buyEnter.ProposedNotional, ShouldNotBeNil)
		So(buyEnter.ProposedQuantity.Sign(), ShouldBeGreaterThan, 0)
		So(buyEnter.ProposedNotional.Sign(), ShouldBeGreaterThan, 0)
		So(buyEnter.ReservationID, ShouldNotBeBlank)
		So(buyEnter.Utility, ShouldBeGreaterThan, 0)
		So(buy.availableQuote, ShouldBeLessThan, 1000)
		So(orderRequests(buy.privateRequests, "buy"), ShouldEqual, 1)
		So(decisionCount(buy.decisions, types.ActionHold), ShouldEqual, 0)

		buyForecast, hasBuyForecast := latestForecast(buy.forecasts)
		So(hasBuyForecast, ShouldBeTrue)
		So(buyForecast.ExpectedReturn, ShouldBeGreaterThan, 0)
		So(buyEnter.ProposedNotional.Float64(), ShouldBeLessThanOrEqualTo,
			buyForecast.BuyCapacity)

		_, hasSellEnter := decisionFor(sell.decisions, types.ActionEnter)
		So(hasSellEnter, ShouldBeFalse)
		So(orderRequests(sell.privateRequests, "buy"), ShouldEqual, 0)
		sellForecast, hasSellForecast := latestForecast(sell.forecasts)
		So(hasSellForecast, ShouldBeTrue)
		So(sellForecast.ExpectedReturn, ShouldBeLessThan, 0)

		_, hasThinEnter := decisionFor(thin.decisions, types.ActionEnter)
		So(hasThinEnter, ShouldBeFalse)
		So(orderRequests(thin.privateRequests, "buy"), ShouldEqual, 0)
		So(thin.availableQuote, ShouldEqual, float64(1000))
		thinDecision, hasThinDecision := symbolDecision(thin.decisions)
		So(hasThinDecision, ShouldBeTrue)
		So(thinDecision.Action, ShouldEqual, types.ActionNothing)
		So(thinDecision.Reason, ShouldContainSubstring, "minimum")
	})
}

/*
strategyFrames creates one market fact pattern. Aggressive arrivals and the L3
midpoint move in direction; askQuantity independently controls executability.
*/
func strategyFrames(direction float64, askQuantity float64) iter.Seq[tests.Frame] {
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
		prices[index] = 0.5667 * math.Exp(direction*0.01*float64(index))
		quantities[index] = 10
		spreads[index] = 0.0002
		depths[index] = 500
		stamps[index] = startedAt.Add(time.Duration(index) * time.Second)
		bids[index] = []float64{240, 80}
		asks[index] = []float64{askQuantity, askQuantity / 3}
		sides[index] = "buy"

		if index%4 == 3 {
			sides[index] = "sell"
		}

		if direction < 0 {
			bids[index] = []float64{60, 20}
			asks[index] = []float64{240, 80}
			sides[index] = "sell"

			if index%4 == 3 {
				sides[index] = "buy"
			}
		}
	}

	level3 := conditions.Level3Path(prices, bids, asks, stamps)
	market := conditions.MarketPathWithSides(
		prices, quantities, sides, spreads, depths,
	)

	return tests.RoundRobin(level3.Frames(), market.Frames())
}

/*
playStrategyMarket boots the production graph around mock Conns, drives normal
Crypto.Tick planning, and captures the resulting decisions and wallet claim.
*/
func playStrategyMarket(
	t *testing.T,
	frames iter.Seq[tests.Frame],
) *strategyMarketResult {
	t.Helper()
	configureStrategyMarket(t)
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
	bootFrames := serveStrategyBoot(ctx, mock, nil)
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

	select {
	case <-bootFrames:
	case <-ctx.Done():
		t.Fatal("strategy market boot frames timed out")
	}

	result := &strategyMarketResult{}
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

		result.decisions = append(result.decisions, thesis.Decisions...)
		result.forecasts = append(result.forecasts, thesis.Forecasts...)
	}

	result.availableQuote, err = wired.Balance.AvailableQuote()

	if err != nil {
		t.Fatal(err)
	}

	result.privateRequests = mock.Private().Writes()

	return result
}

/*
configureStrategyMarket pins deterministic production settings and restores the
process-wide configuration after each replay.
*/
func configureStrategyMarket(t *testing.T) string {
	t.Helper()
	dataPath := t.TempDir()
	settings := map[string]any{
		"trading.model":                              "live",
		"trading.allocation.max_fraction":            0.20,
		"trading.slots.normal":                       2,
		"trading.slots.reserved":                     0,
		"market.quote_currency":                      "USD",
		"market.subscribe_batch":                     200,
		"market.subscribe_pace":                      time.Duration(0),
		"market.l3_enabled":                          false,
		"market.forecast.rls.initial_variance":       1.0,
		"market.forecast.rls.forgetting_factor":      1.0,
		"market.forecast.rls.calibration_confidence": 0.95,
		"signals.fluid.integration_interval":         100 * time.Millisecond,
		"signals.feed_timeline_capacity":             512,
		"signals.feed_track_capacity":                512,
		"system.data_path":                           dataPath,
	}
	previous := make(map[string]any, len(settings))

	for key, value := range settings {
		previous[key] = viper.Get(key)
		viper.Set(key, value)
	}

	t.Cleanup(func() {
		for key, value := range previous {
			viper.Set(key, value)
		}
	})

	return dataPath
}

/*
serveStrategyBoot emits only the instrument and quote-wallet snapshots required
by the ordinary production boot stages.
*/
func serveStrategyBoot(
	ctx context.Context,
	mock *mockapi.MockAPI,
	balancePayload []byte,
) <-chan struct{} {
	ready := make(chan struct{})

	if len(balancePayload) == 0 {
		balancePayload = []byte(`{
			"channel":"balances","type":"snapshot","sequence":1,"data":[{
				"asset":"USD","balance":"1000","available":"1000","reserved":"0"
			}]}`)
	}

	go func() {
		defer close(ready)
		instrumentSent := false
		balanceSent := false
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()

		for !instrumentSent || !balanceSent {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			for _, request := range mock.Public().Writes() {
				if !instrumentSent && bytes.Contains(request, []byte(`"channel":"instrument"`)) {
					mock.Emit("instrument", []byte(`{
						"channel":"instrument","type":"snapshot","data":{"pairs":[{
							"symbol":"MATIC/USD","base":"MATIC","quote":"USD","status":"online",
							"qty_precision":8,"qty_increment":0.00000001,"price_precision":4,
							"cost_precision":6,"cost_min":0.43,"tick_size":0.0001,
							"price_increment":0.0001,"qty_min":4
						},{
							"symbol":"BTC/USD","base":"BTC","quote":"USD","status":"online",
							"qty_precision":8,"qty_increment":0.00000001,"price_precision":1,
							"cost_precision":5,"cost_min":0.5,"tick_size":0.1,
							"price_increment":0.1,"qty_min":0.0001
						}]}}`))
					instrumentSent = true
				}
			}

			for _, request := range mock.Private().Writes() {
				if !balanceSent && bytes.Contains(request, []byte(`"channel":"balances"`)) {
					mock.Private().Emit("balances", balancePayload)
					balanceSent = true
				}
			}
		}
	}()

	return ready
}

/* decisionFor returns the first decision carrying action. */
func decisionFor(decisions []types.Decision, action types.Action) (types.Decision, bool) {
	for _, decision := range decisions {
		if decision.Action == action {
			return decision, true
		}
	}

	return types.Decision{}, false
}

/* decisionCount reports how often one action was selected across the replay. */
func decisionCount(decisions []types.Decision, action types.Action) int {
	count := 0

	for _, decision := range decisions {
		if decision.Action == action {
			count++
		}
	}

	return count
}

/* symbolDecision returns the latest decision for the controlled subject. */
func symbolDecision(decisions []types.Decision) (types.Decision, bool) {
	for index := len(decisions) - 1; index >= 0; index-- {
		if decisions[index].Symbol == conditions.Subject() {
			return decisions[index], true
		}
	}

	return types.Decision{}, false
}

/*
latestForecast returns the latest forecast for the controlled subject.
*/
func latestForecast(forecasts []types.Forecasts) (types.Forecasts, bool) {
	for index := len(forecasts) - 1; index >= 0; index-- {
		if forecasts[index].Symbol == conditions.Subject() {
			return forecasts[index], true
		}
	}

	return types.Forecasts{}, false
}

/*
orderRequests counts live Conn order requests for one side.
*/
func orderRequests(requests [][]byte, side string) int {
	count := 0

	for _, request := range requests {
		if bytes.Contains(request, []byte(`"method":"add_order"`)) &&
			bytes.Contains(request, []byte(`"side":"`+side+`"`)) {
			count++
		}
	}

	return count
}
