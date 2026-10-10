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
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

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
					scale := math.Sqrt(res.PriorVariance)

					// Relative distinguishability: refuse the z-score when the
					// noise scale is below sqrt(eps)*max(1, |val|, |baseline|).
					// Absolute eps alone still admits billion-scale z squared
					// from collapsed floors.
					ref := math.Max(1, math.Max(math.Abs(val), math.Abs(res.Baseline)))

					if scale > math.Sqrt(2.220446049250313e-16)*ref {
						res.ScoreScale = scale
						res.ZScore = res.Residual / res.ScoreScale

						if !math.IsInf(res.ZScore, 0) && !math.IsNaN(res.ZScore) {
							energy := res.ZScore * res.ZScore
							op.energies = append(op.energies, energy)
							totalEnergy += energy
						}
					}
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
