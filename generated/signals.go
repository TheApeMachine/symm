package generated

import (
	nomagique "github.com/theapemachine/symm/nomagique"
)

/*
NewSignals compiles all available signal definitions into a single nomagique.Number pipeline.
*/
func NewSignals() nomagique.Number[any] {
	cvd := NewCvdTrade()
	hawkes := NewHawkesTrade()

	return nomagique.NewNumber[any](func(in any) any {
		if in == nil {
			return nil
		}

		var readings []float64

		if r, ok := cvd(in).([]float64); ok {
			readings = append(readings, r...)
		}

		if r, ok := hawkes(in).([]float64); ok {
			readings = append(readings, r...)
		}

		if len(readings) == 0 {
			return nil
		}

		return readings
	})
}
