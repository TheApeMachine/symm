package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Match yields a price observation only when its symbol equals the configured identity.
*/
type Match struct {
	*core.PrimitiveError

	symbol string
	out    PriceObservation
}

func NewMatch(symbol string) *Match {
	return &Match{PrimitiveError: core.NewPrimitiveError(), symbol: symbol}
}

func (match *Match) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			return
		}

		for arriving := range in {
			observation := *(*PriceObservation)(arriving)

			if observation.Symbol != match.symbol {
				continue
			}

			match.out = observation

			if !yield(unsafe.Pointer(&match.out)) {
				return
			}
		}
	}
}
