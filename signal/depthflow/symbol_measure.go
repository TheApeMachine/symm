package depthflow

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

func (state *DepthSymbol) Measure() (perspectives.Measurement, float64, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.bookDiverged || !state.bookReady {
		return state.measureTradePressureLocked()
	}

	bids := state.book.Bids
	asks := state.book.Asks
	mid := state.last

	if len(bids) > 0 && len(asks) > 0 {
		mid = (bids[0].Price + asks[0].Price) / 2
	}

	if mid <= 0 && state.bid > 0 && state.ask > 0 {
		mid = (state.bid + state.ask) / 2
	}

	if mid <= 0 {
		return perspectives.Measurement{}, 0, nil
	}

	if len(bids) == 0 || len(asks) == 0 {
		return state.measureTradePressureLocked()
	}

	imbalance, ok := state.weightedImbalanceLocked(bids, asks, mid)
	level1, levelOK := state.level1ImbalanceLocked(bids, asks)

	if !ok || !levelOK {
		return state.measureTradePressureLocked()
	}

	flatImbalance, flatOK := state.flatImbalanceLocked(bids, asks)

	if imbalance == 0 {
		category, evidence := depthflowReading("", imbalance, flatImbalance, flatOK, 0)
		standout := perspectives.UnitMagnitudeMargin(0)
		confidence, err := state.tracked.Observe(category, evidence, standout)

		if err != nil {
			return perspectives.Measurement{}, 0, err
		}

		return perspectives.Measurement{
			Symbol:     state.symbol,
			Source:     perspectives.SourceDepthFlow,
			Category:   category,
			Last:       state.last,
			SpreadBPS:  state.spreadBPSLocked(),
			Strength:   0,
			Confidence: confidence,
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
			return perspectives.Measurement{}, 0, err
		}

		if raw > 0 {
			category, evidence := depthflowReading(
				reasonDepthImbalance, imbalance, flatImbalance, flatOK, 0,
			)

			// evidence is how cleanly the book lands in its structural category;
			// standout is the strength of the imbalance itself, scored by SNR
			// against this symbol's own history. Different questions, different
			// numbers.
			standout := perspectives.UnitMagnitudeMargin(raw)
			confidence, err := state.tracked.Observe(category, evidence, standout)

			if err != nil {
				return perspectives.Measurement{}, 0, err
			}

			return perspectives.Measurement{
				Symbol:     state.symbol,
				Source:     perspectives.SourceDepthFlow,
				Category:   category,
				Last:       state.last,
				SpreadBPS:  state.spreadBPSLocked(),
				Strength:   raw,
				Confidence: confidence,
			}, standout, nil
		}
	}

	raw := math.Abs(level1)
	category, evidence := depthflowReading(
		reasonDepthSkeptic, imbalance, flatImbalance, flatOK, 0,
	)
	standout := perspectives.UnitMagnitudeMargin(raw)

	confidence, err := state.tracked.Observe(category, evidence, standout)

	if err != nil {
		return perspectives.Measurement{}, 0, err
	}

	return perspectives.Measurement{
		Symbol:     state.symbol,
		Source:     perspectives.SourceDepthFlow,
		Category:   category,
		Last:       state.last,
		SpreadBPS:  state.spreadBPSLocked(),
		Strength:   raw,
		Confidence: confidence,
	}, standout, nil
}

func (state *DepthSymbol) measureTradePressureLocked() (perspectives.Measurement, float64, error) {
	flow := math.Abs(state.buyPressure)

	if flow <= 0 {
		flow = math.Abs(state.pressure.Value())
	}

	if flow <= 0 {
		return perspectives.Measurement{}, 0, nil
	}

	category, evidence := depthflowReading("trade_pressure", 0, 0, false, flow)
	standout := perspectives.UnitMagnitudeMargin(flow)

	confidence, err := state.tracked.Observe(category, evidence, standout)

	if err != nil {
		return perspectives.Measurement{}, 0, err
	}

	return perspectives.Measurement{
		Symbol:     state.symbol,
		Source:     perspectives.SourceDepthFlow,
		Category:   category,
		Last:       state.last,
		SpreadBPS:  state.spreadBPSLocked(),
		Strength:   flow,
		Confidence: confidence,
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
