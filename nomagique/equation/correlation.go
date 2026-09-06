package equation

import (
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewCorrelation composes covariance / sqrt(left energy * right energy).
// It preserves finite-sample asynchronous values outside [-1,1]. Empty or
// zero-energy normalization is undefined, not zero or a previous result.
func NewCorrelation() core.Primitive {
	return NewRatio[float64](
		store.NewGet("covariance"),
		transport.NewPipe(
			NewProduct[float64](store.NewGet("left_energy"), store.NewGet("right_energy")),
			calculus.NewSqrt(transport.NewIO(core.From(0.0))),
		),
	)
}
