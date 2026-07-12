package types

import (
	"time"

	"github.com/theapemachine/errnie"
)

/*
LogicEpoch preserves all typed measurements observed for one symbol at one
exact event time. It intentionally carries no category until logic has enough
independent numerical evidence to justify an interpretation.
*/
type LogicEpoch struct {
	Symbol       string        `json:"symbol"`
	At           time.Time     `json:"at"`
	Measurements []Measurement `json:"measurements"`
}

/*
Validate proves that an epoch is one coherent symbol and event-time slice.
Logic retains complete measurements rather than references, so accepting a
mismatched identity here would make later provenance irrecoverably ambiguous.
*/
func (epoch LogicEpoch) Validate() error {
	if epoch.Symbol == "" || epoch.At.IsZero() || len(epoch.Measurements) == 0 {
		return errnie.Err(
			errnie.Validation,
			"logic epoch: symbol, event time, and measurements required",
			nil,
		)
	}

	for _, measurement := range epoch.Measurements {
		if measurement.Symbol != epoch.Symbol || !measurement.At.Equal(epoch.At) {
			return errnie.Err(
				errnie.Validation,
				"logic epoch: measurement identity does not match epoch",
				nil,
			)
		}

		if err := measurement.Validate(); err != nil {
			return errnie.Err(
				errnie.Validation,
				"logic epoch: invalid measurement",
				err,
			)
		}
	}

	return nil
}

/*
Clone returns an independent epoch value so an append-only Thesis journal
cannot be changed through a caller-owned measurement slice.
*/
func (epoch LogicEpoch) Clone() LogicEpoch {
	measurements := make([]Measurement, len(epoch.Measurements))

	for index, measurement := range epoch.Measurements {
		measurements[index] = measurement.Clone()
	}

	epoch.Measurements = measurements

	return epoch
}
