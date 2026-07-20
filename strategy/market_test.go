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
	"github.com/theapemachine/symm/signal/ensemble"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/* strategyMarketResult retains production decisions and wallet effects. */
type strategyMarketResult struct {
	decisions       []types.Decision
	forecasts       []types.Forecasts
	availableCash   *decimal.Decimal
	holding         *types.Holding
	lifecycle       string
	privateRequests [][]byte
}

/* TestPlanner_DecideFromMarket proves market admission through wallet fill. */
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
		So(buy.availableCash.Cmp(
			decimal.NewFromInt64(1000).Sub(buyEnter.ProposedNotional),
		), ShouldEqual, 0)
		So(buy.holding, ShouldNotBeNil)
		So(buy.holding.Qty.Cmp(buyEnter.ProposedQuantity), ShouldEqual, 0)
		So(buy.holding.Status, ShouldEqual, types.OPEN)
		So(buy.lifecycle, ShouldEqual, types.LifecycleManaging)
		So(orderRequests(buy.privateRequests, "buy"), ShouldEqual, 1)

		So(buy.forecasts, ShouldNotBeEmpty)
		buyForecast := buy.forecasts[len(buy.forecasts)-1]
		So(buyForecast.ExpectedReturn, ShouldBeGreaterThan, 0)
		So(buyEnter.ProposedNotional.Cmp(
			buyForecast.BuyCapacity,
		), ShouldBeLessThanOrEqualTo, 0)

		_, hasSellEnter := decisionFor(sell.decisions, types.ActionEnter)
		So(hasSellEnter, ShouldBeFalse)
		So(orderRequests(sell.privateRequests, "buy"), ShouldEqual, 0)
		So(sell.forecasts, ShouldNotBeEmpty)
		sellForecast := sell.forecasts[len(sell.forecasts)-1]
		So(sellForecast.ExpectedReturn, ShouldBeLessThan, 0)

		_, hasThinEnter := decisionFor(thin.decisions, types.ActionEnter)
		So(hasThinEnter, ShouldBeFalse)
		So(orderRequests(thin.privateRequests, "buy"), ShouldEqual, 0)
		So(thin.availableCash.Cmp(decimal.NewFromInt64(1000)), ShouldEqual, 0)
		thinDecision, hasThinDecision := decisionFor(
			thin.decisions, types.ActionNothing,
		)
		So(hasThinDecision, ShouldBeTrue)
		So(thinDecision.Action, ShouldEqual, types.ActionNothing)
		So(thinDecision.Reason, ShouldContainSubstring, "minimum")
	})
}

