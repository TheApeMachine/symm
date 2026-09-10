package algo

import (
	"fmt"
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/matrix"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/vector"
)

/*
Design is the observation matrix and its outcomes. Callers include an intercept
column explicitly.
*/
type Design struct {
	X [][]float64
	Y []float64
}

/*
Fit is one ordinary-least-squares solution. Rank deficiency and n<=p are
Defined=false with empty coefficients and an undefined residual variance.
*/
type Fit struct {
	Coefficients        []float64
	CoefficientVariance []float64
	Rank                int
	Observations        int
	Parameters          int
	ResidualSSE         float64
	ResidualVariance    float64
	Defined             bool
}

/*
OLS owns the normal equations and the configured linear solver.
*/
type OLS struct {
	core.Base[Design, Fit]
	solver *GaussJordan
}

func NewOLS(tolerance float64) *OLS {
	return &OLS{solver: NewGaussJordan(tolerance)}
}

func (op *OLS) Next(
	in iter.Seq[core.Primitive[Design, Design]],
) iter.Seq[core.Primitive[Fit, Fit]] {
	return func(yield func(core.Primitive[Fit, Fit]) bool) {
		for arriving := range in {
			fit, err := op.Fit(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(fit)) {
				return
			}
		}
	}
}

/*
Fit forms X'X β = X'y and solves it. There is no ridge or invented rank.
*/
func (op *OLS) Fit(design Design) (Fit, error) {
	observations := len(design.X)

	if observations != len(design.Y) {
		return Fit{}, fmt.Errorf("%w: OLS design rows and outcomes differ", core.ErrShape)
	}

	parameters := 0

	if observations > 0 {
		parameters = len(design.X[0])
	}

	for _, row := range design.X {
		if len(row) != parameters {
			return Fit{}, fmt.Errorf("%w: OLS design is ragged", core.ErrShape)
		}
	}

	undefined := Fit{
		Observations:        observations,
		Parameters:          parameters,
		ResidualVariance:    math.NaN(),
		Coefficients:        []float64{},
		CoefficientVariance: []float64{},
	}

	if observations == 0 || parameters == 0 || observations <= parameters {
		return undefined, nil
	}

	transpose, err := transport.Evaluate(matrix.NewTranspose[float64](), transport.Values(design.X))

	if err != nil {
		return Fit{}, err
	}

	product := matrix.NewProduct()
	xtx := product.Multiply(transpose, design.X)

	if err := product.Error(); err != nil {
		return Fit{}, err
	}

	column, err := transport.Evaluate(matrix.NewColumn(), transport.Values(design.Y...))

	if err != nil {
		return Fit{}, err
	}

	xty := product.Multiply(transpose, column)

	if err := product.Error(); err != nil {
		return Fit{}, err
	}

	identity, err := transport.Evaluate(matrix.NewIdentity(), transport.Values(float64(parameters)))

	if err != nil {
		return Fit{}, err
	}

	right, err := transport.Evaluate(matrix.NewAugment(), transport.Values(matrix.AugmentInput{
		Left:  xty,
		Right: identity,
	}))

	if err != nil {
		return Fit{}, err
	}

	solved, err := op.solver.Solve(System{Left: xtx, Right: right})

	if err != nil {
		return Fit{}, err
	}

	if !solved.Defined {
		return undefined, nil
	}

	coefficients := make([]float64, parameters)
	inverse := make([][]float64, parameters)
	xtyVector := make([]float64, parameters)

	for row := range parameters {
		coefficients[row] = solved.Solution[row][0]
		inverse[row] = solved.Solution[row][1:]
		xtyVector[row] = xty[row][0]
	}

	projection, err := transport.Evaluate(vector.NewDot(), transport.Values(vector.Pair{
		Left:  coefficients,
		Right: xtyVector,
	}))

	if err != nil {
		return Fit{}, err
	}

	energy := 0.0

	for _, outcome := range design.Y {
		energy += outcome * outcome
	}

	sse := energy - projection

	if sse < 0 {
		sse = 0
	}

	residualVariance := sse / float64(observations-parameters)
	diagonal, err := transport.Evaluate(matrix.NewDiagonal(), transport.Values(inverse))

	if err != nil {
		return Fit{}, err
	}

	variance, err := transport.Evaluate(vector.NewScale(), transport.Values(vector.ScaleInput{
		Values: diagonal,
		Factor: residualVariance,
	}))

	if err != nil {
		return Fit{}, err
	}

	return Fit{
		Coefficients:        coefficients,
		CoefficientVariance: variance,
		Rank:                solved.Rank,
		Observations:        observations,
		Parameters:          parameters,
		ResidualSSE:         sse,
		ResidualVariance:    residualVariance,
		Defined:             true,
	}, nil
}
