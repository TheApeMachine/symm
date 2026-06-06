package depthflow

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives/types"
)

func (state *DepthSymbol) Measure() (types.Measurement, float64, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.bookDiverged || !state.bookReady {
		return state.measureTradePressureLocked()
	}

	bids := state.book.Bids
	asks := state.book.Asks
	last := state.quoteLastLocked()

	if last <= 0 {
		return types.Measurement{}, 0, nil
	}

	mid := last

	if len(bids) == 0 || len(asks) == 0 {
		return state.measureTradePressureLocked()
	}

	imbalance, ok := state.weightedImbalanceLocked(bids, asks, mid)
	level1, levelOK := state.level1ImbalanceLocked(bids, asks)

	if !ok || !levelOK {
		return state.measureTradePressureLocked()
	}

	flatImbalance, flatOK := state.flatImbalanceLocked(bids, asks)
	imbalance = types.AdjustSourceValue(types.SourceDepthFlow, imbalance)
	level1 = types.AdjustSourceValue(types.SourceDepthFlow, level1)

	if flatOK {
		flatImbalance = types.AdjustSourceValue(types.SourceDepthFlow, flatImbalance)
	}

	if imbalance == 0 {
		category, evidence := depthflowReading("", imbalance, flatImbalance, flatOK, 0)
		standout := evidence

		if err := state.tracked.Observe(category, evidence); err != nil {
			return types.Measurement{}, 0, err
		}

		return types.Measurement{
			Symbol:     state.symbol,
			Source:     types.SourceDepthFlow,
			Category:   category,
			Last:       last,
			SpreadBPS:  state.spreadBPSLocked(),
			Strength:   evidence,
			Confidence: evidence,
		}, standout, nil
	}

	spoofed := state.isSpoofSkewLocked(imbalance, level1)

	if flatOK {
		spoofed = spoofed || state.isSpoofSkewLocked(flatImbalance, level1)
	}

	if !spoofed {
		pressure := 1.0

		if state.buyPressure > 0 && imbalance > 0 {
			pressure = (state.buyPressure + 1) / 2
		}

		if state.buyPressure < 0 && imbalance < 0 {
			pressure = (1 - state.buyPressure) / 2
		}

		raw, err := state.score.Push(math.Abs(imbalance), pressure)

		if err != nil {
			return types.Measurement{}, 0, err
		}

		if raw > 0 {
			category, evidence := depthflowReading(
				reasonDepthImbalance, imbalance, flatImbalance, flatOK, 0,
			)

			// evidence is how cleanly the book lands in its structural category;
			// standout is the strength of the imbalance itself, scored by SNR
			// against this symbol's own history. Different questions, different
			// numbers.
			standout := evidence

			if err := state.tracked.Observe(category, evidence); err != nil {
				return types.Measurement{}, 0, err
			}

			return types.Measurement{
				Symbol:     state.symbol,
				Source:     types.SourceDepthFlow,
				Category:   category,
				Last:       last,
				SpreadBPS:  state.spreadBPSLocked(),
				Strength:   raw,
				Confidence: evidence,
			}, standout, nil
		}
	}

	raw := math.Abs(level1)
	category, evidence := depthflowReading(
		reasonDepthSkeptic, imbalance, flatImbalance, flatOK, 0,
	)
	standout := evidence

	if err := state.tracked.Observe(category, evidence); err != nil {
		return types.Measurement{}, 0, err
	}

	return types.Measurement{
		Symbol:     state.symbol,
		Source:     types.SourceDepthFlow,
		Category:   category,
		Last:       last,
		SpreadBPS:  state.spreadBPSLocked(),
		Strength:   raw,
		Confidence: evidence,
	}, standout, nil
}

func (state *DepthSymbol) measureTradePressureLocked() (types.Measurement, float64, error) {
	last := state.quoteLastLocked()

	if last <= 0 {
		return types.Measurement{}, 0, nil
	}

	flow := math.Abs(state.buyPressure)

	if flow <= 0 {
		flow = math.Abs(state.pressure.Value())
	}

	flow = types.AdjustSourceValue(types.SourceDepthFlow, flow)

	if flow <= 0 {
		return types.Measurement{}, 0, nil
	}

	category, evidence := depthflowReading("trade_pressure", 0, 0, false, flow)
	standout := evidence

	if err := state.tracked.Observe(category, evidence); err != nil {
		return types.Measurement{}, 0, err
	}

	return types.Measurement{
		Symbol:     state.symbol,
		Source:     types.SourceDepthFlow,
		Category:   category,
		Last:       last,
		SpreadBPS:  state.spreadBPSLocked(),
		Strength:   flow,
		Confidence: evidence,
	}, standout, nil
}

func (state *DepthSymbol) spreadBPSLocked() float64 {
	if state.bid > 0 && state.ask > 0 && state.ask >= state.bid {
		mid := (state.bid + state.ask) / 2

		if mid > 0 {
			return (state.ask - state.bid) / mid * 10000
		}
	}

	bids := state.book.Bids
	asks := state.book.Asks

	if len(bids) == 0 || len(asks) == 0 {
		return 0
	}

	mid := (bids[0].Price + asks[0].Price) / 2

	if mid <= 0 {
		return 0
	}

	return (asks[0].Price - bids[0].Price) / mid * 10000
}
