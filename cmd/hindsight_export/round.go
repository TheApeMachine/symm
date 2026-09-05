package main

import (
	"math"
	"strings"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func buildRound(
	runID string,
	sequence uint64,
	ordinal uint64,
	state *telemetry.EnvelopeState,
	decision *telemetry.Decision,
	includeMetrics bool,
) round {
	record := round{
		Run:              runID,
		Sequence:         sequence,
		Ordinal:          ordinal,
		Tick:             state.Tick(),
		Symbol:           string(decision.Symbol()),
		At:               decision.At(),
		Action:           string(decision.Action()),
		PredictiveStatus: string(decision.PredictiveStatus()),
		PredictiveReady:  decision.PredictiveReady(),
		Reason:           string(decision.Reason()),
		Cause:            string(decision.Cause()),
		Confidence:       decision.Confidence(),
		ForecastSource:   string(decision.ForecastSource()),
		ForecastModel:    string(decision.ForecastModel()),
		ForecastHorizon:  decision.ForecastHorizon(),
		CalibrationCount: decision.CalibrationCount(),
		ReferencePrice:   string(decision.ReferencePrice()),
		ProposedNotional: string(decision.ProposedNotional()),
		TaskSkill:        decision.TaskSkill(),
	}

	for index := range decision.AlternativesLength() {
		alternative := new(telemetry.NamedNumber)

		if !decision.Alternatives(alternative, index) {
			continue
		}

		name := string(alternative.Name())
		value := alternative.Value()

		switch {
		case strings.HasPrefix(name, "move:"):
			record.Probabilities = put(record.Probabilities,
				strings.TrimPrefix(name, "move:"), value)
		case strings.HasPrefix(name, "consensus:"):
			record.Consensus = put(record.Consensus,
				strings.TrimPrefix(name, "consensus:"), value)
		case strings.HasPrefix(name, "execution:"):
			record.Execution = put(record.Execution,
				strings.TrimPrefix(name, "execution:"), value)
		}
	}


	if includeMetrics && state != nil {
		record.Metrics = extractAllMetrics(state)
	}

	return record
}

func extractAllMetrics(state *telemetry.EnvelopeState) map[string]float64 {
	if state == nil {
		return nil
	}

	var metrics map[string]float64

	metrics = extractMeasurement(metrics, "cvd", state.Cvd(nil))
	metrics = extractMeasurement(metrics, "pumpdump", state.PumpDump(nil))
	metrics = extractMeasurement(metrics, "derivatives", state.Derivatives(nil))
	metrics = extractMeasurement(metrics, "toxicity", state.Toxicity(nil))
	metrics = extractMeasurement(metrics, "hawkes", state.Hawkes(nil))
	metrics = extractMeasurement(metrics, "liquidity", state.Liquidity(nil))
	metrics = extractMeasurement(metrics, "depthflow", state.DepthFlow(nil))
	metrics = extractMeasurement(metrics, "morphology", state.Morphology(nil))
	metrics = extractMeasurement(metrics, "leadlag", state.LeadLag(nil))
	metrics = extractMeasurement(metrics, "correlation", state.Correlation(nil))
	metrics = extractMeasurement(metrics, "sentiment", state.Sentiment(nil))

	return metrics
}

func extractMeasurement(
	target map[string]float64,
	prefix string,
	measurement *telemetry.EnvelopeMeasurement,
) map[string]float64 {
	if measurement == nil {
		return target
	}

	authority := measurementAuthority(measurement)

	for index := range measurement.MetricsLength() {
		metric := new(telemetry.EnvelopeMeasurementMetric)

		if !measurement.Metrics(metric, index) {
			continue
		}

		value := metric.Value(nil)

		if value == nil {
			continue
		}

		metricValue := value.Raw()

		if string(value.Unit()) != string(data.UnitCount) &&
			!strings.Contains(string(value.Label()), "ordinal") {
			metricValue *= authority
		}

		target = put(target, prefix+"/"+string(metric.Key()), metricValue)
	}

	return target
}

/* measurementAuthority mirrors data.Readout.Value for persisted measurements. */
func measurementAuthority(measurement *telemetry.EnvelopeMeasurement) float64 {
	maturity := math.Max(0, math.Min(1, measurement.Maturity()))
	estimated := false

	for index := range measurement.MetadataLength() {
		metadata := new(telemetry.NamedNumber)

		if !measurement.Metadata(metadata, index) {
			continue
		}

		name := string(metadata.Name())

		if name == data.MetadataSupport || name == data.MetadataDivergence ||
			name == data.MetadataMahalanobisSNR {
			estimated = true
		}
	}

	if !estimated {
		return maturity
	}

	snrFactor := 0.5

	if measurement.SnrDefined() && measurement.Snr() <= 0 {
		snrFactor = 0.1
	}

	if measurement.SnrDefined() && measurement.Snr() > 0 {
		snrFactor = measurement.Snr() / (1 + measurement.Snr())
	}

	return maturity * snrFactor
}

func put(target map[string]float64, key string, value float64) map[string]float64 {
	if target == nil {
		target = make(map[string]float64)
	}

	target[key] = value

	return target
}

