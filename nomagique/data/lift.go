package data

import (
	"github.com/theapemachine/errnie"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
Lift maps SignalMeasurements into <source>/<metric> Frame slots.
*/
func Lift[M any](measurements [11]*Measurement[M]) nmtypes.Frame {
	frame := nmtypes.Frame{}

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		if measurement.Err != nil {
			frame.Err = errnie.Error(errnie.Err(
				errnie.Validation,
				"lift: signal measurement failed",
				measurement.Err,
			))

			return frame
		}

		source := measurement.Source

		for label, metric := range measurement.Metrics {
			slot := nmtypes.MustIntern(source + "/" + label)
			value, valid := any(metric.Raw).(float64)

			if !valid {
				frame.Err = errnie.Error(errnie.Err(
					errnie.Validation,
					"lift: metric must be float64",
					nil,
				))

				return frame
			}

			frame.Put(slot, value)
		}
	}

	return frame
}
