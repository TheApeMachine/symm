package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
CorrelationVelocity measures how fast the cohort's signed correlation moves.
*/
type CorrelationVelocity struct {
	*core.PrimitiveError

	velocity core.Primitive
}

func NewCorrelationVelocity() *CorrelationVelocity {
	return &CorrelationVelocity{PrimitiveError: core.NewPrimitiveError(), velocity: temporal.NewVelocity()}
}

func (correlationVelocity *CorrelationVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return correlationVelocity.velocity.Next(in)
}
