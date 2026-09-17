package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Slot is one keyed retained value. Key is the market symbol.
*/
type Slot[T any] struct {
	Key   string
	Value T
}

/*
Keyed holds the latest T per symbol. A write replaces that symbol's value.
An empty or nil run yields every held slot without replacing anything.
*/
type Keyed[T any] struct {
	*core.PrimitiveError

	current map[string]T
	order   []string
	out     Slot[T]
}

func NewKeyed[T any]() *Keyed[T] {
	return &Keyed[T]{
		PrimitiveError: core.NewPrimitiveError(),
		current:        make(map[string]T),
	}
}

func (keyed *Keyed[T]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			keyed.emit(yield)
			return
		}

		wrote := false

		for arriving := range in {
			slot := *(*Slot[T])(arriving)
			keyed.set(slot)
			keyed.out = slot
			wrote = true

			if !yield(unsafe.Pointer(&keyed.out)) {
				return
			}
		}

		if wrote {
			return
		}

		keyed.emit(yield)
	}
}

func (keyed *Keyed[T]) set(slot Slot[T]) {
	if _, exists := keyed.current[slot.Key]; !exists {
		keyed.order = append(keyed.order, slot.Key)
	}

	keyed.current[slot.Key] = slot.Value
}

func (keyed *Keyed[T]) emit(yield func(unsafe.Pointer) bool) {
	for _, key := range keyed.order {
		keyed.out = Slot[T]{Key: key, Value: keyed.current[key]}

		if !yield(unsafe.Pointer(&keyed.out)) {
			return
		}
	}
}

/*
Stamp keys each float the inner primitive yields with a name taken from the
arriving record, so Keyed can retain one value per symbol.
*/
type Stamp[R any] struct {
	*core.PrimitiveError

	inner core.Primitive
	key   func(*R) string
	out   Slot[float64]
}

/*
Fixed keys each arriving float with a constructor-supplied symbol.
*/
type Fixed struct {
	*core.PrimitiveError

	key string
	out Slot[float64]
}

func NewFixed(key string) *Fixed {
	return &Fixed{PrimitiveError: core.NewPrimitiveError(), key: key}
}

func (fixed *Fixed) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			fixed.out = Slot[float64]{Key: fixed.key, Value: *(*float64)(arriving)}

			if !yield(unsafe.Pointer(&fixed.out)) {
				return
			}
		}
	}
}

func NewStamp[R any](inner core.Primitive, key func(*R) string) *Stamp[R] {
	return &Stamp[R]{PrimitiveError: core.NewPrimitiveError(), inner: inner, key: key}
}

func (stamp *Stamp[R]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if stamp.inner != nil {
			defer func() { stamp.Error(stamp.inner.Error()) }()
		}

		for arriving := range in {
			record := (*R)(arriving)
			one := func(pass func(unsafe.Pointer) bool) {
				pass(arriving)
			}

			for out := range stamp.inner.Next(one) {
				stamp.out = Slot[float64]{Key: stamp.key(record), Value: *(*float64)(out)}

				if !yield(unsafe.Pointer(&stamp.out)) {
					return
				}
			}
		}
	}
}
