package engine

/*
SymbolRisk holds live microstructure risk metrics for execution elasticity.
*/
type SymbolRisk struct {
	Reynolds       float64
	Turbulence     float64
	SpectralRadius float64
}

/*
RiskExporter exposes per-symbol topology metrics for dynamic execution.
*/
type RiskExporter interface {
	SymbolRisk(symbol string) (SymbolRisk, bool)
}
