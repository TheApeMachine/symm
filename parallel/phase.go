package parallel

import (
	"math"
	"sync"

	nmstore "github.com/theapemachine/symm/nomagique/store"
)

const phaseHistory = 8

/*
SymbolPhase tracks Kuramoto order and a guidance scalar from lit regions.
This is the per-symbol physics readout fed to Book B and Book C.
*/
type SymbolPhase struct {
	R         float64
	DeltaR    float64
	Guidance  float64
	Heat      float64
	Conflict  float64
	Stable    bool
	Surprise  float64
	priorR    float64
	history   []float64
}

type PhaseField struct {
	mu      sync.Mutex
	bySymbol map[string]*SymbolPhase
}

func NewPhaseField() *PhaseField {
	return &PhaseField{bySymbol: map[string]*SymbolPhase{}}
}

func (field *PhaseField) Observe(symbol string, regions []*nmstore.Region, surprise float64) SymbolPhase {
	if field == nil || symbol == "" {
		return SymbolPhase{}
	}

	field.mu.Lock()
	defer field.mu.Unlock()

	state := field.bySymbol[symbol]
	if state == nil {
		state = &SymbolPhase{}
		field.bySymbol[symbol] = state
	}

	state.priorR = state.R
	state.R, state.Guidance, state.Heat, state.Conflict = kuramotoFromRegions(regions)
	state.DeltaR = state.R - state.priorR
	state.Surprise = surprise

	state.history = append(state.history, state.R)
	if len(state.history) > phaseHistory {
		state.history = state.history[len(state.history)-phaseHistory:]
	}

	state.Stable = modeStable(state.history)

	return *state
}

func kuramotoFromRegions(regions []*nmstore.Region) (r, guidance, heat, conflict float64) {
	if len(regions) == 0 {
		return 0, 0, 0, 0
	}

	var sumCos, sumSin float64
	var enter, exit float64

	for _, region := range regions {
		if region == nil {
			continue
		}

		phase := float64(region.Key) / float64(nmstore.RegionCount) * 2 * math.Pi
		weight := region.Brightness

		sumCos += weight * math.Cos(phase)
		sumSin += weight * math.Sin(phase)
		heat += weight

		switch nmstore.RegionSide(region.Key) {
		case "enter":
			enter += weight * region.Deformation
		case "exit":
			exit += weight * region.Deformation
		}
	}

	r = math.Hypot(sumCos, sumSin) / math.Max(heat, 1e-9)
	guidance = enter - exit
	conflict = math.Abs(enter-exit) / math.Max(enter+exit, 1e-9)

	return r, guidance, heat, conflict
}

func modeStable(history []float64) bool {
	if len(history) < 3 {
		return false
	}

	mean := 0.0
	for _, v := range history {
		mean += v
	}

	mean /= float64(len(history))

	variance := 0.0
	for _, v := range history {
		d := v - mean
		variance += d * d
	}

	variance /= float64(len(history))

	return mean > 0.35 && variance < 0.02
}
