package logic

import (
	"github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"
)

type Boundary[T any] struct {
	signal    types.Signal[T]
	clamp     *manifold.Clamp
	positionX float64
	positionZ float64
	rho       float64
	momX      float64
	momY      float64
	momZ      float64
	energy    float64
	pressure  float64
	// metrics is the measurement's raw continuous state. The clamp is the
	// carrier for this signal; what it carries into the field is the
	// measurement. Each metric is a mass quantum the carrier deposits, so a
	// rich measurement injects a proportionally larger particle population than
	// a single point deposit.
	metrics map[string]float64
}

func NewBoundary(signal types.Signal[any]) *Boundary[any] {
	return &Boundary[any]{
		signal: signal,
	}
}
