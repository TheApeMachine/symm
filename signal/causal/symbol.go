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

Confidence is category clarity: how decisively the returned category wins over its
neighbors on the ladder or fallback path; SNR is how surprising that clarity is
versus the symbol's own recent baseline, not how large the strength is.
*/
type CausalSymbol struct {
	mu             sync.RWMutex
	samples        []causalSample
	pendingSamples []pendingCausalSample
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
	tracked        *perspectives.Category
}

func NewCausalSymbol() *CausalSymbol {
	return &CausalSymbol{
		samples:      make([]causalSample, 0, causalHistoryCap),
		volumeWindow: adaptive.NewWindow(tradeWindow),
		pressure:     adaptive.NewEMA(0),
		hy:           newHYReturns(contagionWindow()),
		tracked:      perspectives.NewCategory(perspectives.CategoryTypeNone),
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

	_, _ = state.volumeWindow.Next(
		0,
		float64(tick.Timestamp.UnixNano()),
		tick.Qty,
		state.lastPrice,
	)

	if tick.Price > 0 {
		state.hy.Observe(tick.Timestamp.UnixNano(), tick.Price)
	}

	sign := -1.0

	if tick.Side == "buy" {
		sign = 1.0
	}

	pressure, err := state.pressure.Next(0, sign)

	if err != nil {
		errnie.Error(err)
		return
	}

	state.buyPressure = pressure
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

func (state *CausalSymbol) Measure(
	macroMomentum, contagion float64,
	now time.Time,
) (perspectives.Measurement, bool) {
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
		outcome := state.evaluate(currentSample, contagion)

		if outcome.raw > 0 {
			category := causalCategory(outcome.reason)
			evidence := causalEvidence(
				category, outcome, macroMomentum, state.changePct, state.buyPressure, true,
			)

			confidence, err := state.tracked.Observe(category, evidence)

			if err != nil {
				errnie.Error(err)

				return perspectives.Measurement{}, false
			}

			return perspectives.Measurement{
				Source:     perspectives.SourceCausal,
				Category:   category,
				Strength:   outcome.raw,
				Confidence: confidence,
				Last:       state.lastPrice,
			}, true
		}
	}

	reason := "macro_association"

	if state.buyPressure != 0 && state.changePct == 0 {
		reason = "flow_pressure"
	}

	if state.changePct == 0 && macroMomentum == 0 && state.buyPressure == 0 {
		return perspectives.Measurement{}, false
	}

	fallbackRaw := math.Max(math.Abs(macroMomentum), math.Abs(state.changePct))
	category := causalCategory(reason)
	evidence := causalEvidence(
		category, causalOutcome{}, macroMomentum, state.changePct, state.buyPressure, false,
	)

	confidence, err := state.tracked.Observe(category, evidence)

	if err != nil {
		errnie.Error(err)

		return perspectives.Measurement{}, false
	}

	return perspectives.Measurement{
		Source:     perspectives.SourceCausal,
		Category:   category,
		Strength:   fallbackRaw,
		Confidence: confidence,
		Last:       state.lastPrice,
	}, true
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

func (state *CausalSymbol) evaluate(current causalSample, contagion float64) causalOutcome {
	if len(state.samples) < minCausalHistory {
		return causalOutcome{}
	}

	nodeTable, err := causalTable(state.samples)

	if err != nil {
		return causalOutcome{}
	}

	roles, inverted, condition := selectRolesFromTable(nodeTable, contagion)
	suffix := ""

	if inverted {
		suffix = "_regime_inversion"
	}

	association := associationEffectFromTable(nodeTable, roles)
	intervention := kernelBackdoorEffectFromTable(nodeTable, roles)

	outcome := causalOutcome{
		intervention: intervention,
		association:  association,
		inverted:     inverted,
		contagion:    contagion,
		condition:    condition,
	}

	if intervention <= 0 {
		return outcome
	}

	model, fitOK := fitNonLinearTable(nodeTable, roles.predictors())

	if !fitOK {
		outcome.raw = intervention
		outcome.reason = "intervention" + suffix

		return outcome
	}

	interventionFlow := flowInterventionLevelFromTable(nodeTable, roles)
	uplift := nonLinearCounterfactualUpliftFor(current, model, interventionFlow, roles)
	outcome.uplift = uplift

	if uplift <= 0 {
		outcome.raw = intervention
		outcome.reason = "intervention" + suffix

		return outcome
	}

	confounded := math.Abs(intervention-association) > math.Abs(association)*confoundFraction
	outcome.reason = "intervention" + suffix

	if confounded {
		outcome.reason = "counterfactual_like" + suffix
	}

	outcome.raw = intervention

	return outcome
}

func pairConditionNumber(samples []causalSample) float64 {
	nodeTable, err := causalTable(samples)

	if err != nil {
		return 0
	}

	condition, err := nodeTable.PairConditionNumber(liquidityNode, localFlowNode)

	if err != nil {
		return 0
	}

	if math.IsInf(condition, -1) {
		return 0
	}

	return condition
}
