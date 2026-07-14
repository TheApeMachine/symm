package logic

import "github.com/theapemachine/symm/types"

/*
CategoryComposer assigns contextual categories from typed measurements and the
symbol graph. Signals never emit categories; composition retains traceable
support, opposition, and missing complementary evidence.
*/
type CategoryComposer struct{}

/*
Compose appends pump-lifecycle and liquidity categories for one symbol epoch.
*/
func (composer *CategoryComposer) Compose(
	thesis *types.Thesis,
	symbol string,
) {
	graph := thesis.Graphs[symbol]

	if category, ok := composer.pumpLifecycle(symbol, thesis.Measurements, graph); ok {
		thesis.Categories = append(thesis.Categories, category)
	}

	if category, ok := composer.liquidityState(symbol, thesis.Measurements, graph); ok {
		thesis.Categories = append(thesis.Categories, category)
	}

	if category, ok := composer.toxicityState(symbol, thesis.Measurements, graph); ok {
		thesis.Categories = append(thesis.Categories, category)
	}
}
