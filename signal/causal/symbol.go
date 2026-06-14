package causal

import (
	"errors"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	nomadaptive "github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/nomagique/core"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/nomagique/vector"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

var errBookTouchNotReady = errors.New("causal: book touch is not ready")

/*
CausalSymbol holds per-symbol Pearl-ladder history and microstructure state.
DAG: MacroMomentum → PriceVelocity ← LocalFlow, Liquidity backdoors macro/flow.

Confidence is how decisively the returned category wins over its neighbors on the
ladder or fallback path; SNR is how surprising that selection is versus the symbol's
own recent baseline, not how large the strength is.
*/
type CausalSymbol struct {
	err              error
	samples          []causalSample
	pendingSamples   []pendingCausalSample
	hy               *correlation.WindowSet
	regime           *causal.RegimeTracker
	pearl            *algorithm.Pearl
	lastPrice        float64
	sessionAnchor    float64
	bid              float64
	ask              float64
	dailyQuoteVol    float64
	changePct        float64
	spreadBPS        float64
	spreadBPSHistory []float64
	imbalance        float64
	buyPressure      float64
	volumeWindow     *VolumeWindow
	pressure         *nomadaptive.Exponential
	l1Features       *vector.FeatureExtractor
}

func NewCausalSymbol() (*CausalSymbol, error) {
	runtimeConfig := loadRuntimeConfig()

	l1Features, err := vector.NewL1BookExtractor()

	if err != nil {
		return nil, errnie.Error(err)
	}

	return &CausalSymbol{
		samples:      make([]causalSample, 0, causalHistoryCap),
		volumeWindow: NewVolumeWindow(tradeWindow),
		pressure:     nomadaptive.EMA(),
		hy:           runtimeConfig.newHYWindowSet(),
		regime:       causal.NewRegimeTracker(),
		l1Features:   l1Features,
	}, nil
}

func (state *CausalSymbol) FeedTicker(row krakenmarket.TickerUpdate) {
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

func (state *CausalSymbol) FeedTrade(tick krakenmarket.TradeUpdate) error {
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

		_, change := anchorChange(state.sessionAnchor, tick.Price)

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

func (state *CausalSymbol) resolveBookTouch(
	delta krakenmarket.BookUpdate,
) (bidPrice, askPrice, bidQty, askQty float64, err error) {
	if len(delta.Bids) == 0 && len(delta.Asks) == 0 {
		return 0, 0, 0, 0, errBookTouchNotReady
	}

	bidPrice = state.bid
	askPrice = state.ask

	if len(delta.Bids) > 0 {
		bidPrice = delta.Bids[0].Price
		bidQty = delta.Bids[0].Qty
	}

	if len(delta.Asks) > 0 {
		askPrice = delta.Asks[0].Price
		askQty = delta.Asks[0].Qty
	}

	if bidPrice <= 0 || askPrice <= 0 {
		return 0, 0, 0, 0, errBookTouchNotReady
	}

	if bidQty <= 0 {
		bidQty, err = state.l1Features.Input(vector.L1BidQty)

		if err != nil {
			return 0, 0, 0, 0, errBookTouchNotReady
		}
	}

	if askQty <= 0 {
		askQty, err = state.l1Features.Input(vector.L1AskQty)

		if err != nil {
			return 0, 0, 0, 0, errBookTouchNotReady
		}
	}

	return bidPrice, askPrice, bidQty, askQty, nil
}

func (state *CausalSymbol) FeedBook(delta krakenmarket.BookUpdate) error {
	bidPrice, askPrice, bidQty, askQty, err := state.resolveBookTouch(delta)

	if err != nil {
		if errors.Is(err, errBookTouchNotReady) {
			return err
		}

		return errnie.Error(err)
	}

	if err := state.l1Features.SetInput(vector.L1BidPrice, bidPrice); err != nil {
		return errnie.Error(err)
	}

	if err := state.l1Features.SetInput(vector.L1AskPrice, askPrice); err != nil {
		return errnie.Error(err)
	}

	if err := state.l1Features.SetInput(vector.L1BidQty, bidQty); err != nil {
		return errnie.Error(err)
	}

	if err := state.l1Features.SetInput(vector.L1AskQty, askQty); err != nil {
		return errnie.Error(err)
	}

	state.l1Features.Extract()

	if state.bid, state.err = state.l1Features.Input(vector.L1BidPrice); state.err != nil {
		return errnie.Error(state.err)
	}

	if state.ask, state.err = state.l1Features.Input(vector.L1AskPrice); state.err != nil {
		return errnie.Error(state.err)
	}

	mid, err := state.l1Features.Feature(vector.L1MidPrice)

	if err != nil {
		return errnie.Error(err)
	}

	if state.lastPrice <= 0 && mid > 0 {
		state.lastPrice = mid
	}

	if state.spreadBPS, state.err = state.l1Features.Feature(vector.L1SpreadBPS); state.err != nil {
		return errnie.Error(state.err)
	}

	state.recordSpreadBPS(state.spreadBPS)

	if state.imbalance, state.err = state.l1Features.Feature(vector.L1Imbalance); state.err != nil {
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
		spreadFloor := state.spreadBPSFloor()
		localFlow := batchVolume * state.buyPressure
		liquidity := bookLiquidity(state.spreadBPS, spreadFloor, batchVolume)

		if spreadFloor <= 0 || liquidity <= 0 {
			return logic.Measurement{}, nil
		}

		state.enqueuePendingLocked(macroMomentum, liquidity, localFlow, state.lastPrice, now)

		currentSample := newCausalSample(macroMomentum, liquidity, localFlow, 0)
		outcome, err := state.evaluate(currentSample, contagion)

		if err != nil {
			return logic.Measurement{}, errnie.Error(err)
		}

		if outcome.Raw > 0 {
			category := causalCategory(outcome.Reason)
			confidence, err := causalShareConfidence(
				category, outcome, macroMomentum, state.changePct, state.buyPressure, true,
			)

			if err != nil {
				return logic.Measurement{}, errnie.Error(err)
			}

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

	if state.changePct == 0 && macroMomentum == 0 && state.buyPressure == 0 {
		return logic.Measurement{}, nil
	}

	fallbackRaw := math.Max(math.Abs(macroMomentum), math.Abs(state.changePct))
	category := logic.CategoryCausalNoise
	confidence, err := causalShareConfidence(
		category, causal.Outcome{}, macroMomentum, state.changePct, state.buyPressure, false,
	)

	if err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

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

	return state.hy.Snapshot(loadRuntimeConfig().contagionTierWindows())
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

	if lastMove < baseline*loadRuntimeConfig().ContagionVolatilityResetSigma {
		return
	}

	_, _, slowWindow := contagionWindowsFromAdaptation()

	series.Trim(slowWindow)
}

func (state *CausalSymbol) evaluate(
	current causalSample,
	contagion float64,
) (causal.Outcome, error) {
	if len(state.samples) < minCausalHistory {
		return causal.Outcome{}, nil
	}

	state.ensurePearl()
	state.pearl.ReplaceStreams(state.nodeStreams())
	state.pearl.SetContagion(nomagique.Scalar(contagion))

	_ = state.pearl.Observe(
		nomagique.Scalar(current.nodes[macroMomentumNode]),
		nomagique.Scalar(current.nodes[liquidityNode]),
		nomagique.Scalar(current.nodes[localFlowNode]),
		nomagique.Scalar(current.nodes[priceVelocityNode]),
	)

	return state.pearl.Outcome(), nil
}

func (state *CausalSymbol) ensurePearl() {
	if state.pearl != nil {
		return
	}

	state.pearl = algorithm.NewPearl(
		priceVelocityNode,
		loadRuntimeConfig().ladderConfig(),
		state.nodeStreams(),
		nomagique.Scalar(0),
		nil,
		state.regime,
	)
}

func (state *CausalSymbol) nodeStreams() []core.Numbers {
	if len(state.samples) == 0 {
		return nil
	}

	macroMomentum := make([]float64, len(state.samples))
	liquidity := make([]float64, len(state.samples))
	localFlow := make([]float64, len(state.samples))
	priceVelocity := make([]float64, len(state.samples))

	for index := range state.samples {
		macroMomentum[index] = state.samples[index].nodes[macroMomentumNode]
		liquidity[index] = state.samples[index].nodes[liquidityNode]
		localFlow[index] = state.samples[index].nodes[localFlowNode]
		priceVelocity[index] = state.samples[index].nodes[priceVelocityNode]
	}

	return []core.Numbers{
		nomagique.Numbers(macroMomentum...),
		nomagique.Numbers(liquidity...),
		nomagique.Numbers(localFlow...),
		nomagique.Numbers(priceVelocity...),
	}
}

func anchorChange(anchor, price float64) (float64, float64) {
	if anchor <= 0 || price <= 0 {
		return anchor, 0
	}

	return anchor, (price - anchor) / anchor
}
