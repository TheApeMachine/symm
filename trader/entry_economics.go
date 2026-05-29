package trader

import (
	"math"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

/*
entryReturnRequirement is the shared return scale for entry decisions.
It preserves raw fractional returns while adding the economic threshold a
forecast must clear before it is meaningful for this symbol and quote.
*/
type entryReturnRequirement struct {
	friction           float64
	stopFraction       float64
	requiredEdgeReturn float64
	requiredRReturn    float64
	requiredReturn     float64
}

func newEntryReturnRequirement(
	friction float64,
	stopFraction float64,
) entryReturnRequirement {
	requirement := entryReturnRequirement{
		friction:     friction,
		stopFraction: stopFraction,
	}

	if validReturnScale(friction) && friction > 0 {
		requirement.requiredEdgeReturn = friction * config.System.EntryEdgeMultiple
		requirement.requiredReturn = requirement.requiredEdgeReturn
	}

	if validReturnScale(stopFraction) && stopFraction > 0 {
		requirement.requiredRReturn = stopFraction * config.System.TakeProfitR

		if requirement.requiredRReturn > requirement.requiredReturn {
			requirement.requiredReturn = requirement.requiredRReturn
		}
	}

	return requirement
}

func (crypto *Crypto) entryReturnRequirement(
	symbol string,
	measurement engine.Measurement,
) entryReturnRequirement {
	return newEntryReturnRequirement(
		entryFrictionReturn(measurement),
		crypto.stopFractionFor(symbol),
	)
}

func (requirement entryReturnRequirement) edge(
	predictedReturn float64,
) float64 {
	return predictedReturn - requirement.friction
}

func (requirement entryReturnRequirement) multiple(
	returnValue float64,
) (float64, bool) {
	if !validReturnScale(returnValue) || requirement.requiredReturn <= 0 {
		return 0, false
	}

	return returnValue / requirement.requiredReturn, true
}

func (requirement entryReturnRequirement) multipleOrZero(
	returnValue float64,
) float64 {
	multiple, ok := requirement.multiple(returnValue)

	if !ok {
		return 0
	}

	return multiple
}

func (requirement entryReturnRequirement) significant(
	returnValue float64,
) bool {
	if requirement.requiredReturn <= 0 {
		return false
	}

	return returnValue >= requirement.requiredReturn
}

func (requirement entryReturnRequirement) auditFields(
	symbol string,
	predictedReturn float64,
) map[string]any {
	fields := map[string]any{
		"symbol":               symbol,
		"predicted_return":     predictedReturn,
		"friction":             requirement.friction,
		"edge":                 requirement.edge(predictedReturn),
		"required_multiple":    config.System.EntryEdgeMultiple,
		"required_return":      requirement.requiredReturn,
		"required_edge_return": requirement.requiredEdgeReturn,
		"stop_fraction":        requirement.stopFraction,
		"required_r":           config.System.TakeProfitR,
		"required_r_return":    requirement.requiredRReturn,
	}

	if multiple, ok := requirement.multiple(predictedReturn); ok {
		fields["prediction_multiple"] = multiple
	}

	return fields
}

func validReturnScale(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
