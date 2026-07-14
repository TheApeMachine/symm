package logic

import (
	"math"

	"github.com/theapemachine/symm/types"
)

func latestMeasurement(
	symbol string,
	measurements []*types.Measurement,
	subject types.SubjectType,
	metric types.MetricType,
) (*types.Measurement, bool) {
	var latest *types.Measurement

	for _, measurement := range measurements {
		if measurement == nil ||
			measurement.Symbol != symbol ||
			measurement.Subject != subject ||
			measurement.Metric != metric ||
			measurement.Validity.State != types.ValidityValid {
			continue
		}

		if latest == nil || measurement.At.After(latest.At) {
			latest = measurement
		}
	}

	return latest, latest != nil
}

func latestSideMeasurement(
	symbol string,
	measurements []*types.Measurement,
	subject types.SubjectType,
	metric types.MetricType,
	side types.MeasurementSide,
) (*types.Measurement, bool) {
	var latest *types.Measurement

	for _, measurement := range measurements {
		if measurement == nil ||
			measurement.Symbol != symbol ||
			measurement.Subject != subject ||
			measurement.Metric != metric ||
			measurement.Side != side ||
			measurement.Validity.State != types.ValidityValid {
			continue
		}

		if latest == nil || measurement.At.After(latest.At) {
			latest = measurement
		}
	}

	return latest, latest != nil
}

func missingSubjects(
	symbol string,
	measurements []*types.Measurement,
	required []types.SubjectType,
) []string {
	missing := make([]string, 0, len(required))

	for _, subject := range required {
		if _, ok := latestSubject(symbol, measurements, subject); !ok {
			missing = append(missing, string(subject))
		}
	}

	return missing
}

func latestSubject(
	symbol string,
	measurements []*types.Measurement,
	subject types.SubjectType,
) (*types.Measurement, bool) {
	var latest *types.Measurement

	for _, measurement := range measurements {
		if measurement == nil ||
			measurement.Symbol != symbol ||
			measurement.Subject != subject ||
			measurement.Validity.State != types.ValidityValid {
			continue
		}

		if latest == nil || measurement.At.After(latest.At) {
			latest = measurement
		}
	}

	return latest, latest != nil
}

func graphEvidence(
	graph *types.Graph,
	anchorKey string,
) ([]string, []string) {
	if graph == nil || anchorKey == "" {
		return nil, nil
	}

	supporting := make([]string, 0)
	opposing := make([]string, 0)

	for _, edge := range graph.Edges {
		if edge.To != anchorKey {
			continue
		}

		switch edge.Type {
		case types.Supports:
			supporting = append(supporting, edge.From)
		case types.Contradicts:
			opposing = append(opposing, edge.From)
		}
	}

	return supporting, opposing
}

func weightedValue(measurement types.Measurement) float64 {
	if measurement.Maturity <= 0 {
		return 0
	}

	return math.Abs(measurement.Raw) * measurement.Maturity
}

func evidenceConfidence(
	maturity float64,
	supportCount int,
	opposeCount int,
	missingCount int,
) float64 {
	if maturity <= 0 {
		return 0
	}

	denominator := 1.0 + float64(opposeCount+missingCount)
	numerator := 1.0 + float64(supportCount)

	return maturity * numerator / (maturity + denominator)
}
