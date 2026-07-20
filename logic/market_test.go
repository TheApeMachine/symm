package logic_test

import (
	"bytes"
	"context"
	"iter"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
logicMarketResult retains the public logic outputs observed while one synthetic
market crosses the production stack. It contains no alternate calculation.
*/
type logicMarketResult struct {
	state        manifold.State
	cognition    types.Cognition
	hypotheses   []types.Hypothesis
	forecasts    []types.Forecasts
	measurements []*types.Measurement
}

/*
TestAnalyzer_UpdateFromMarket proves the logic layer against independently
defined buy-support and sell-pressure L3/trade regimes.
*/
func TestAnalyzer_UpdateFromMarket(t *testing.T) {
	buy := playLogicMarket(t, logicFrames(true))
	sell := playLogicMarket(t, logicFrames(false))
	Convey("Given mirrored buy-support and sell-pressure markets", t, func() {
		So(buy.state.GasReady(), ShouldBeTrue)
		So(sell.state.GasReady(), ShouldBeTrue)
		So(buy.state.Wave, ShouldNotBeEmpty)
		So(sell.state.Wave, ShouldNotBeEmpty)
		So(buy.state.PhaseScan, ShouldHaveLength, len(buy.state.Wave))
		So(sell.state.PhaseScan, ShouldHaveLength, len(sell.state.Wave))
		So(buy.state.BuyIntensity, ShouldBeGreaterThan, buy.state.SellIntensity)
		So(sell.state.SellIntensity, ShouldBeGreaterThan, sell.state.BuyIntensity)
		So(buy.state.StressAnisotropy, ShouldBeGreaterThanOrEqualTo, 0)
		So(sell.state.StressAnisotropy, ShouldBeGreaterThanOrEqualTo, 0)
		So(buy.state.SellCapacity.Cmp(buy.state.BuyCapacity), ShouldBeGreaterThan, 0)
		So(sell.state.BuyCapacity.Cmp(sell.state.SellCapacity), ShouldBeGreaterThan, 0)
		So(buy.state.ReferencePrice.Cmp(sell.state.ReferencePrice), ShouldBeGreaterThan, 0)

		So(buy.cognition.Ready, ShouldBeTrue)
		So(sell.cognition.Ready, ShouldBeTrue)
		So(buy.cognition.Winner, ShouldEqual, "buy")
		So(sell.cognition.Winner, ShouldEqual, "sell")

		So(buy.hypotheses, ShouldNotBeEmpty)
		So(sell.hypotheses, ShouldNotBeEmpty)
		latestBuyHypothesis := buy.hypotheses[len(buy.hypotheses)-1]
		latestSellHypothesis := sell.hypotheses[len(sell.hypotheses)-1]

		for _, hypothesis := range []types.Hypothesis{
			latestBuyHypothesis, latestSellHypothesis,
		} {
			So(hypothesis.Claim, ShouldEqual,
				"trade_arrival_imbalance_affects_next_l3_epoch_mid_return")
			So(hypothesis.Treatment, ShouldEqual,
				"buy_sell_arrival_intensity_imbalance")
			So(hypothesis.Outcome, ShouldEqual, "next_l3_epoch_mid_log_return")
		}

		So(buy.forecasts, ShouldNotBeEmpty)
		So(sell.forecasts, ShouldNotBeEmpty)
		latestBuyForecast := buy.forecasts[len(buy.forecasts)-1]
		latestSellForecast := sell.forecasts[len(sell.forecasts)-1]
		So(latestBuyForecast.ExpectedReturn, ShouldBeGreaterThan, 0)
		So(latestSellForecast.ExpectedReturn, ShouldBeLessThan, 0)

		for _, forecast := range []types.Forecasts{
			latestBuyForecast, latestSellForecast,
		} {
			So(forecast.Ready, ShouldBeTrue)
			So(forecast.Calibrated, ShouldBeTrue)
			So(forecast.CalibrationSamples, ShouldBeGreaterThan, 0)
			So(forecast.IncrementalSkillLowerBound, ShouldBeGreaterThan, 0)
			So(forecast.SourceEpoch, ShouldBeGreaterThan, uint64(1))
			So(forecast.ExpiresEpoch, ShouldEqual, forecast.SourceEpoch+1)
			So(forecast.Target, ShouldEqual, "next_l3_epoch_mid_log_return")
			So(forecast.ModelVersion, ShouldEqual, "resonance_return_head_v2_rls")
		}

		for _, result := range []*logicMarketResult{buy, sell} {
			energy := false
			surprise := false

			for _, measurement := range result.measurements {
				if measurement.Source != types.SourceResonance {
					continue
				}

				energy = energy || measurement.Metric == types.MetricResonanceEnergy
				surprise = surprise || measurement.Metric == types.MetricResonanceSurprise
				So(measurement.ValidateStruct(), ShouldBeNil)
			}

			So(energy, ShouldBeTrue)
			So(surprise, ShouldBeTrue)
		}
	})
}

/*
logicFrames builds mirrored market stories: dominant aggressive arrivals move
the next L3 midpoint in their direction while visible opposing touch is thin.
*/
func logicFrames(buying bool) iter.Seq[tests.Frame] {
	const horizon = 48
	startedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mids := make([]float64, horizon)
	prices := make([]float64, horizon)
	quantities := make([]float64, horizon)
	sides := make([]string, horizon)
	bids := make([][]float64, horizon)
	asks := make([][]float64, horizon)
	stamps := make([]time.Time, horizon)
	direction := 1.0

	if !buying {
		direction = -1
	}

	for index := range horizon {
		mids[index] = 0.5667 + direction*0.0001*float64(index)
		prices[index] = mids[index]
		quantities[index] = 10
		stamps[index] = startedAt.Add(time.Duration(index) * time.Second)
		sides[index] = dominantSide(index, buying)

		if buying {
			bids[index] = []float64{240, 80}
			asks[index] = []float64{60, 20}
			continue
		}

		bids[index] = []float64{60, 20}
		asks[index] = []float64{240, 80}
	}

	level3 := conditions.Level3Path(mids, bids, asks, stamps)
	trades := conditions.TradePath(prices, quantities, sides, stamps)

	return tests.RoundRobin(level3.Frames(), trades.Frames())
}

/*
dominantSide keeps both marks identifiable while assigning three of every four
arrivals to the regime's independently chosen direction.
*/
func dominantSide(index int, buying bool) string {
	dominant := "buy"
	minority := "sell"

	if !buying {
		dominant = "sell"
		minority = "buy"
	}

	if index%4 == 3 {
		return minority
	}

	return dominant
}

/*
playLogicMarket boots the ordinary production stack around injected Conns and
records only the logic outputs written to successive production Theses.
*/
func playLogicMarket(
	t *testing.T,
	frames iter.Seq[tests.Frame],
) *logicMarketResult {
	t.Helper()
	configureLogicMarket(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	mock := mockapi.NewMockAPI()
	if responseErr := mock.SetTradeVolumeResponse(&kraken.TradeVolume{
		Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFee{
			"MATICUSD": {Fee: decimal.NewFromFloat64(0.26)},
		}},
	}); responseErr != nil {
		t.Fatal(responseErr)
	}
	api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), nil)
	live := websocket.New(ctx, nil, true, websocket.Level3WebSocketURL)
	t.Cleanup(live.Close)
	api.AttachLevel3(live)
	if subscribeErr := live.ApplyLevel3([]byte(`{
		"method":"subscribe",
		"params":{"channel":"level3","symbol":["MATIC/USD"],"depth":10}
	}`)); subscribeErr != nil {
		t.Fatal(subscribeErr)
	}
	tree := dmt.NewTree("")
	t.Cleanup(func() {
		if closeErr := tree.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	bootFrames := serveLogicBoot(ctx, mock)
	channel := make(chan []byte, viper.GetInt("system.websocket.channel.buffer"))
	booter := system.NewBooter(ctx, channel)
	thesis := types.NewThesis(channel, nil)
	wired, err := stack.Boot(ctx, api, stack.Options{
		Booter:  booter,
		Channel: channel,
		Thesis:  thesis,
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

	select {
	case <-bootFrames:
	case <-ctx.Done():
		t.Fatal("logic market boot frames timed out")
	}

	t.Cleanup(wired.Close)
	wired.Analyzer.Focus(conditions.Subject())
	result := &logicMarketResult{}
	cutAt := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)

	frameIndex := 0

	for frame := range frames {
		frameIndex++

		if frame.Channel == "level3" {
			if applyErr := live.ApplyLevel3(frame.Payload); applyErr != nil {
				t.Fatalf("apply L3 frame %d: %v\n%s", frameIndex, applyErr, frame.Payload)
			}

			continue
		}

		mock.Emit(frame.Channel, frame.Payload)
		thesis, tickErr := wired.Crypto.Tick(cutAt)

		if tickErr != nil {
			t.Fatal(tickErr)
		}

		cutAt = cutAt.Add(time.Second)

		if thesis != nil {
			captureLogicResult(result, thesis)
		}
	}

	return result
}

/*
captureLogicResult copies one cut's public logic products before the durable
Thesis is reset for the next production cut.
*/
func captureLogicResult(result *logicMarketResult, thesis *types.Thesis) {
	if value, found := thesis.Manifold.Load(conditions.Subject()); found {
		result.state = value.(manifold.State)
	}

	if value, found := thesis.Cognition.Load(conditions.Subject()); found {
		result.cognition = value.(types.Cognition)
	}

	result.hypotheses = append(result.hypotheses, thesis.Hypotheses...)
	result.forecasts = append(result.forecasts, thesis.Forecasts...)
	result.measurements = append(result.measurements, thesis.Measurements...)
}

/*
configureLogicMarket pins only the production settings needed for deterministic
virtual-time replay and restores the process-wide configuration afterward.
*/
func configureLogicMarket(t *testing.T) {
	t.Helper()
	settings := map[string]any{
		"trading.model":                              "live",
		"trading.slots.normal":                       1,
		"trading.slots.reserved":                     0,
		"market.quote_currency":                      "USD",
		"market.subscribe_batch":                     200,
		"market.subscribe_pace":                      time.Duration(0),
		"market.l3_enabled":                          false,
		"market.forecast.rls.initial_variance":       1.0,
		"market.forecast.rls.forgetting_factor":      1.0,
		"market.forecast.rls.calibration_confidence": 0.95,
		"signals.fluid.integration_interval":         100 * time.Millisecond,
		"signals.feed_timeline_capacity":             256,
		"signals.feed_track_capacity":                256,
		"system.websocket.channel.buffer":            64,
		"system.data_path":                           t.TempDir(),
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
}

/*
serveLogicBoot answers only the instrument and balance subscriptions required
by the same boot stages used in production.
*/
func serveLogicBoot(ctx context.Context, mock *mockapi.MockAPI) <-chan struct{} {
	ready := make(chan struct{})

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
				if instrumentSent || !bytes.Contains(request, []byte(`"channel":"instrument"`)) {
					continue
				}

				mock.Emit("instrument", []byte(`{
					"channel":"instrument","type":"snapshot","data":{"pairs":[{
						"symbol":"MATIC/USD","base":"MATIC","quote":"USD","status":"online",
						"qty_precision":8,"qty_increment":0.00000001,"price_precision":4,
						"cost_precision":6,"cost_min":0.43,"tick_size":0.0001,
						"price_increment":0.0001,"qty_min":4
					}]}}`))
				instrumentSent = true
			}

			for _, request := range mock.Private().Writes() {
				if balanceSent || !bytes.Contains(request, []byte(`"channel":"balances"`)) {
					continue
				}

				mock.Private().Emit("balances", []byte(`{
					"channel":"balances","type":"snapshot","sequence":1,"data":[{
						"asset":"USD","balance":"1000","available":"1000","reserved":"0"
					}]}`))
				balanceSent = true
			}
		}
	}()

	return ready
}
