package hawkes

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Gate classifies the arrival: the trade feed's side provenance is the mark,
and only a recognized side carries one. Anything else fails the measurement
here and never reaches the arrival path. Every arrival is yielded exactly
once, invalid or not.
*/
type Gate struct {
	*core.PrimitiveError
}

/*
NewGate creates the arrival classification stage.
*/
func NewGate() core.Primitive {
	return &Gate{PrimitiveError: core.NewPrimitiveError()}
}

func (op *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			m := *(**data.Measurement)(arriving)
			side := m.Meta("side")

			if side != "buy" && side != "sell" {
				reject(m, fmt.Errorf(
					"%w: hawkes: trade side %q carries no excitation mark", core.ErrDomain, side,
				))

				if !yield(arriving) {
					return
				}

				continue
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
