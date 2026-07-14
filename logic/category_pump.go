package logic

import "github.com/theapemachine/symm/types"

func (composer *CategoryComposer) pumpLifecycle(
	symbol string,
	measurements []*types.Measurement,
	graph *types.Graph,
) (types.Category, bool) {
	dominant, totalWeight, ok := dominantPumpMeasurement(symbol, measurements)

	if !ok || totalWeight <= 0 {
		return types.Category{}, false
	}

	categoryType := pumpCategoryType(dominant.Measurement.Subject)

	if categoryType == types.CategoryTypeNone {
		return types.Category{}, false
	}

	required := []types.SubjectType{
		types.SubjectTradeArrivals,
		types.SubjectPeerLiquidity,
		types.SubjectBookImbalance,
		types.SubjectLevel3Tape,
	}
	missing := missingSubjects(symbol, measurements, required)
	dominantKey := types.MeasurementKey(&dominant.Measurement)
	supporting, opposing := graphEvidence(graph, dominantKey)

	strength := weightedValue(dominant.Measurement) / totalWeight
	confidence := evidenceConfidence(
		dominant.Measurement.Maturity,
		len(supporting),
		len(opposing),
		len(missing),
	)

	return types.Category{
		Symbol:     symbol,
		Type:       categoryType,
		Strength:   strength,
		Confidence: confidence,
		Surprisal:  1 - confidence,
		Maturity:   dominant.Measurement.Maturity,
		Supporting: supporting,
		Opposing:   opposing,
		Missing:    missing,
	}, true
}

func dominantPumpMeasurement(
	symbol string,
	measurements []*types.Measurement,
) (types.Node, float64, bool) {
	subjects := []types.SubjectType{
		types.SubjectPumpIgnition,
		types.SubjectPumpCompression,
		types.SubjectPumpTrend,
		types.SubjectPumpExhaustion,
		types.SubjectPumpComposite,
	}
	var dominant types.Node
	totalWeight := 0.0
	found := false

	for _, subject := range subjects {
		measurement, ok := latestMeasurement(symbol, measurements, subject, types.MetricStrength)

		if subject == types.SubjectPumpIgnition {
			measurement, ok = latestMeasurement(symbol, measurements, subject, types.MetricIgnition)
		}

		if subject == types.SubjectPumpCompression {
			measurement, ok = latestMeasurement(symbol, measurements, subject, types.MetricCompression)
		}

		if subject == types.SubjectPumpTrend {
			measurement, ok = latestMeasurement(symbol, measurements, subject, types.MetricTrend)
		}

		if subject == types.SubjectPumpExhaustion {
			measurement, ok = latestMeasurement(symbol, measurements, subject, types.MetricExhaustion)
		}

		if !ok || measurement.Source != types.SourcePumpDump {
			continue
		}

		weight := weightedValue(*measurement)
		totalWeight += weight

		if !found || weight > weightedValue(dominant.Measurement) {
			found = true
			dominant = types.Node{
				Key:         types.MeasurementKey(measurement),
				Measurement: *measurement,
			}
		}
	}

	return dominant, totalWeight, found
}

func pumpCategoryType(subject types.SubjectType) types.CategoryType {
	switch subject {
	case types.SubjectPumpIgnition:
		return types.CategoryVerticalIgnition
	case types.SubjectPumpCompression:
		return types.CategoryCoiledCompression
	case types.SubjectPumpTrend:
		return types.CategoryOrganicTrend
	case types.SubjectPumpExhaustion:
		return types.CategoryFadedExhaustion
	case types.SubjectPumpComposite:
		return types.CategoryFrenzy
	default:
		return types.CategoryTypeNone
	}
}
