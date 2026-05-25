package trader

import "github.com/theapemachine/symm/engine"

/*
RiskReader exposes live microstructure risk for one symbol.
*/
type RiskReader interface {
	SymbolRisk(symbol string) (engine.SymbolRisk, bool)
}
