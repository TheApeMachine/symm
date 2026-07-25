package types

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
WireMeasurements publishes one focus-gated, aggregated UI frame for a signal's
measurement batch. Flat thesis rows collapse into compact metrics maps so the
socket carries one observation per source×symbol×at instead of one frame row
per metric. A full channel drops the frame rather than stalling measure; ticker
rewire of retained book batches recovers focus kernels after a drop.
*/
func WireMeasurements(rows []*Measurement, ui chan []byte) {
	if len(rows) == 0 || ui == nil {
		return
	}

	aggregated := AggregateMeasurements(Focused(rows))

	if len(aggregated) == 0 {
		return
	}

	frame, err := datura.Map[any]{
		"measurements": aggregated,
	}.Marshal()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"wire: measurements marshal failed",
			err,
		))
		return
	}

	select {
	case ui <- frame:
	default:
		errnie.Error(errnie.Err(
			errnie.TooManyRequests,
			"wire: ui channel saturated; dropped measurements",
			nil,
		))
	}
}
