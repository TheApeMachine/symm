package numeric

import "github.com/theapemachine/errnie"

/*
Observe selects the observations passed into a nested pipeline stage.
*/
type Observe func([]float64) []float64

/*
Accumulate runs a nested Derived pipeline and multiplies its output into the
running product from prior stages. When out is zero the branch output stands
alone.
*/
type Accumulate struct {
	pipeline *Derived
	observe  Observe
}

/*
NewAccumulate wires a nested pipeline into a fan-in stage.
*/
func NewAccumulate(pipeline *Derived, observe Observe) *Accumulate {
	if observe == nil {
		observe = func(values []float64) []float64 {
			return values
		}
	}

	return &Accumulate{
		pipeline: pipeline,
		observe:  observe,
	}
}

func (accumulate *Accumulate) Next(out float64, values ...float64) (float64, error) {
	factor, err := accumulate.pipeline.Push(accumulate.observe(values)...)

	if err != nil {
		return 0, errnie.Error(err)
	}

	if factor <= 0 {
		return 0, nil
	}

	if out <= 0 {
		return factor, nil
	}

	return out * factor, nil
}

func (accumulate *Accumulate) Reset() error {
	return accumulate.pipeline.Reset()
}

func (accumulate *Accumulate) clone() *Accumulate {
	if accumulate == nil {
		return nil
	}

	return &Accumulate{
		pipeline: accumulate.pipeline.Clone(),
		observe:  accumulate.observe,
	}
}
