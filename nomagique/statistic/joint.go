package statistic

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
JointInput is an observation vector with one coordinate per channel.
*/
type JointInput struct {
	Values []float64
}

/*
JointReading is per-channel log-moment views and the average standardized energy SNR.
*/
type JointReading struct {
	Channels   []CausalResidualResult
	Energies   []float64
	SNR        float64
	SNRDefined bool
}

/*
Joint applies moment tracking and residual analysis per channel coordinate.
*/
type Joint struct {
	err      error
	moments  []*Moments
	channels []CausalResidualResult
	energies []float64
	out      JointReading
}

func NewJoint(dimension int) core.Primitive {
	moments := make([]*Moments, dimension)

	for i := range moments {
		moments[i] = &Moments{}
	}

	return &Joint{
		moments:  moments,
		channels: make([]CausalResidualResult, dimension),
		energies: make([]float64, 0, dimension),
	}
}

func (op *Joint) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*JointInput)(arriving)

			if len(input.Values) != len(op.moments) {
				op.err = core.ErrShape
				return
			}

			op.energies = op.energies[:0]
			totalEnergy := 0.0

			for i, val := range input.Values {
				m := op.moments[i]
				reading := m.Update(val)

				res := CausalResidualResult{
					MomentReading: reading,
					HasPrior:      reading.Prior.Count > 0,
					Baseline:      math.Exp(reading.Prior.Mean),
					Residual:      val - reading.Prior.Mean,
				}

				if reading.Prior.Count > 1 && reading.Prior.M2 > 0 {
					res.PriorVariance = reading.Prior.M2 / (reading.Prior.Count - 1)
					res.ScoreScale = math.Sqrt(res.PriorVariance)
					res.ZScore = res.Residual / res.ScoreScale
					energy := res.ZScore * res.ZScore
					op.energies = append(op.energies, energy)
					totalEnergy += energy
				}

				op.channels[i] = res
			}

			op.out = JointReading{
				Channels: op.channels,
				Energies: op.energies,
			}

			if len(op.energies) > 0 {
				op.out.SNR = totalEnergy / float64(len(op.energies))
				op.out.SNRDefined = true
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

func (op *Joint) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = err
			break
		}
	}

	return op.err
}
