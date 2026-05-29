package causal

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
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
	mu                sync.RWMutex
	pair              asset.Pair
	samples           []causalSample
	pendingSamples    []pendingCausalSample
	interventionHist  map[string][]float64
	upliftHist        map[string][]float64
	confidenceHistory []float64
	hy                *hyReturns
	lastPrice         float64
	bid               float64
	ask               float64
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
		pair:             pair,
		samples:          make([]causalSample, 0, causalHistoryCap),
		interventionHist: make(map[string][]float64),
		upliftHist:       make(map[string][]float64),
		volumeWindow:     adaptive.NewWindow(tradeWindow),
		pressure:         adaptive.NewEMA(0),
		calibrator:       engine.NewPredictionCalibrator(params),
		hy:               newHYReturns(contagionWindow()),
	}
}

func (state *CausalSymbol) FeedTicker(row market.TickerRow) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if row.Last > 0 {
		state.lastPrice = row.Last
		state.dailyQuoteVol = row.Volume * row.Last
	}

	if row.Bid > 0 {
		state.bid = row.Bid
	}

	if row.Ask > 0 {
		state.ask = row.Ask
	}

	state.changePct = row.ChangePct
}

func (state *CausalSymbol) FeedTrade(tick trade.Data) {
	state.mu.Lock()
	defer state.mu.Unlock()

	errnie.Does(func() (float64, error) {
		return state.volumeWindow.Next(
			0,
			float64(tick.Timestamp.UnixNano()),
			tick.Qty,
			state.lastPrice,
		)
	}).Or(func(err error) {
		errnie.Error(err)
	})

	// Feed the asynchronous price print into the Hayashi-Yoshida return series that backs
	// cross-asset contagion detection. Trade prints carry both a price and a timestamp, so
	// they are the natural clock for an estimator built to tolerate non-synchronous sampling.
	if tick.Price > 0 {
		state.hy.Observe(tick.Timestamp.UnixNano(), tick.Price)
	}

	sign := -1.0

	if tick.Side == "buy" {
		sign = 1.0
	}

	state.buyPressure = errnie.Does(func() (float64, error) {
		return state.pressure.Next(0, sign)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()
}

func (state *CausalSymbol) FeedBook(delta market.BookLevelsDelta) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if len(delta.Bids) == 0 || len(delta.Asks) == 0 {
		return
	}

	bid := delta.Bids[0].Price
	ask := delta.Asks[0].Price
	mid := (bid + ask) / 2

	state.bid = bid
	state.ask = ask

	if state.lastPrice <= 0 && mid > 0 {
		state.lastPrice = mid
	}

	if mid > 0 {
		state.spreadBPS = (ask - bid) / mid * 10000
	}

	total := delta.Bids[0].Volume + delta.Asks[0].Volume

	if total > 0 {
		state.imbalance = (delta.Bids[0].Volume - delta.Asks[0].Volume) / total
	}
}

func (state *CausalSymbol) Measure(macroMomentum, contagion float64, now time.Time) (engine.Measurement, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.lastPrice <= 0 {
		return engine.Measurement{}, false
	}

	// Promote any pending readings whose forward return has matured.
	state.resolvePendingLocked(now)

	batchVolume := state.volumeWindow.Sum()
	reason := "macro_association"
	confidence := 0.0

	if batchVolume > 0 && state.spreadBPS > 0 && state.imbalance != 0 && state.buyPressure != 0 {
		localFlow := batchVolume * (state.buyPressure + 1) / 2
		liquidity := bookLiquidity(state.spreadBPS, batchVolume)

		// Record this reading; it will be labeled with its forward return
		// causalForwardWindow from now.
		state.enqueuePendingLocked(macroMomentum, liquidity, localFlow, state.lastPrice, now)

		// Predict the forward velocity for the current feature vector using the
		// model fitted on already-labeled (forward) samples.
		currentSample := newCausalSample(macroMomentum, liquidity, localFlow, 0)
		fullConfidence, fullReason := state.evaluate(currentSample, contagion)

		if fullConfidence > 0 {
			return engine.Measurement{
				Type:       engine.Causal,
				Source:     causalSource,
				Regime:     "causal",
				Reason:     fullReason,
				Category:   causalCategory(fullReason),
				Pairs:      []asset.Pair{state.pair},
				Confidence: fullConfidence,
				Last:       state.lastPrice,
				Bid:        state.bid,
				Ask:        state.ask,
			}, true
		}
	}

	confidence = engine.AlignConfidence(
		engine.ConfidenceFromScore(math.Abs(state.changePct)),
		engine.ConfidenceFromScore(math.Abs(macroMomentum)),
	)

	if confidence <= 0 && state.buyPressure != 0 {
		confidence = engine.ConfidenceFromScore(math.Abs(state.buyPressure))
		reason = "flow_pressure"
	}

	if confidence <= 0 && state.changePct != 0 {
		confidence = engine.ConfidenceFromScore(math.Abs(state.changePct))
	}

	if confidence <= 0 {
		return engine.Measurement{}, false
	}

	return engine.Measurement{
		Type:       engine.Causal,
		Source:     causalSource,
		Regime:     "causal",
		Reason:     reason,
		Category:   causalCategory(reason),
		Pairs:      []asset.Pair{state.pair},
		Confidence: confidence,
		Last:       state.lastPrice,
		Bid:        state.bid,
		Ask:        state.ask,
	}, true
}

/*
causalCategory maps the causal reason onto the structural-origin perspective:
a validated local-flow driver is endogenous alpha; the panic (regime-inverted)
roles mean liquidity itself is driving price (a shock); a macro-only read is
systemic beta (the asset is a passenger); a bare flow-pressure fallback is
causal noise (no statistically grounded driver).
*/
func causalCategory(reason string) engine.Category {
	switch reason {
	case "intervention", "counterfactual_like":
		return engine.CatEndogenousAlpha
	case "intervention_regime_inversion", "counterfactual_like_regime_inversion":
		return engine.CatLiquidityShock
	case "macro_association":
		return engine.CatSystemicBeta
	default:
		return engine.CatCausalNoise
	}
}

func (state *CausalSymbol) ApplyFeedback(feedback engine.PredictionFeedback) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.calibrator.Apply(feedback)
}

