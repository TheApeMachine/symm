package causal

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const tradeWindow = 5 * time.Minute

/*
CausalSymbol holds per-symbol Pearl-ladder history and microstructure state.
DAG: MacroMomentum → PriceVelocity ← LocalFlow, Liquidity backdoors macro/flow.

SNR is the backdoor-adjusted intervention effect relative to its own per-regime
noise floor — a do(flow) effect standing above the symbol's typical clears the
floor; a confounded, macro-driven move does not.
*/
type CausalSymbol struct {
	mu             sync.RWMutex
	samples        []causalSample
	pendingSamples []pendingCausalSample
	noise          map[string]*adaptive.EMA
	hy             *hyReturns
	lastPrice      float64
	bid            float64
	ask            float64
	dailyQuoteVol  float64
	changePct      float64
	spreadBPS      float64
	imbalance      float64
	buyPressure    float64
	volumeWindow   *adaptive.Window
	pressure       *adaptive.EMA
	floor          *adaptive.SNR
}

func NewCausalSymbol() *CausalSymbol {
	return &CausalSymbol{
		samples:      make([]causalSample, 0, causalHistoryCap),
		noise:        make(map[string]*adaptive.EMA),
		volumeWindow: adaptive.NewWindow(tradeWindow),
		pressure:     adaptive.NewEMA(0),
		hy:           newHYReturns(contagionWindow()),
		floor:        adaptive.NewSNR(),
	}
}

func (state *CausalSymbol) FeedTicker(row market.TickerUpdate) {
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

func (state *CausalSymbol) FeedTrade(tick market.TradeUpdate) {
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

func (state *CausalSymbol) FeedBook(delta market.BookUpdate) {
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

	total := delta.Bids[0].Qty + delta.Asks[0].Qty

	if total > 0 {
		state.imbalance = (delta.Bids[0].Qty - delta.Asks[0].Qty) / total
	}
}

func (state *CausalSymbol) Measure(macroMomentum, contagion float64, now time.Time) (perspectives.Measurement, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.lastPrice <= 0 {
		return perspectives.Measurement{}, false
	}

	state.resolvePendingLocked(now)

	batchVolume := state.volumeWindow.Sum()

	if batchVolume > 0 && state.spreadBPS > 0 && state.imbalance != 0 && state.buyPressure != 0 {
		localFlow := batchVolume * (state.buyPressure + 1) / 2
		liquidity := bookLiquidity(state.spreadBPS, batchVolume)

		state.enqueuePendingLocked(macroMomentum, liquidity, localFlow, state.lastPrice, now)

		currentSample := newCausalSample(macroMomentum, liquidity, localFlow, 0)
		snr, reason := state.evaluate(currentSample, contagion)

		if snr > 0 {
			return perspectives.Measurement{
				Source:   perspectives.SourceCausal,
				Category: causalCategory(reason),
				SNR:      snr,
			}, true
		}
	}

	// Fallback: no statistically grounded local driver. Emit the passenger /
	// noise read at the baseline floor so it informs but does not fire actions.
	reason := "macro_association"

	if state.buyPressure != 0 && state.changePct == 0 {
		reason = "flow_pressure"
	}

	if state.changePct == 0 && macroMomentum == 0 && state.buyPressure == 0 {
		return perspectives.Measurement{}, false
	}

	return perspectives.Measurement{
		Source:   perspectives.SourceCausal,
		Category: causalCategory(reason),
		SNR:      1,
	}, true
}

/*
causalCategory maps the causal reason onto the structural-origin perspective:
a validated local-flow driver is endogenous alpha; the panic (regime-inverted)
roles mean liquidity itself is driving price (a shock); a macro-only read is
systemic beta (the asset is a passenger); a bare flow-pressure fallback is
causal noise (no statistically grounded driver).
*/
func causalCategory(reason string) perspectives.CategoryType {
	switch reason {
	case "intervention", "counterfactual_like":
		return perspectives.CategoryEndogenousAlpha
	case "intervention_regime_inversion", "counterfactual_like_regime_inversion":
		return perspectives.CategoryLiquidityShock
	case "macro_association":
		return perspectives.CategorySystemicBeta
	default:
		return perspectives.CategoryCausalNoise
	}
}

func (state *CausalSymbol) ChangePct() float64 {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.changePct
}

/*
HYSnapshot returns an independent copy of the symbol's Hayashi-Yoshida return
series so the signal can compute cross-asset correlation without holding this
symbol's lock during the sweep.
*/
func (state *CausalSymbol) HYSnapshot() *hyReturns {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.hy.clone()
}

/*
snrFor scores one causal effect against its own per-regime running noise floor,
then folds the reading into the floor. The first reading is neutral (1).
*/
func (state *CausalSymbol) snrFor(label string, effect float64) float64 {
	floorEMA := state.noise[label]

	if floorEMA == nil {
		floorEMA = adaptive.NewEMA(0)
		state.noise[label] = floorEMA
	}

	floor := floorEMA.Value()
	_, _ = floorEMA.Next(0, effect)

	if floor <= 0 {
		return 1
	}

	return effect / floor
}

func (state *CausalSymbol) evaluate(current causalSample, contagion float64) (float64, string) {
	if len(state.samples) < minCausalHistory {
		return 0, ""
	}

	samples := state.samples
	roles, inverted := selectRoles(samples, contagion)
	suffix := ""

	if inverted {
		suffix = "_regime_inversion"
	}

	association := associationEffectFor(samples, roles)
	intervention := kernelBackdoorEffectFor(samples, roles)

	if intervention <= 0 {
		return 0, ""
	}

	snr := state.snrFor(roles.label, intervention)

	model, fitOK := fitNonLinearStructuralFor(samples, roles)

	if !fitOK {
		return snr, "intervention" + suffix
	}

	interventionFlow := flowInterventionLevelFor(samples, roles)
	uplift := nonLinearCounterfactualUpliftFor(current, model, interventionFlow, roles)

	if uplift <= 0 {
		return snr, "intervention" + suffix
	}

	confounded := math.Abs(intervention-association) > math.Abs(association)*0.25
	reason := "intervention" + suffix

	if confounded {
		reason = "counterfactual_like" + suffix
	}

	return snr, reason
}
