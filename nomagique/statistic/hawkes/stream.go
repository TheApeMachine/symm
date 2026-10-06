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
and only a recognized side carries one. Anything else fails the stage here
and never reaches the arrival path.
*/
type Gate struct {
	*core.PrimitiveError
	input data.Map[string]
}

/*
NewGate creates the arrival classification stage.
*/
func NewGate() core.Primitive {
	return &Gate{
		PrimitiveError: core.NewPrimitiveError(),
		input:          data.NewMap("buy", "buy", "sell", "sell"),
	}
}

func (op *Gate) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			buy, buyOK := values.Values["buy"]
			sell, sellOK := values.Values["sell"]

			if !buyOK || !sellOK || buy == sell || (buy != 1 && sell != 1) {
				op.Error(fmt.Errorf(
					"%w: hawkes: trade side carries no excitation mark", core.ErrDomain,
				))
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
