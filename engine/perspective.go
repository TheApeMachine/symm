package engine

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
FuseMeasurements combines independent source confidences into one alignment score
and counts distinct contributing sources.
*/
func FuseMeasurements(measurements []Measurement) (jointConfidence float64, sourceCount int) {
	factors := make([]float64, 0, len(measurements))
	sources := make(map[string]struct{})

	for _, measurement := range measurements {
		if measurement.Confidence <= 0 {
			continue
		}

		factors = append(factors, measurement.Confidence)

		if measurement.Source != "" {
			sources[measurement.Source] = struct{}{}
		}
	}

	return AlignConfidence(factors...), len(sources)
}
