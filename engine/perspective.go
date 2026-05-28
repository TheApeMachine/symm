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
	RegimeUnknown MarketRegime = iota
	RegimeDead
	RegimeChoppy
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
PerspectiveSource returns the synthetic source name used for perspective-level
forecasts and feedback.
*/
func PerspectiveSource(perspectiveType PerspectiveType) string {
	switch perspectiveType {
	case PerspectiveFlow:
		return "perspective:flow"
	case PerspectiveCrossAsset:
		return "perspective:cross_asset"
	case PerspectiveSentiment:
		return "perspective:sentiment"
	default:
		return "perspective:microstructure"
	}
}

/*
FuseMeasurements combines independent source confidences into one joint
confidence and counts distinct contributing sources.
*/
func FuseMeasurements(measurements []Measurement) (jointConfidence float64, sourceCount int) {
	sources := make(map[string]float64)
	anonymous := make([]float64, 0, len(measurements))

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

	// Effective independent count per family is sqrt(k): each member's
	// noisy-OR exponent is 1/sqrt(k) so within-family corroboration does not
	// compound as if the sources were independent.
	familyCount := make(map[string]int)

	for source := range sources {
		familyCount[sourceFamily(source)]++
	}

	complement := 1.0

	for source, confidence := range sources {
		weight := 1.0

		if count := familyCount[sourceFamily(source)]; count > 1 {
			weight = 1.0 / math.Sqrt(float64(count))
		}

		complement *= math.Pow(1-clampUnit01(confidence), weight)
	}

	// Sourceless measurements are treated as fully independent.
	for _, confidence := range anonymous {
		complement *= 1 - clampUnit01(confidence)
	}

	if complement >= 1 {
		return 0, len(sources)
	}

	return 1 - complement, len(sources)
}

func clampUnit01(value float64) float64 {
	if value < 0 {
		return 0
	}

	if value > 1 {
		return 1
	}

	return value
}

// sourceFamily groups signals that consume the same market-data stream and are
// therefore positively correlated. Members of one family are down-weighted in
// fusion so co-firing does not overstate joint confidence. Any source not
// listed is its own family (treated as independent).
func sourceFamily(source string) string {
	switch source {
	case "depthflow", "fluid", "hawkes", "pumpdump", "cvd", "bookflow":
		return "microstructure"
	case "correlation", "leadlag", "liquidity", "sentiment":
		return "cross_section"
	default:
		return source
	}
}