func (state *CausalSymbol) ChangePct() float64 {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.changePct
}

/*
HYSnapshot returns an independent copy of the symbol's Hayashi-Yoshida return series so the
signal can compute cross-asset correlation without holding this symbol's lock during the sweep.
*/
func (state *CausalSymbol) HYSnapshot() *hyReturns {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.hy.clone()
}

func (state *CausalSymbol) evaluate(current causalSample, contagion float64) (float64, string) {
	if len(state.samples) < minCausalHistory {
		return 0, ""
	}

	samples := state.samples
	roles, inverted := selectRoles(samples, contagion)
	suffix := ""

	if inverted {
		// The edges have flipped: liquidity is now the driving treatment and local flow a
		// lagging response. Tag the reason so the inversion is visible downstream.
		suffix = "_regime_inversion"
	}

	association := associationEffectFor(samples, roles)
	intervention := kernelBackdoorEffectFor(samples, roles) * state.calibrator.Scale()

	if intervention <= 0 {
		state.recordIntervention(roles.label, intervention)

		return 0, ""
	}

	// Normalize against the fence BEFORE recording: otherwise the fresh
	// observation is part of the percentile cohort that defines its own
	// gate, which half-saturates the gate on every reading. The fence
	// represents what we have seen, not what we are currently seeing. The
	// fences are kept per regime so flow-effect and liquidity-effect
	// magnitudes — which live on different scales — never contaminate one another.
	interventionNormalized := state.calibrator.NormalizeConfidence(
		intervention, state.interventionHist[roles.label],
	)
	state.recordIntervention(roles.label, intervention)

	model, fitOK := fitNonLinearStructuralFor(samples, roles)

	if !fitOK {
		return engine.ProvisionalConfidence(
			interventionNormalized, intervention,
		), "intervention" + suffix
	}

	interventionFlow := flowInterventionLevelFor(samples, roles)
	uplift := nonLinearCounterfactualUpliftFor(current, model, interventionFlow, roles)

	if uplift <= 0 {
		normalized := engine.ProvisionalConfidence(
			interventionNormalized, intervention,
		)
		state.recordConfidence(intervention)

		return normalized, "intervention" + suffix
	}

	upliftNormalized := state.calibrator.NormalizeConfidence(
		uplift, state.upliftHist[roles.label],
	)
	state.recordUplift(roles.label, uplift)

	confounded := math.Abs(intervention-association) > math.Abs(association)*0.25
	reason := "intervention" + suffix

	if confounded && uplift > 0 {
		reason = "counterfactual_like" + suffix
	}

	interventionScore := engine.ProvisionalConfidence(
		interventionNormalized, intervention,
	)
	upliftScore := engine.ProvisionalConfidence(
		upliftNormalized, uplift,
	)
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

func (state *CausalSymbol) recordIntervention(label string, effect float64) {
	if effect == 0 {
		return
	}

	history := append(state.interventionHist[label], effect)

	if len(history) > causalHistoryCap {
		history = history[len(history)-causalHistoryCap:]
	}

	state.interventionHist[label] = history
}

func (state *CausalSymbol) recordUplift(label string, uplift float64) {
	if uplift <= 0 {
		return
	}

	history := append(state.upliftHist[label], uplift)

	if len(history) > causalHistoryCap {
		history = history[len(history)-causalHistoryCap:]
	}

	state.upliftHist[label] = history
}
