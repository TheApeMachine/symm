package hawkes

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
)

// NewCompensator integrates both intensities across the observed window.
func NewCompensator() core.Primitive {
	return equation.NewSum[float64](
		equation.NewProduct[float64](equation.NewSum[float64](store.NewGet("mu_x"), store.NewGet("mu_y")), store.NewGet("span")),
		equation.NewSum[float64](
			equation.NewProduct[float64](
				equation.NewRatio[float64](equation.NewSum[float64](store.NewGet("alpha_xx"), store.NewGet("alpha_yx")), store.NewGet("beta")),
				store.NewGet("integral_x"),
			),
			equation.NewProduct[float64](
				equation.NewRatio[float64](equation.NewSum[float64](store.NewGet("alpha_xy"), store.NewGet("alpha_yy")), store.NewGet("beta")),
				store.NewGet("integral_y"),
			),
		),
	)
}
