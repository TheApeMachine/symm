package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Pick owns selection of one candidate. The predicate sees the held value and the
arrival as a pair and decides whether the arrival replaces what is held.
*/
type Pick struct {
	*core.PrimitiveError

	predicate core.Primitive
	held      bool
	current   float64
	out       float64
	pair      [2]float64
}

func NewPick(predicate core.Primitive) *Pick {
	return &Pick{PrimitiveError: core.NewPrimitiveError(), predicate: predicate}
}

func (pick *Pick) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		defer func() {

			if pick.predicate != nil {
				if err := pick.predicate.Error(); err != nil {
					pick.Error(err)
				}
			}
		}()
		for arriving := range in {
			in := (*float64)(arriving)
			val := *in

			if !pick.held {
				pick.held = true
				pick.current = val
				pick.out = val

				if !yield(unsafe.Pointer(&pick.out)) {
					return
				}

				continue
			}

			take := false
			pick.pair = [2]float64{val, pick.current}

			for decision := range pick.predicate.Next(func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&pick.pair))
			}) {
				dec := (*bool)(decision)
				take = *dec
			}

			if take {
				pick.current = val
			}

			pick.out = pick.current
			if !yield(unsafe.Pointer(&pick.out)) {
				return
			}
		}
	}
}
