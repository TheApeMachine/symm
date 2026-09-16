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
	*core.PrimitiveError

	moments  []*Moments
	channels []CausalResidualResult
	energies []float64
	out      JointReading
}

func NewJoint(dimension int) *Joint {
	moments := make([]*Moments, dimension)

	for i := range moments {
		moments[i] = &Moments{}
	}

	return &Joint{PrimitiveError: core.NewPrimitiveError(), moments: moments,
		channels: make([]CausalResidualResult, dimension),
		energies: make([]float64, 0, dimension),
	}
}

func (joint *Joint) Next(
	in iter.Seq[unsafe.Pointer],
) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*JointInput)(arriving)

			if len(input.Values) != len(joint.moments) {
				joint.Error(core.ErrShape)
				return
			}

			joint.energies = joint.energies[:0]
			totalEnergy := 0.0

			for i, val := range input.Values {
				m := joint.moments[i]
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
					joint.energies = append(joint.energies, energy)
					totalEnergy += energy
				}

				joint.channels[i] = res
			}

			joint.out = JointReading{
				Channels: joint.channels,
				Energies: joint.energies,
			}

			if len(joint.energies) > 0 {
				joint.out.SNR = totalEnergy / float64(len(joint.energies))
				joint.out.SNRDefined = true
			}

			if !yield(unsafe.Pointer(&joint.out)) {
				return
			}
		}
	}
}
