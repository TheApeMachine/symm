package trader

import (
	"sync"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

type trailRiskState struct {
	smoothedTurbulence float64
	smoothedSpectral   float64
}

/*
trailRiskFilter smooths topology metrics before widening stops.
*/
type trailRiskFilter struct {
	mu     sync.Mutex
	states map[string]*trailRiskState
}

func newTrailRiskFilter() *trailRiskFilter {
	return &trailRiskFilter{
		states: make(map[string]*trailRiskState),
	}
}

func (filter *trailRiskFilter) trailMultiple(
	symbol string,
	risk engine.SymbolRisk,
) float64 {
	multiple := config.System.TrailSpreadMultiple

	if multiple <= 0 {
		multiple = 2
	}

	if filter == nil || symbol == "" {
		return multiple
	}

	filter.mu.Lock()
	state := filter.states[symbol]

	if state == nil {
		state = &trailRiskState{}
		filter.states[symbol] = state
	}

	alpha := config.System.TrailRiskEMAAlpha

	if alpha <= 0 {
		alpha = 0.2
	}

	if risk.Turbulence > 0 {
		state.smoothedTurbulence = alpha*risk.Turbulence +
			(1-alpha)*state.smoothedTurbulence
	}

	if risk.SpectralRadius > 0 {
		state.smoothedSpectral = alpha*risk.SpectralRadius +
			(1-alpha)*state.smoothedSpectral
	}

	filter.mu.Unlock()

	spectralAt := config.System.TrailSpectralWidenAt

	if spectralAt <= 0 {
		spectralAt = 0.85
	}

	spectralGain := config.System.TrailSpectralWidenGain

	if spectralGain <= 0 {
		spectralGain = 4
	}

	if state.smoothedSpectral >= spectralAt {
		multiple *= 1 + (state.smoothedSpectral-spectralAt)*spectralGain
	}

	turbAt := config.System.TrailTurbWidenAt

	if turbAt <= 0 {
		turbAt = 1
	}

	turbMultiple := config.System.TrailTurbWidenMultiple

	if turbMultiple <= 0 {
		turbMultiple = 1.5
	}

	if state.smoothedTurbulence >= turbAt {
		multiple *= turbMultiple
	}

	reAt := config.System.TrailReynoldsWidenAt

	if reAt <= 0 {
		reAt = 50
	}

	reGain := config.System.TrailReynoldsWidenGain

	if reGain <= 0 {
		reGain = 0.01
	}

	if risk.Reynolds >= reAt {
		multiple *= 1 + (risk.Reynolds-reAt)*reGain
	}

	return multiple
}

func trailPctFromQuoteRisk(
	last, bid, ask float64,
	symbol string,
	riskReader RiskReader,
	filter *trailRiskFilter,
) float64 {
	if last <= 0 || bid <= 0 || ask <= 0 || ask < bid {
		return 0
	}

	spreadPct := (ask - bid) / last * 100
	multiple := config.System.TrailSpreadMultiple

	if multiple <= 0 {
		multiple = 2
	}

	if riskReader != nil && filter != nil && symbol != "" {
		risk, ok := riskReader.SymbolRisk(symbol)

		if ok {
			multiple = filter.trailMultiple(symbol, risk)
		}
	}

	return spreadPct * multiple
}

func (filter *trailRiskFilter) forget(symbol string) {
	if filter == nil || symbol == "" {
		return
	}

	filter.mu.Lock()
	delete(filter.states, symbol)
	filter.mu.Unlock()
}
