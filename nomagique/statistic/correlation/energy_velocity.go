package correlation

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
EnergyVelocity measures how fast the relative return energy rate moves.
*/
type EnergyVelocity struct {
	*core.PrimitiveError

	velocity core.Primitive
}

func NewEnergyVelocity() *EnergyVelocity {
	return &EnergyVelocity{PrimitiveError: core.NewPrimitiveError(), velocity: temporal.NewVelocity()}
}

func (energyVelocity *EnergyVelocity) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return energyVelocity.velocity.Next(in)
}
