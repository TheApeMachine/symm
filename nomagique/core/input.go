package core

import (
	"iter"
	"unsafe"
)

/*
Input supplies a keyed request. Origin identifies the sender independently of
Key and Value. A nil Value means no write payload was supplied; a pointer to a
zero value is a valid payload. Action names the operations for the recipient.
The request and its payload are borrowed until the downstream yield returns.
*/
type Input[Origin, Key, Value any] struct {
	*PrimitiveError
	Origin Identifiable[Origin]
	Action *Action
	Key    Key
	Value  *Value
}

// NewInput binds a key and optional borrowed payload to the requested actions.
func NewInput[Origin, Key, Value any](
	origin Identifiable[Origin], action *Action, key Key, value *Value,
) *Input[Origin, Key, Value] {
	return &Input[Origin, Key, Value]{
		PrimitiveError: NewPrimitiveError(),
		Origin:         origin,
		Action:         action,
		Key:            key,
		Value:          value,
	}
}

/*
Next binds each incoming Value address to this request without copying or
accumulating payloads. With nil input it yields the configured request once.
An empty, non-nil input yields no requests.
*/
func (input *Input[Origin, Key, Value]) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			yield(unsafe.Pointer(input))
			return
		}

		for arriving := range in {
			input.Value = (*Value)(arriving)

			if !yield(unsafe.Pointer(input)) {
				return
			}
		}
	}
}
