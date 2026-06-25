package exhaust

import (
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/statutil"
)

type symbolHistory struct {
	stamps         []float64
	depthDrops     []float64
	spreadWidens   []float64
	imbalanceFlips []float64
	fadeHistory    []float64
	lastDepth      float64
	lastSpread     float64
	lastImbalance  float64
	lastPressure   float64
}

func (signal *Signal) ensureSymbol(symbol string) *symbolHistory {
	raw, loaded := signal.symbols.Load(symbol)

	if loaded {
		state, ok := raw.(*symbolHistory)

		if ok {
			return state
		}
	}

	state := &symbolHistory{}
	signal.symbols.Store(symbol, state)

	return state
}

func (signal *Signal) recordBook(
	symbol string,
	stamp, depth, spread, imbalance, thinning, widen, flip float64,
) {
	state := signal.ensureSymbol(symbol)

	if thinning > 0 || len(state.depthDrops) > 0 {
		state.depthDrops = append(state.depthDrops, thinning)
	}

	if widen > 0 || len(state.spreadWidens) > 0 {
		state.spreadWidens = append(state.spreadWidens, widen)
	}

	if flip > 0 || len(state.imbalanceFlips) > 0 {
		state.imbalanceFlips = append(state.imbalanceFlips, flip)
	}

	state.stamps = append(state.stamps, stamp)
	state.lastDepth = depth
	state.lastSpread = spread
	state.lastImbalance = imbalance
}

func (signal *Signal) recordTrade(symbol string, stamp, pressure, fade float64) {
	state := signal.ensureSymbol(symbol)

	if fade > 0 || len(state.fadeHistory) > 0 {
		state.fadeHistory = append(state.fadeHistory, fade)
	}

	state.stamps = append(state.stamps, stamp)
	state.lastPressure = pressure
}

func (signal *Signal) fadeSamples(symbol string) []float64 {
	fadeHistory, _ := signal.tradeHistory(symbol)

	return fadeHistory
}

func (signal *Signal) peakPressureFade(symbol string) float64 {
	fades, _ := signal.tradeHistory(symbol)

	if len(fades) == 0 {
		return 0
	}

	peak := 0.0

	for _, fade := range fades {
		if fade > peak {
			peak = fade
		}
	}

	return peak
}

func (signal *Signal) bookHistory(symbol string) (
	depthDrops, spreadWidens, imbalanceFlips []float64,
	prevDepth, prevSpread, prevImbalance float64,
) {
	if raw, ok := signal.symbols.Load(symbol); ok {
		state, stateOK := raw.(*symbolHistory)

		if stateOK && len(state.stamps) > 0 {
			keep := statutil.WindowDepth(state.stamps)

			return statutil.Tail(state.depthDrops, keep),
				statutil.Tail(state.spreadWidens, keep),
				statutil.Tail(state.imbalanceFlips, keep),
				state.lastDepth,
				state.lastSpread,
				state.lastImbalance
		}
	}

	if signal.tree == nil {
		return nil, nil, nil, 0, 0, 0
	}

	query := datura.Acquire("exhaust", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourceExhaustion)))

	defer query.Release()

	var stamps []float64
	lastDepth, lastSpread, lastImbalance := 0.0, 0.0, 0.0

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		stamp := datura.Peek[float64](prior, "timestamp")
		depth := datura.Peek[float64](prior, "depth")
		spread := datura.Peek[float64](prior, "spread")
		imbalance := datura.Peek[float64](prior, "imbalance")

		if lastDepth > 0 && depth < lastDepth {
			depthDrops = append(depthDrops, (lastDepth-depth)/lastDepth)
		}

		if lastSpread > 0 && spread > lastSpread {
			spreadWidens = append(spreadWidens, (spread-lastSpread)/lastSpread)
		}

		if lastImbalance != 0 || imbalance != 0 {
			imbalanceFlips = append(imbalanceFlips, math.Abs(imbalance-lastImbalance))
		}

		lastDepth = depth
		lastSpread = spread
		lastImbalance = imbalance
		stamps = append(stamps, stamp)
		prevDepth = depth
		prevSpread = spread
		prevImbalance = imbalance
	}

	keep := statutil.WindowDepth(stamps)

	return statutil.Tail(depthDrops, keep),
		statutil.Tail(spreadWidens, keep),
		statutil.Tail(imbalanceFlips, keep),
		prevDepth,
		prevSpread,
		prevImbalance
}

func (signal *Signal) tradeHistory(symbol string) (fadeHistory []float64, prevPressure float64) {
	if raw, ok := signal.symbols.Load(symbol); ok {
		state, stateOK := raw.(*symbolHistory)

		if stateOK && len(state.stamps) > 0 {
			keep := statutil.WindowDepth(state.stamps)

			return statutil.Tail(state.fadeHistory, keep), state.lastPressure
		}
	}

	if signal.tree == nil {
		return nil, 0
	}

	query := datura.Acquire("exhaust", datura.APPJSON)
	query.WithRole("measurement")
	query.WithScope(symbol)
	errnie.Error(query.SetOrigin(string(logic.SourceExhaustion)))

	defer query.Release()

	var stamps []float64

	for prior := range signal.tree.Seek(query.Prefix("role", "scope", "origin")) {
		fadeHistory = append(fadeHistory, datura.Peek[float64](prior, "pressureFade"))
		stamps = append(stamps, datura.Peek[float64](prior, "timestamp"))
		prevPressure = datura.Peek[float64](prior, "pressure")
	}

	return statutil.Tail(fadeHistory, statutil.WindowDepth(stamps)), prevPressure
}
