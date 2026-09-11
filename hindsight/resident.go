package hindsight

import (
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

/*
Resident resolves what the running binary actually held at one capture coordinate.

The instrument's own captures are walked backwards from the coordinate and the
first value seen for each quantity is kept, so what comes back is the latest
value causally available there rather than the nearest one in time (§9). The
walk never looks forward (§31): a capture after the coordinate carried a fact
the binary did not have yet.

Nothing is projected into a second shape. A measurement already says what it is,
where it came from and how old it is, so the reading is the measurements
themselves — one per quantity, each with the capture that carried it.
*/
func Resident(
	catalog *tables.Catalog,
	index *RunIndex,
	symbol string,
	target EnvelopeRef,
	budget int,
) ([]*data.Measurement[float64], error) {
	if catalog == nil || index == nil {
		return nil, nil
	}

	held := make(map[string]*data.Measurement[float64])
	order := make([]string, 0)

	for _, capture := range index.CapturesBefore(symbol, target, budget) {
		payload, stored, err := catalog.ReadStatePayload(
			string(capture.Origin.Run),
			uint64(capture.Origin.Sequence),
			capture.Ordinal,
		)

		if err != nil {
			return nil, err
		}

		if !stored {
			continue
		}

		measurements, err := types.MeasurementsFromState(payload)

		if err != nil {
			return nil, err
		}

		for _, measurement := range measurements {
			if measurement == nil {
				continue
			}

			// Captures arrive newest first, so the first value seen for a
			// quantity is the one that was resident. An older capture carrying
			// the same quantity is what it replaced, never what it held.
			quantity := measurement.Source + "\x00" + measurement.ID

			if _, carried := held[quantity]; carried {
				continue
			}

			held[quantity] = measurement
			order = append(order, quantity)
		}
	}

	reading := make([]*data.Measurement[float64], 0, len(order))

	for _, quantity := range order {
		reading = append(reading, held[quantity])
	}

	return reading, nil
}