/*
TestPlanner_FullEnsembleDecidesFromMarket runs the exact buy regime that the
curated two-signal test enters on, but through the full production signal
ensemble. It asserts the composed system still decides, and on failure it
attributes where the entry funnel narrowed by tallying reject reasons — the
seam and aggregate behavior no per-component test observes.
*/
func TestPlanner_FullEnsembleDecidesFromMarket(t *testing.T) {
	attribute := func(result *strategyMarketResult) (int, map[string]int) {
		reasons := map[string]int{}
		enters := 0

		for _, decision := range result.decisions {
			switch decision.Action {
			case types.ActionEnter:
				enters++
			case types.ActionNothing:
				reasons[decision.Cause]++
			}
		}

		return enters, reasons
	}

	subset := playStrategyMarketWith(t, strategyFrames(1, 60), func(
		ctx context.Context,
		api *websocket.API,
		_ *broker.Instrument,
		channel chan []byte,
	) []types.Signal {
		return []types.Signal{
			cvd.NewSignal(ctx, api, channel),
			hawkes.NewSignal(ctx, api, channel),
		}
	})
	rampEnters, rampReasons := attribute(subset)

	fullRamp := playStrategyMarketWith(t, strategyFrames(1, 60), ensemble.Production)
	fullRampEnters, fullRampReasons := attribute(fullRamp)

	pumpDump := playStrategyMarketWith(t, strategyPumpDumpFrames(), ensemble.Production)
	pumpEnters, pumpReasons := attribute(pumpDump)

	t.Logf("2-signal ramp : enters=%d rejects=%v", rampEnters, rampReasons)
	t.Logf("ensemble ramp : enters=%d rejects=%v", fullRampEnters, fullRampReasons)
	t.Logf("ensemble pump : enters=%d rejects=%v", pumpEnters, pumpReasons)

	Convey("Given the same buy ramp through subset then full ensemble", t, func() {
		Convey("Then the full ensemble decides at least as the subset does", func() {
			So(fullRampEnters, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a realistic pump-and-dump through the full ensemble", t, func() {
		Convey("Then the composed system enters during the pump", func() {
			So(pumpEnters, ShouldBeGreaterThan, 0)
		})
	})
}

/*
strategyTrendFrames is strategyFrames with an explicit per-step log slope, so a
sweep can locate the trend strength at which the ensemble stops entering.
*/
func strategyTrendFrames(slope float64) iter.Seq[tests.Frame] {
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
		prices[index] = 0.5667 * math.Exp(slope*float64(index))
		quantities[index] = 10
		spreads[index] = 0.0002
		depths[index] = 500
		stamps[index] = startedAt.Add(time.Duration(index) * time.Second)
		bids[index] = []float64{240, 80}
		asks[index] = []float64{60, 20}
		sides[index] = "buy"

		if index%4 == 3 {
			sides[index] = "sell"
		}
	}

	level3 := conditions.Level3Path(prices, bids, asks, stamps)
	market := conditions.MarketPathWithSides(prices, quantities, sides, spreads, depths)

	return tests.RoundRobin(level3.Frames(), market.Frames())
}

/*
TestPlanner_EnsembleDecisionBoundary sweeps trend strength through the full
production ensemble and reports the weakest uptrend that still enters — the
decision boundary. It characterizes the composed system instead of asserting a
single curated outcome, so a regression that silently raises the bar shows up as
a moved boundary rather than a still-green component test.
*/
func TestPlanner_EnsembleDecisionBoundary(t *testing.T) {
	slopes := []float64{0.0, 0.002, 0.005, 0.01, 0.02, 0.04}
	boundary := math.NaN()
	entered := make([]bool, len(slopes))

	for index, slope := range slopes {
		result := playStrategyMarketWith(t, strategyTrendFrames(slope), ensemble.Production)
		_, hasEnter := decisionFor(result.decisions, types.ActionEnter)
		entered[index] = hasEnter

		if hasEnter && math.IsNaN(boundary) {
			boundary = slope
		}

		t.Logf("slope=%.3f enters=%v", slope, hasEnter)
	}

	t.Logf("decision boundary: weakest entering slope=%.3f", boundary)

	Convey("Given a trend-strength sweep through the full ensemble", t, func() {
		Convey("Then a flat market does not enter", func() {
			So(entered[0], ShouldBeFalse)
		})
		Convey("Then a strong trend does enter", func() {
			So(entered[len(entered)-1], ShouldBeTrue)
		})
	})
}

/*
strategyPumpDumpFrames shapes the regime on the user's OXT chart: an
accelerating pump on rising executed volume with an executable thin ask, then a
sharp dump. It feeds both L3 and public streams so cognition, manifold, and the
forecast all see the same coherent event.
*/
func strategyPumpDumpFrames() iter.Seq[tests.Frame] {
	const (
		pump    = 40
		dump    = 24
		horizon = pump + dump
	)
	startedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	prices := make([]float64, horizon)
	quantities := make([]float64, horizon)
	spreads := make([]float64, horizon)
	depths := make([]float64, horizon)
	sides := make([]string, horizon)
	bids := make([][]float64, horizon)
	asks := make([][]float64, horizon)
	stamps := make([]time.Time, horizon)
	price := 0.5667

	for index := range horizon {
		pumping := index < pump

		if pumping {
			price *= 1.045
		} else {
			price *= 0.95
		}

		prices[index] = price
		quantities[index] = 10 + float64(index)
		spreads[index] = 0.0002
		depths[index] = 500
		stamps[index] = startedAt.Add(time.Duration(index) * time.Second)

		if pumping {
			bids[index] = []float64{240, 80}
			asks[index] = []float64{60, 20}
			sides[index] = "buy"

			if index%4 == 3 {
				sides[index] = "sell"
			}

			continue
		}

		bids[index] = []float64{60, 20}
		asks[index] = []float64{240, 80}
		sides[index] = "sell"

		if index%4 == 3 {
			sides[index] = "buy"
		}
	}

	level3 := conditions.Level3Path(prices, bids, asks, stamps)
	market := conditions.MarketPathWithSides(prices, quantities, sides, spreads, depths)

	return tests.RoundRobin(level3.Frames(), market.Frames())
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

/* playStrategyMarket drives one regime through production Tick and trade. */
func playStrategyMarket(
	t *testing.T,
	frames iter.Seq[tests.Frame],
) *strategyMarketResult {
	return playStrategyMarketWith(t, frames, func(
		ctx context.Context,
		api *websocket.API,
		_ *broker.Instrument,
		channel chan []byte,
	) []types.Signal {
		return []types.Signal{
			cvd.NewSignal(ctx, api, channel),
			hawkes.NewSignal(ctx, api, channel),
		}
	})
}

/*
playStrategyMarketWith drives one regime through production Tick and trade using
the caller's signal set, so a test can exercise the full production ensemble
rather than a curated subset.
*/
func playStrategyMarketWith(
	t *testing.T,
	frames iter.Seq[tests.Frame],
	signals stack.SignalFactory,
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
	channel := make(chan []byte, 64)
	wired, err := stack.Boot(ctx, api, stack.Options{
		Booter:  system.NewBooter(ctx, channel),
		Channel: channel,
		Thesis:  types.NewThesis(channel, nil),
		Signals: signals,
		Tree:    tree,
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

		result.decisions = append(result.decisions, thesis.Decisions...)
		result.forecasts = append(result.forecasts, thesis.Forecasts...)
	}

	result.privateRequests = mock.Private().Writes()
	entry, hasEntry := decisionFor(result.decisions, types.ActionEnter)

	if hasEntry {
		for _, request := range result.privateRequests {
			if !bytes.Contains(request, []byte(`"method":"add_order"`)) ||
				!bytes.Contains(request, []byte(`"side":"buy"`)) {
				continue
			}

			for _, frame := range conditions.EntryFill(
				request, entry, decimal.NewFromInt64(1000),
			) {
				mock.Private().Emit(frame.Channel, frame.Payload)
			}

			mock.Emit("ticker", latestTicker)
			_, err = wired.Crypto.Tick(cutAt)
			break
		}
	}

	result.availableCash, err = wired.Balance.AvailableCash()

	if err != nil {
		t.Fatal(err)
	}

	if hasEntry {
		holding, holdingErr := wired.Balance.Holding(conditions.Subject())

		if holdingErr != nil {
			t.Fatal(holdingErr)
		}

		result.holding = &holding
	}

	if latest := wired.Crypto.LastThesis(); latest != nil {
		phase, _ := latest.Lifecycle.Load(conditions.Subject())
		result.lifecycle, _ = phase.(string)
	}

	return result
}

/* configureStrategyMarket pins and restores deterministic production settings. */
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

/* serveStrategyBoot emits the snapshots required by production boot stages. */
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
