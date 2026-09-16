package core

import (
	"iter"
	"unsafe"
)

/*
ActionType fortifies the Actionable contract with a set of canonical
actions that a Primitive can implement, which helps to retain compatibility
between various Primitive types. The thinking here is that one Primitive
that takes an action can take the Actionable input from another Primitive
that implement the Actional interface.
*/
type ActionType uint8

const (
	ActionNone ActionType = iota
	ActionIdentify
	ActionRead
	ActionWrite
	ActionExecute
)

/*
Action allows a single Next method on a Primitive to behave
differently, based on the input. The simple example to help
reason about this is: reading versus writing.
*/
type Action struct {
	*PrimitiveError

	sequence []ActionType
}

func NewAction(actions ...ActionType) *Action {
	return &Action{PrimitiveError: NewPrimitiveError(), sequence: actions}
}

/*
Next ...
*/
func (action *Action) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for i := range in {
			if !yield(i) {
				return
			}
		}
	}
}
