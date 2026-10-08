package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Member admits an arriving price/timestamp observation for a symbol into its
retained path in the shared PathStore.
Operands arrive in order: [price, atNano].
Yields the accepted price and atNano downstream.
*/
type Member struct {
	*core.PrimitiveError
	symbol string
	store  *PathStore
	out    [2]float64
}

func NewMember(symbol string, store *PathStore) core.Primitive {
	return &Member{
		PrimitiveError: core.NewPrimitiveError(),
		symbol:         symbol,
		store:          store,
	}
}

func (op *Member) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var vals [2]float64
		idx := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if idx < 2 {
				vals[idx] = *(*float64)(arriving)
				idx++
			}
		}

		if idx < 2 {
			op.Error(core.ErrShape)
			return
		}

		price := vals[0]
		atNano := vals[1]

		symbol := op.symbol
		if symbol == "" && op.store != nil {
			symbol = op.store.Current()
		}

		if symbol != "" && op.store != nil {
			existing := op.store.paths[symbol]
			sample := [2]float64{atNano, price}

			if len(existing) == 0 || atNano > existing[len(existing)-1][0] {
				op.store.paths[symbol] = append(existing, sample)
			}

			if len(existing) > 0 && atNano == existing[len(existing)-1][0] {
				existing[len(existing)-1] = sample
			}
		}

		op.out[0] = price
		op.out[1] = atNano

		if !yield(unsafe.Pointer(&op.out[0])) {
			return
		}

		if !yield(unsafe.Pointer(&op.out[1])) {
			return
		}
	}
}
