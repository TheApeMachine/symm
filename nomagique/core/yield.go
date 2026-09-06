package core

import "fmt"

// Yield advances the configured accumulator source once, then drains the
// incoming delivery run. The accumulator source owns when this operation ends
// its run. An inert Proto must be presented by transport, on either connection.
//
// Different accumulator and input types permit generic storage/transport folds;
// folds over Primitive keep objects opaque. No special case delivers a Proto.
// With no input a snapshot of the accumulator's yielded payload is returned.
// Failure owners receive errors even when a producer fails on its final nil.
func Yield[A, B any](left, right Primitive, fold func(A, B) A, owners ...Primitive) Primitive {
	var failures PrimitiveError
	defer func() {
		for _, owner := range owners {
			if owner != nil {
				owner.Error(failures.Error())
			}
		}
	}()
	if left == nil {
		return nil
	}
	seed := left.Next(nil)
	failures.Error(left.Error())
	if seed == nil {
		return nil
	}
	held := To[A](seed)
	failures.Error(seed.Error())
	if right == nil {
		out := From(held)
		if out != nil {
			out.Error(failures.Error())
		}
		return out
	}
	failures.Error(right.Error())
	for value := right.Next(nil); value != nil; value = right.Next(nil) {
		failures.Error(value.Error())
		var typed B
		var ok bool
		if _, opaque := any((*B)(nil)).(*Primitive); opaque {
			typed, ok = any(value).(B)
		} else {
			typed, ok = value.Read().(B)
		}
		if !ok {
			failures.Error(fmt.Errorf("%w: received %T", ErrWrongType, value.Read()))
			continue
		}
		// A failed seed is not silently turned into a valid arithmetic zero.
		if seed.Error() == nil {
			held = fold(held, typed)
		}
	}
	failures.Error(right.Error())
	for _, owner := range owners {
		if owner != nil {
			failures.Error(owner.Error())
		}
	}
	out := From(held)
	if out != nil {
		out.Error(failures.Error())
	}
	return out
}
