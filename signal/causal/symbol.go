package causal

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	nomadaptive "github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/correlation"
	ckernel "github.com/theapemachine/nomagique/kernel/causal"
	"github.com/theapemachine/nomagique/vector"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/numeric"
)

const tradeWindow = 5 * time.Minute

/*
CausalSymbol holds per-symbol Pearl-ladder history and microstructure state.
DAG: MacroMomentum → PriceVelocity ← LocalFlow, Liquidity backdoors macro/flow.

Confidence is how decisively the returned category wins over its neighbors on the
ladder or fallback path; SNR is how surprising that selection is versus the symbol's
own recent baseline, not how large the strength is.
*/
type CausalSymbol struct {
	err            error
	samples        []causalSample
	pendingSamples []pendingCausalSample
	hy             *correlation.WindowSet
	regime         *ckernel.RegimeTracker
	lastPrice      float64
	sessionAnchor  float64
	bid            float64
	ask            float64
	dailyQuoteVol  float64
	changePct      float64
	spreadBPS      float64
	imbalance      float64
	buyPressure    float64
	volumeWindow   *VolumeWindow
	pressure       *nomadaptive.Exponential
	l1Features     *vector.FeatureExtractor
}

func NewCausalSymbol() (*CausalSymbol, error) {
	l1Features, err := newL1FeatureExtractor()

	if err != nil {
		return nil, errnie.Error(err)
	}

	return &CausalSymbol{
		samples:      make([]causalSample, 0, causalHistoryCap),
		volumeWindow: NewVolumeWindow(tradeWindow),
		pressure:     nomadaptive.EMA(),
		hy:           newHYWindowSet(),
		regime:       ckernel.NewRegimeTracker(),
		l1Features:   l1Features,
	}, nil
}

