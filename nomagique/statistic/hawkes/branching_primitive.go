package hawkes

import "github.com/theapemachine/symm/nomagique/types"

/*
Branching-scoped Frame facts, per signal/hawkes/README.md sections 13-15.
*/
var (
	SymbolOffspringBuyFromBuy   = types.MustIntern("hawkes/obs/offspring_buy_from_buy")
	SymbolOffspringBuyFromSell  = types.MustIntern("hawkes/obs/offspring_buy_from_sell")
	SymbolOffspringSellFromBuy  = types.MustIntern("hawkes/obs/offspring_sell_from_buy")
	SymbolOffspringSellFromSell = types.MustIntern("hawkes/obs/offspring_sell_from_sell")
	SymbolSpectralRadius        = types.MustIntern("hawkes/obs/branching_spectral_radius")
	SymbolDescendantsFromBuy    = types.MustIntern("hawkes/obs/expected_descendants_from_buy")
	SymbolDescendantsFromSell   = types.MustIntern("hawkes/obs/expected_descendants_from_sell")
)

/*
Branching reports the fitted branching matrix K=A/beta, its spectral radius,
and expected total descendants per source mark, from the model fitted before
the current event. Expected descendants are undefined (left absent) whenever
ρ(K)≥1 — README section 15 forbids clamping or substituting a finite value
in that case.
*/
func Branching(input *types.Frame) {
	_, _, alphaXX, alphaXY, alphaYX, alphaYY, beta, ok := ReadModel(input)

	if !ok {
		return
	}

	matrix := branchingMatrix(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	if !finite(matrix[0][0], matrix[0][1], matrix[1][0], matrix[1][1]) {
		return
	}

	input.Put(SymbolOffspringBuyFromBuy, matrix[0][0])
	input.Put(SymbolOffspringBuyFromSell, matrix[0][1])
	input.Put(SymbolOffspringSellFromBuy, matrix[1][0])
	input.Put(SymbolOffspringSellFromSell, matrix[1][1])
	input.Put(SymbolSpectralRadius, spectralRadius(matrix))

	buyParent, sellParent, ok := totalDescendants(alphaXX, alphaXY, alphaYX, alphaYY, beta)

	if !ok {
		return
	}

	input.Put(SymbolDescendantsFromBuy, buyParent)
	input.Put(SymbolDescendantsFromSell, sellParent)
}
