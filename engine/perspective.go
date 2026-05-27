package engine

import "math"

type PerspectiveType uint8

const (
	PerspectiveMicrostructure PerspectiveType = iota
	PerspectiveFlow
	PerspectiveCrossAsset
	PerspectiveSentiment
)

type MarketRegime uint8

const (
	RegimeDead MarketRegime = iota
	RegimeTrending
	RegimeBullish
	RegimeBearish
)

type Perspective struct {
	Type         PerspectiveType
	Measurements []Measurement
	Regime       MarketRegime
}

/*
FuseMeasurements combines independent source confidences into one joint
confidence and counts distinct contributing sources.
*/
func FuseMeasurements(measurements []Measurement) (jointConfidence float64, sourceCount int) {
	anonymous := make([]float64, 0, len(measurements))
	sources := make(map[string]float64)

	for _, measurement := range measurements {
		if measurement.Confidence <= 0 {
			continue
		}

		if measurement.Source == "" {
			anonymous = append(anonymous, measurement.Confidence)
			continue
		}

		if measurement.Confidence > sources[measurement.Source] {
			sources[measurement.Source] = measurement.Confidence
		}
	}

	factors := make([]float64, 0, len(sources)+len(anonymous))

	for _, confidence := range sources {
		factors = append(factors, confidence)
	}

	factors = append(factors, anonymous...)

	if len(factors) == 0 {
		return 0, len(sources)
	}

	product := 1.0

	for _, confidence := range factors {
		product *= confidence
	}

	return math.Pow(product, 1/float64(len(factors))), len(sources)
}
