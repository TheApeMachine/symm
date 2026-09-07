package transport

import "github.com/theapemachine/symm/nomagique/core"

// Discard consumes a run without yielding values. Failures are retained.
type Discard struct{ core.PrimitiveError }

func NewDiscard() *Discard { return &Discard{} }
func (discard *Discard) Next(in core.Primitive) core.Primitive {
	if in == nil {
		return nil
	}

	discard.Error(in.Error())

	for value := in.Next(nil); value != nil; value = in.Next(nil) {
		discard.Error(value.Error())
	}

	discard.Error(in.Error())
	return nil
}
func (discard *Discard) Read() any { return nil }
