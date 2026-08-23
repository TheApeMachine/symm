package nomagique

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
KeyedStreams is the source-compatible name for a keyed Number. New code should
use Number directly so the top-level composition boundary has one name.

Deprecated: use Number.
*/
type KeyedStreams[Key comparable] struct {
	number *Number[Key]
}

// NewKeyedStreams creates one isolated stream per key.
func NewKeyedStreams[Key comparable](
	primitive types.Primitive,
	initial func(Key) types.Frame,
) *KeyedStreams[Key] {
	return &KeyedStreams[Key]{
		number: NewNumberWithInitial(initial, primitive),
	}
}

// Step routes one input to the unit selected by key.
func (collection *KeyedStreams[Key]) Step(key Key, input types.Frame) (types.Frame, error) {
	if collection == nil || collection.number == nil {
		return types.Frame{}, errnie.Error(errnie.Err(
			errnie.Internal,
			"keyed stream primitive is nil",
			nil,
		))
	}

	return collection.number.Step(key, input)
}

// Project returns the last committed state for key.
func (collection *KeyedStreams[Key]) Project(key Key) (types.Frame, bool) {
	if collection == nil || collection.number == nil {
		return types.Frame{}, false
	}

	return collection.number.Project(key)
}

// Output returns the last successful output for key.
func (collection *KeyedStreams[Key]) Output(key Key) (types.Frame, bool) {
	if collection == nil || collection.number == nil {
		return types.Frame{}, false
	}

	return collection.number.Output(key)
}

// Error returns the last transition failure for key.
func (collection *KeyedStreams[Key]) Error(key Key) (error, bool) {
	if collection == nil || collection.number == nil {
		return nil, false
	}

	return collection.number.Error(key)
}

// Delete removes one keyed stream.
func (collection *KeyedStreams[Key]) Delete(key Key) {
	if collection == nil || collection.number == nil {
		return
	}

	collection.number.Delete(key)
}

// Range visits committed keyed state snapshots.
func (collection *KeyedStreams[Key]) Range(yield func(Key, types.Frame) bool) {
	if collection == nil || collection.number == nil {
		return
	}

	collection.number.Range(yield)
}