func (state *CausalSymbol) FeedTicker(row market.TickerUpdate) {
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

func (state *CausalSymbol) FeedTrade(tick market.TradeUpdate) error {
	_, err := state.volumeWindow.Next(
		0,
		float64(tick.Timestamp.UnixNano()),
		tick.Qty,
		state.lastPrice,
	)

	if err != nil {
		return errnie.Error(err)
	}

	if tick.Price > 0 {
		state.lastPrice = tick.Price

		if state.sessionAnchor <= 0 {
			state.sessionAnchor = tick.Price
		}

		_, change := numeric.AnchorChange(state.sessionAnchor, tick.Price)

		if change != 0 {
			state.changePct = change
		}

		state.hy.Observe(tick.Timestamp.UnixNano(), tick.Price)
		state.maybeResetHYOnShock()
	}

	sign := -1.0

	if tick.Side == "buy" {
		sign = 1.0
	}

	state.buyPressure = float64(nomagique.Scalar(sign).Observe(state.pressure))

	return nil
}

func (state *CausalSymbol) FeedBook(delta market.BookUpdate) error {
	if len(delta.Bids) == 0 || len(delta.Asks) == 0 {
		return errnie.Error(fmt.Errorf("causal: book update requires bid and ask levels"))
	}

	if err := state.l1Features.SetInput(
		l1InputBidPrice, delta.Bids[0].Price,
	); state.err != nil {
		return errnie.Error(err)
	}

	if err := state.l1Features.SetInput(
		l1InputAskPrice, delta.Asks[0].Price,
	); state.err != nil {
		return errnie.Error(err)
	}

	if err := state.l1Features.SetInput(
		l1InputBidQty, delta.Bids[0].Qty,
	); state.err != nil {
		return errnie.Error(err)
	}

	if err := state.l1Features.SetInput(
		l1InputAskQty, delta.Asks[0].Qty,
	); state.err != nil {
		return errnie.Error(err)
	}

	state.l1Features.Extract()

	if state.bid, state.err = state.l1Features.Input(
		l1InputBidPrice,
	); state.err != nil {
		return errnie.Error(state.err)
	}

	if state.ask, state.err = state.l1Features.Input(
		l1InputAskPrice,
	); state.err != nil {
		return errnie.Error(state.err)
	}

	mid, err := state.l1Features.Feature(l1FeatureMidPrice)

	if err != nil {
		return errnie.Error(err)
	}

	if state.lastPrice <= 0 && mid > 0 {
		state.lastPrice = mid
	}

	if state.spreadBPS, state.err = state.l1Features.Feature(
		l1FeatureSpreadBPS,
	); state.err != nil {
		return errnie.Error(state.err)
	}

	if state.imbalance, state.err = state.l1Features.Feature(
		l1FeatureImbalance,
	); state.err != nil {
		return errnie.Error(state.err)
	}

	return nil
}

func (state *CausalSymbol) Measure(
	macroMomentum, contagion float64, now time.Time,
) (logic.Measurement, error) {
	state.resolvePendingLocked(now)

	batchVolume := state.volumeWindow.Sum()

	if batchVolume > 0 && state.spreadBPS > 0 && state.imbalance != 0 && state.buyPressure != 0 {
		localFlow := batchVolume * (state.buyPressure + 1) / 2
		liquidity := bookLiquidity(state.spreadBPS, batchVolume)

		state.enqueuePendingLocked(macroMomentum, liquidity, localFlow, state.lastPrice, now)

		currentSample := newCausalSample(macroMomentum, liquidity, localFlow, 0)
		outcome, err := state.evaluate(currentSample, contagion)

		if err != nil {
			return logic.Measurement{}, errnie.Error(err)
		}

		if outcome.Raw > 0 {
			category := causalCategory(outcome.Reason)
			confidence := causalEvidence(
				category, outcome, macroMomentum, state.changePct, state.buyPressure, true,
			)

			measurement := logic.Measurement{
				Source:     logic.SourceCausal,
				Symbol:     "",
				Category:   category,
				Strength:   outcome.Raw,
				Confidence: confidence,
				Price:      state.lastPrice,
			}

			if measurement.Strength <= 0 || measurement.Confidence <= 0 {
				return logic.Measurement{}, nil
			}

			return measurement, nil
		}
	}

	reason := "macro_association"

	if state.buyPressure != 0 && state.changePct == 0 {
		reason = "flow_pressure"
	}

	if state.changePct == 0 && macroMomentum == 0 && state.buyPressure == 0 {
		return logic.Measurement{}, nil
	}

	fallbackRaw := math.Max(math.Abs(macroMomentum), math.Abs(state.changePct))
	category := causalCategory(reason)
	confidence := causalEvidence(
		category, ckernel.Outcome{}, macroMomentum, state.changePct, state.buyPressure, false,
	)

	if fallbackRaw <= 0 || confidence <= 0 {
		return logic.Measurement{}, nil
	}

	measurement := logic.Measurement{
		Source:     logic.SourceCausal,
		Category:   category,
		Strength:   fallbackRaw,
		Confidence: confidence,
		Price:      state.lastPrice,
	}

	return measurement, nil
}

func (state *CausalSymbol) ChangePct() float64 {
	return state.changePct
}

func (state *CausalSymbol) spreadPrice() float64 {
	if state.lastPrice <= 0 || state.spreadBPS <= 0 {
		return 0
	}

	return state.lastPrice * state.spreadBPS / 10000
}

func (state *CausalSymbol) symbolRow(
	symbol string,
	macroMomentum float64,
	at time.Time,
) (*market.Symbol, error) {
	if state.lastPrice <= 0 {
		return nil, fmt.Errorf("causal: price is required")
	}

	value := math.Abs(state.changePct)

	if value <= 0 {
		value = magnitudeMargin(math.Abs(state.buyPressure))
	}

	if value <= 0 {
		value = magnitudeMargin(math.Abs(macroMomentum))
	}

	if value <= 0 {
		return nil, fmt.Errorf("causal: value is required")
	}

	volume := state.volumeWindow.Sum()

	if volume <= 0 {
		volume = state.dailyQuoteVol
	}

	if volume <= 0 {
		return nil, fmt.Errorf("causal: volume is required")
	}

	pressure := state.buyPressure

	if pressure == 0 {
		pressure = 1
	}

	return market.NewSymbolRow(symbol, state.lastPrice, value, volume, pressure, at)
}

/*
HYSnapshot returns an independent copy of the symbol's Hayashi-Yoshida return
series so the signal can compute cross-asset correlation without holding this
symbol's lock during the sweep.
*/
func (state *CausalSymbol) HYSnapshot() *correlation.IntervalSeries {
	if state.hy == nil || state.hy.Series() == nil {
		return nil
	}

	_, mediumWindow, _ := contagionWindowsFromAdaptation()

	return state.hy.Series().CloneTail(mediumWindow)
}

func (state *CausalSymbol) HYWindowSnapshot() correlation.WindowSnapshot {
	if state.hy == nil {
		return correlation.WindowSnapshot{}
	}

	return state.hy.Snapshot(contagionTierWindows())
}

func (state *CausalSymbol) maybeResetHYOnShock() {
	if state.hy == nil || state.hy.Series() == nil {
		return
	}

	series := state.hy.Series()
	lastMove := series.LastReturnMagnitude()
	baseline := series.RealizedVolatilityExcludingLast()

	if baseline <= 0 {
		return
	}

	if lastMove < baseline*contagionVolatilityResetSigma() {
		return
	}

	_, _, slowWindow := contagionWindowsFromAdaptation()

	series.Trim(slowWindow)
}

func (state *CausalSymbol) evaluate(current causalSample, contagion float64) (ckernel.Outcome, error) {
	if len(state.samples) < minCausalHistory {
		return ckernel.Outcome{}, nil
	}

	rows := make([][]float64, len(state.samples))

	for index := range state.samples {
		rows[index] = state.samples[index].nodes[:]
	}

	nodeTable, err := ckernel.NewNodeTable(rows, priceVelocityNode, minCausalHistory)

	if err != nil {
		return ckernel.Outcome{}, errnie.Error(err)
	}

	return ckernel.Evaluate(
		nodeTable,
		current.nodes[:],
		contagion,
		ladderConfigFromViper(),
		state.regime,
	), nil
}
