package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

// Observe requires each configured fact to be present and otherwise preserves input.
func Observe(required ...nomagique.Symbol) nomagique.Primitive {
	facts := append([]nomagique.Symbol(nil), required...)

	return func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
		for _, symbol := range facts {
			if input.Has(symbol) {
				continue
			}

			if name, found := nomagique.SymbolName(symbol); found {
				return state, types.Frame{}, fmt.Errorf("logic: observation is missing coordinate %s", name)
			}

			return state, types.Frame{}, fmt.Errorf("logic: observation is missing coordinate %d", symbol)
		}

		return state, input, nil
	}
}

// EnsureFinite requires each configured fact to be present and finite.
func EnsureFinite(required ...nomagique.Symbol) nomagique.Primitive {
	facts := append([]nomagique.Symbol(nil), required...)

	return func(state types.Frame, input types.Frame) (types.Frame, types.Frame, error) {
		for _, symbol := range facts {
			value, found := input.Get(symbol)

			if !found || !utils.IsFinite(value) {
				if name, named := nomagique.SymbolName(symbol); named {
					return state, types.Frame{}, fmt.Errorf("logic: coordinate %s must be present and finite", name)
				}

				return state, types.Frame{}, fmt.Errorf("logic: coordinate %d must be present and finite", symbol)
			}
		}

		return state, input, nil
	}
}
