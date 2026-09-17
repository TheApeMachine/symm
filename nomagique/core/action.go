package core

import (
	"iter"
	"unsafe"
)

// ActionType identifies a canonical operation a receiving primitive may support.
type ActionType uint8

const (
	ActionNone ActionType = iota
	ActionIdentify
	ActionRead
	ActionWrite
	ActionExecute
)

// Action supplies operations in their declared order without executing them.
type Action struct {
	*PrimitiveError
	sequence []ActionType
}

func NewAction(actions ...ActionType) *Action {
	return &Action{PrimitiveError: NewPrimitiveError(), sequence: actions}
}

// Next completes upstream work, then yields the configured operation sequence.
func (action *Action) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in != nil {
			for range in {
			}
		}

		for index := range action.sequence {
			if !yield(unsafe.Pointer(&action.sequence[index])) {
				return
			}
		}
	}
}
