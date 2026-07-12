package trader

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type signalMeasurement struct {
	measurements []*types.Measurement
	err          error
}

func measureSignals(
	signals []types.Signal[any],
	measure func(types.Signal[any]) ([]*types.Measurement, error),
) []signalMeasurement {
	results := make([]signalMeasurement, len(signals))

	for index, signal := range signals {
		results[index].measurements, results[index].err = measure(signal)

		if results[index].err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				results[index].err.Error(),
				results[index].err,
			))
		}
	}

	return results
}

/*
defaultCrossSection returns crossSection unchanged when it is already
set, or a fresh, empty CrossSection otherwise, so a feed built without an
explicit Signal.CrossSection (as most direct unit tests do) never hands a
nil pointer to a signal that dereferences it.
*/
func defaultCrossSection(crossSection *types.CrossSection) *types.CrossSection {
	if crossSection != nil {
		return crossSection
	}

	fallback, err := types.NewCrossSection(types.DefaultCrossSectionConfig())

	if err != nil {
		errnie.Error(err)
	}

	return fallback
}
