package causal

import (
	"math"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const tradeWindow = 5 * time.Minute

/*
CausalSymbol holds per-symbol Pearl-ladder history and microstructure state.
DAG: MacroMomentum → PriceVelocity ← LocalFlow, Liquidity backdoors macro/flow.
*/
type CausalSymbol struct {
	pair              asset.Pair
	samples           []causalSample
	interventionHist  []float64
	upliftHist        []float64
	confidenceHistory []float64
	lastPrice         float64
	lastAt            time.Time
	lastElapsed       time.Duration
	hasPrior          bool
	dailyQuoteVol     float64
	changePct         float64
	spreadBPS         float64
	imbalance         float64
	buyPressure       float64
	volumeWindow      *adaptive.Window
	pressure          *adaptive.EMA
	calibrator        engine.PredictionCalibrator
}

func NewCausalSymbol(pair asset.Pair, params engine.CalibrationParams) *CausalSymbol {
	return &CausalSymbol{
		pair:         pair,
		samples:      make([]causalSample, 0, causalHistoryCap),
		volumeWindow: adaptive.NewWindow(tradeWindow),
		pressure:     adaptive.NewEMA(0),
		calibrator:   engine.NewPredictionCalibrator(params),
	}
}

func (state *CausalSymbol) FeedTicker(row market.TickerRow) {
	if row.Last > 0 {
		state.lastPrice = row.Last
		state.dailyQuoteVol = row.Volume * row.Last
	}

	state.changePct = row.ChangePct
}

func (state *CausalSymbol) FeedTrade(tick trade.Data) {
	_, _ = state.volumeWindow.Next(
		0,
		float64(tick.Timestamp.UnixNano()),
		tick.Qty,
		state.lastPrice,
	)

	sign := -1.0

	if tick.Side == "buy" {
		sign = 1.0
	}

	state.buyPressure, _ = state.pressure.Next(0, sign)
}

func (state *CausalSymbol) FeedBook(delta market.BookLevelsDelta) {
	if len(delta.Bids) == 0 || len(delta.Asks) == 0 {
		return
	}

	bid := delta.Bids[0].Price
	ask := delta.Asks[0].Price
	mid := (bid + ask) / 2

	if mid > 0 {
		state.spreadBPS = (ask - bid) / mid * 10000
	}

	total := delta.Bids[0].Volume + delta.Asks[0].Volume

	if total > 0 {
		state.imbalance = (delta.Bids[0].Volume - delta.Asks[0].Volume) / total
	}
}

func (state *CausalSymbol) Measure(macroMomentum float64, now time.Time) (engine.Measurement, bool) {
	batchVolume := state.volumeWindow.Sum()

	if state.lastPrice <= 0 || batchVolume <= 0 || state.spreadBPS <= 0 ||
		state.imbalance <= 0 || state.buyPressure <= 0 {
		return engine.Measurement{}, false
	}

	localFlow := batchVolume * (state.buyPressure + 1) / 2
	liquidity := bookLiquidity(state.spreadBPS, batchVolume)
	sample, ready := state.buildSample(macroMomentum, liquidity, localFlow, state.lastPrice, now)

	if !ready {
		state.commitSample(sample, state.lastPrice, now)

		return engine.Measurement{}, false
	}

	confidence, reason := state.evaluate(sample)

	state.commitSample(sample, state.lastPrice, now)

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	return engine.Measurement{
		Type:       engine.Causal,
		Source:     causalSource,
		Regime:     "causal",
		Reason:     reason,
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
	}, true
}

func (state *CausalSymbol) ApplyFeedback(feedback engine.PredictionFeedback) {
	state.calibrator.Apply(feedback)
}

func (state *CausalSymbol) buildSample(
	macroMomentum, liquidity, localFlow, price float64,
	now time.Time,
) (causalSample, bool) {
	velocity := 0.0

	if state.hasPrior && !state.lastAt.IsZero() && state.lastPrice > 0 && price > 0 {
		elapsedSec := now.Sub(state.lastAt).Seconds()

		if elapsedSec > 0 {
			velocity = (price - state.lastPrice) / state.lastPrice / elapsedSec
		}
	}

	sample := causalSample{
		macroMomentum: macroMomentum,
		liquidity:     liquidity,
		localFlow:     localFlow,
		priceVelocity: velocity,
	}

	if !state.hasPrior {
		state.lastAt = now
		state.hasPrior = true

		return sample, false
	}

	return sample, len(state.samples) >= minCausalHistory
}

func (state *CausalSymbol) commitSample(sample causalSample, price float64, now time.Time) {
	if !state.lastAt.IsZero() {
		state.lastElapsed = now.Sub(state.lastAt)
	}

	state.samples = append(state.samples, sample)

	if len(state.samples) > causalHistoryCap {
		state.samples = state.samples[len(state.samples)-causalHistoryCap:]
	}

	state.lastAt = now
}

func (state *CausalSymbol) evaluate(current causalSample) (float64, string) {
	if len(state.samples) < minCausalHistory {
		return 0, ""
	}

	samples := state.samples
	association := associationEffect(samples)
	intervention := kernelBackdoorFlowEffect(samples) * state.calibrator.Scale()

	if intervention <= 0 {
		state.recordIntervention(intervention)

		return 0, ""
	}

	state.recordIntervention(intervention)

	model, fitOK := fitNonLinearStructural(samples)

	if !fitOK {
		return state.calibrator.NormalizeConfidence(intervention, state.interventionHist), "intervention"
	}

	interventionFlow := flowInterventionLevel(samples)
	uplift := nonLinearCounterfactualUplift(current, model, interventionFlow)

	if uplift <= 0 {
		normalized := state.calibrator.NormalizeConfidence(intervention, state.interventionHist)
		state.recordConfidence(intervention)

		return normalized, "intervention"
	}

	state.recordUplift(uplift)

	confounded := math.Abs(intervention-association) > math.Abs(association)*0.25
	reason := "intervention"

	if confounded && uplift > 0 {
		reason = "counterfactual_like"
	}

	interventionScore := state.calibrator.NormalizeConfidence(intervention, state.interventionHist)
	upliftScore := state.calibrator.NormalizeConfidence(uplift, state.upliftHist)
	score := interventionScore

	if upliftScore > 0 {
		score = 0.6*interventionScore + 0.4*upliftScore
	}

	if score <= 0 {
		return 0, ""
	}

	state.recordConfidence(intervention)

	return score, reason
}

func (state *CausalSymbol) recordConfidence(confidence float64) {
	if confidence <= 0 {
		return
	}

	state.confidenceHistory = append(state.confidenceHistory, confidence)

	if len(state.confidenceHistory) > causalHistoryCap {
		state.confidenceHistory = state.confidenceHistory[len(state.confidenceHistory)-causalHistoryCap:]
	}
}

func (state *CausalSymbol) recordIntervention(effect float64) {
	if effect == 0 {
		return
	}

	state.interventionHist = append(state.interventionHist, effect)

	if len(state.interventionHist) > causalHistoryCap {
		state.interventionHist = state.interventionHist[len(state.interventionHist)-causalHistoryCap:]
	}
}

func (state *CausalSymbol) recordUplift(uplift float64) {
	if uplift <= 0 {
		return
	}

	state.upliftHist = append(state.upliftHist, uplift)

	if len(state.upliftHist) > causalHistoryCap {
		state.upliftHist = state.upliftHist[len(state.upliftHist)-causalHistoryCap:]
	}
}
