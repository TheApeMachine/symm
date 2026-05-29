package market

import (
	"fmt"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
Measurement is one classified signal reading in the market layer.

The pipeline:

 1. Each signal (fluid, hawkes, pumpdump, …) publishes engine.Measurement with
    Source and Category (a DECISION.md row, e.g. "coiled_compression").
 2. Story and Perspective ingest those readings via MeasurementFromEngine,
    storing perspectives.CategoryType so they can be matched to tree branches.
 3. A perspective tree (see market/perspectives) is navigated only when the
    current measurement set contains the CategoryType required at each branch.
    Missing categories mean that path is not relevant.

Measurements are per-source verdicts, not a probability map: one signal emits
one category per tick. Confidence is how completely the observation matches
that category's criteria (from the engine reading).
*/
type Measurement struct {
	Source     string
	Category   perspectives.CategoryType
	Confidence float64
}

/*
MeasurementFromEngine converts a signal-layer reading into market vocabulary.
Returns an error when Category is empty or not mapped to a tree key.
*/
func MeasurementFromEngine(reading engine.Measurement) (Measurement, error) {
	if reading.Source == "" {
		return Measurement{}, fmt.Errorf("market: measurement missing source")
	}

	if reading.Category == engine.CategoryNone {
		return Measurement{}, fmt.Errorf("market: measurement from %q has no category", reading.Source)
	}

	category, err := CategoryFromEngine(reading.Category)

	if err != nil {
		return Measurement{}, err
	}

	if category == perspectives.CategoryTypeNone {
		return Measurement{}, fmt.Errorf("market: measurement from %q has unmapped category %q",
			reading.Source, reading.Category)
	}

	return Measurement{
		Source:     reading.Source,
		Category:   category,
		Confidence: reading.Confidence,
	}, nil
}

/*
HasCategory reports whether this measurement matches a tree branch key.
*/
func (measurement Measurement) HasCategory(category perspectives.CategoryType) bool {
	return measurement.Category == category
}
