package nomagique

import (
	"iter"
	"reflect"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Number threads a run through the stages it is named with.

Stages carry their own types in their own signatures, and Go's generics are
invariant, so unlike stages cannot be named together while those types are
still visible. Each stage is asked for its own Next and handed the run the
stage before it produced; the types meet where the values do rather than where
the composition is written.
*/
func Number(stages ...any) func() iter.Seq[core.Primitive[any, any]] {
	return func() iter.Seq[core.Primitive[any, any]] {
		var run reflect.Value

		for _, stage := range stages {
			next := reflect.ValueOf(stage).MethodByName("Next")

			if !next.IsValid() || next.Type().NumIn() != 1 {
				continue
			}
			in := run

			// A stage at the head has nothing upstream, and a stage whose run
			// is of another shape is being handed something it cannot read: in
			// both the honest argument is the empty run its own Next declares.
			// That run is an iterator that yields nothing, not a nil function.
			if !in.IsValid() || in.Type() != next.Type().In(0) {
				in = emptyRun(next.Type().In(0))
			}
			run = next.Call([]reflect.Value{in})[0]
		}

		return widen(run)
	}
}

func emptyRun(seqType reflect.Type) reflect.Value {
	return reflect.MakeFunc(seqType, func([]reflect.Value) []reflect.Value {
		return nil
	})
}

/*
widen reads whatever the last stage produced as the run a caller can range.

The composition's own type cannot name the last stage's, so what comes back is
read through the shape every stage shares: a value that answers Read.
*/
func widen(run reflect.Value) iter.Seq[core.Primitive[any, any]] {
	return func(yield func(core.Primitive[any, any]) bool) {
		if !run.IsValid() || run.IsZero() {
			return
		}
		carrier := &core.Carrier[any]{}
		stop := false

		run.Call([]reflect.Value{reflect.MakeFunc(
			reflect.FuncOf(
				[]reflect.Type{run.Type().In(0).In(0)},
				[]reflect.Type{reflect.TypeOf(true)},
				false,
			),
			func(args []reflect.Value) []reflect.Value {
				if stop {
					return []reflect.Value{reflect.ValueOf(false)}
				}
				held := args[0].MethodByName("Read").Call(nil)[0]

				if !yield(carrier.Carrier(held.Interface()).(core.Primitive[any, any])) {
					stop = true
				}

				return []reflect.Value{reflect.ValueOf(!stop)}
			},
		)})
	}
}
