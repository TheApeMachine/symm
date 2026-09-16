package logic

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Reject owns explicit refusal. It records the configured reason, consumes the
run, and hands nothing over.
*/
type Reject struct {
	*core.PrimitiveError

	reason error
}

func NewReject(reason error) *Reject {
	return &Reject{PrimitiveError: core.NewPrimitiveError(), reason: reason}
}

func (reject *Reject) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(func(unsafe.Pointer) bool) {
		reject.Error(reject.reason)

		for range in {
		}
	}
}
