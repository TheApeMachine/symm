package algo

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
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
OLS owns the ordinary-least-squares fit over arriving designs, delegating the
normal-equation solve to the statistic layer's OLS Primitive. There is no
ridge or invented rank: when the design matrix does not have full column
rank the fit is undefined.
*/
type OLS struct {
	err    error
	solver core.Primitive
	out    Fit
}

/*
NewOLS creates the ordinary-least-squares Primitive. The tolerance is
retained for caller compatibility; the statistic layer's solver owns the
numerical solve.
*/
func NewOLS(tolerance float64) core.Primitive {
	_ = tolerance

	return &OLS{solver: statistic.NewFitOLS()}
}

/*
Next receives *Design payloads and yields a *Fit for each.
*/
func (op *OLS) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			design := (*Design)(arriving)
			request, err := flatten(design)

			if err != nil {
				op.Error(err)
				return
			}

			fit, err := op.fit(request)

			if err != nil {
				op.Error(err)
				return
			}

			op.out = fit

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *OLS) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
single presents one request pointer as a one-element run.
*/
func single(request *statistic.OLSRequest) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		yield(unsafe.Pointer(request))
	}
}

/*
flatten validates one design and converts it to the statistic layer's
row-major request.
*/
func flatten(design *Design) (statistic.OLSRequest, error) {
	observations := len(design.X)

	if observations != len(design.Y) {
		return statistic.OLSRequest{}, fmt.Errorf(
			"%w: algo: OLS design rows and outcomes differ", core.ErrShape,
		)
	}

	parameters := 0

	if observations > 0 {
		parameters = len(design.X[0])
	}

	for _, row := range design.X {
		if len(row) != parameters {
			return statistic.OLSRequest{}, fmt.Errorf(
				"%w: algo: OLS design is ragged", core.ErrShape,
			)
		}
	}

	flat := make([]float64, 0, observations*parameters)

	for _, row := range design.X {
		flat = append(flat, row...)
	}

	return statistic.OLSRequest{X: flat, Y: design.Y, P: parameters}, nil
}

/*
fit drives the statistic layer's solver for one request.
*/
func (op *OLS) fit(request statistic.OLSRequest) (Fit, error) {
	var solved statistic.OLSFit

	for out := range op.solver.Next(single(&request)) {
		solved = *(*statistic.OLSFit)(out)
	}

	if err := op.solver.Error(); err != nil {
		return Fit{}, err
	}

	fit := Fit{
		Coefficients:        solved.Coefficients,
		CoefficientVariance: solved.CoefficientVariance,
		Rank:                solved.Rank,
		Observations:        solved.Observations,
		Parameters:          solved.Parameters,
		ResidualSSE:         solved.ResidualSSE,
		ResidualVariance:    solved.ResidualVariance,
		Defined:             solved.Defined,
	}

	if !solved.Defined {
		fit.ResidualVariance = math.NaN()
		fit.Coefficients = []float64{}
		fit.CoefficientVariance = []float64{}
	}

	return fit, nil
}
