package correlation

import (
	"errors"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
LagEstimate is what an estimator publishes for one timestamp offset.
*/
type LagEstimate struct {
	Correlation float64
	Covariance  float64
	Support     float64
	LeftEnergy  float64
	RightEnergy float64
	Defined     bool
}

/*
EstimateInput is one pair of decoded return paths, their energies, and the
timestamp offset at which the estimator is evaluated.
*/
type EstimateInput struct {
	Left        []temporal.LogReturn
	Right       []temporal.LogReturn
	LeftEnergy  float64
	RightEnergy float64
	Lag         int64
}

/*
LagProfileInput is two price paths searched at every lag.
*/
type LagProfileInput struct {
	Left  []temporal.Price
	Right []temporal.Price
}

/*
LagCandidate retains the complete estimator record and its own support.
*/
type LagCandidate struct {
	LagEstimate
	Index    float64
	LagIndex float64
	X        float64
	Y        float64
}

/*
LagProfile owns the configured estimator and exact discrete search coordinates.
*/
type LagProfile struct {
	err         error
	estimator   core.Primitive
	pathReturns core.Primitive
	spacing     int64
	span        float64
	out         LagCandidate
}

/*
NewLagProfile creates a new LagProfile primitive over the supplied estimator.
*/
func NewLagProfile(estimator core.Primitive, spacing int64, span float64) core.Primitive {
	return &LagProfile{
		estimator:   estimator,
		pathReturns: temporal.NewPathReturns(),
		spacing:     spacing,
		span:        span,
	}
}

func (op *LagProfile) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*LagProfileInput)(arriving)
			left, err := decodePath(op.pathReturns, input.Left)

			if err != nil {
				op.err = errors.Join(op.err, err)
				return
			}

			right, err := decodePath(op.pathReturns, input.Right)

			if err != nil {
				op.err = errors.Join(op.err, err)
				return
			}

			limit := int(op.span*2 + 1)

			for index := 0; index < limit; index++ {
				lagIndex := float64(index) - op.span
				lag := int64(lagIndex * float64(op.spacing))
				reading, err := estimateAt(op.estimator, left, right, lag)

				if err != nil {
					op.err = errors.Join(op.err, err)
					return
				}

				op.out = LagCandidate{
					LagEstimate: reading,
					Index:       float64(index),
					LagIndex:    lagIndex,
					X:           float64(lag) * 1e-9,
					Y:           reading.Correlation,
				}

				if !yield(unsafe.Pointer(&op.out)) {
					return
				}
			}
		}
	}
}

func (op *LagProfile) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	if op.pathReturns != nil {
		if err := op.pathReturns.Error(); err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
decodePath decodes one price path into its returns and their energy.
*/
func decodePath(decoder core.Primitive, prices []temporal.Price) (temporal.ReturnPath, error) {
	var path temporal.ReturnPath

	for out := range decoder.Next(transport.NewValues(temporal.PricePath{Prices: prices}).Next(nil)) {
		path = *(*temporal.ReturnPath)(out)
	}

	if err := decoder.Error(); err != nil {
		return temporal.ReturnPath{}, err
	}

	return path, nil
}

/*
estimateAt drives the configured estimator primitive at one timestamp offset.
*/
func estimateAt(
	executor core.Primitive, left, right temporal.ReturnPath, lag int64,
) (LagEstimate, error) {
	var reading LagEstimate

	for out := range executor.Next(transport.NewValues(EstimateInput{
		Left:        left.Returns,
		Right:       right.Returns,
		LeftEnergy:  left.Energy,
		RightEnergy: right.Energy,
		Lag:         lag,
	}).Next(nil)) {
		reading = *(*LagEstimate)(out)
	}

	if err := executor.Error(); err != nil {
		return LagEstimate{}, err
	}

	return reading, nil
}
