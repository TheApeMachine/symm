package trader

import "github.com/theapemachine/symm/engine"

/*
RiskReader exposes live microstructure risk for one symbol.
*/
type RiskReader interface {
	SymbolRisk(symbol string) (engine.SymbolRisk, bool)
}

/*
SignalRiskBoard merges risk metrics from all signal RiskExporter implementations.
*/
type SignalRiskBoard struct {
	signals []engine.Signal
}

/*
NewSignalRiskBoard creates a risk board over trader signals.
*/
func NewSignalRiskBoard(signals ...engine.Signal) *SignalRiskBoard {
	return &SignalRiskBoard{signals: signals}
}

/*
SymbolRisk returns the peak topology metrics across exporters for one symbol.
*/
func (board *SignalRiskBoard) SymbolRisk(symbol string) (engine.SymbolRisk, bool) {
	if board == nil || symbol == "" {
		return engine.SymbolRisk{}, false
	}

	merged := engine.SymbolRisk{}
	found := false

	for _, signal := range board.signals {
		exporter, ok := signal.(engine.RiskExporter)

		if !ok {
			continue
		}

		risk, ok := exporter.SymbolRisk(symbol)

		if !ok {
			continue
		}

		found = true

		if risk.Reynolds > merged.Reynolds {
			merged.Reynolds = risk.Reynolds
		}

		if risk.Turbulence > merged.Turbulence {
			merged.Turbulence = risk.Turbulence
		}

		if risk.SpectralRadius > merged.SpectralRadius {
			merged.SpectralRadius = risk.SpectralRadius
		}
	}

	return merged, found
}
