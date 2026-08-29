package logic

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/nomagique/utils"
)

// Observe requires each configured fact to be present and otherwise preserves input.
func Observe(required ...types.Symbol) types.Primitive {
	facts := append([]types.Symbol(nil), required...)

	return func(input *types.Frame) {
		for _, symbol := range facts {
			if input.Has(symbol) {
				continue
			}

			if name, found := types.SymbolName(symbol); found {
				input.Err = fmt.Errorf("logic: observation is missing coordinate %s", name)

				return
			}

			input.Err = fmt.Errorf("logic: observation is missing coordinate %d", symbol)

			return
		}
	}
}

// EnsureFinite requires each configured fact to be present and finite.
func EnsureFinite(required ...types.Symbol) types.Primitive {
	facts := append([]types.Symbol(nil), required...)

	return func(input *types.Frame) {
		for _, symbol := range facts {
			value, found := input.Get(symbol)

			if !found || !utils.IsFinite(value) {
				if name, named := types.SymbolName(symbol); named {
					input.Err = fmt.Errorf("logic: coordinate %s must be present and finite", name)

					return
				}

				input.Err = fmt.Errorf("logic: coordinate %d must be present and finite", symbol)

				return
			}
		}
	}
}
