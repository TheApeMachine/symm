package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Origin yields a keyed input only when its origin identity equals the configured symbol.
An empty symbol yields every input.
*/
type Origin struct {
	*core.PrimitiveError

	symbol string
}

func NewOrigin(symbol string) *Origin {
	return &Origin{PrimitiveError: core.NewPrimitiveError(), symbol: symbol}
}

func (origin *Origin) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*core.Input[string, []string, any])(arriving)

			if input == nil {
				continue
			}

			if origin.symbol != "" {
				if input.Origin == nil {
					continue
				}

				if input.Origin.Identity() != origin.symbol {
					continue
				}
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
