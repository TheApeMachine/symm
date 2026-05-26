package pumpdump

import (
	"math"

	"github.com/theapemachine/symm/engine"
)

const precursorMoveBound = 0.001

/*
measureAlignment scores how completely the current microstructure matches a pump setup.
Each factor is derived only from present observations, not symbol history.
*/
func (state *PumpSymbol) measureAlignment(peakSpike float64) (float64, error) {
	spreadScore, err := state.spreadCompression.Next(0, state.spreadBPS)

	if err != nil {
		return 0, err
	}

	return engine.AlignConfidence(
		engine.ExcessRatio(peakSpike),
		bookAlignment(state.imbalance, state.buyPressure),
		engine.ExcessRatio(spreadScore),
		priceMoveAlignment(state.lastPrice, state.volumeWindow.Anchor()),
	), nil
}

func bookAlignment(imbalance, buyPressure float64) float64 {
	side := math.Min(math.Abs(imbalance), 1)
	pressure := (buyPressure + 1) / 2

	if side <= 0 || pressure <= 0 {
		return 0
	}

	return side * pressure
}

func priceMoveAlignment(lastPrice, anchor float64) float64 {
	if anchor <= 0 || lastPrice <= anchor {
		return 0
	}

	move := (lastPrice - anchor) / anchor

	if move <= 0 {
		return 0
	}

	if move <= precursorMoveBound {
		return move / precursorMoveBound
	}

	return engine.ExcessRatio(move / precursorMoveBound)
}
