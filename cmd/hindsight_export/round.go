package main

import (
	"math"
	"strings"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
buildPerspectiveRecord projects one durable Perspective without changing its
event-time contract. Issued records are optimizer inputs; later records remain
available as auditable lifecycle evidence.
*/
func buildPerspectiveRecord(
	runID string,
	sequence uint64,
	ordinal uint64,
	tick int64,
	perspective *telemetry.EnvelopePerspective,
	metrics map[string]float64,
) perspectiveRecord {
	record := perspectiveRecord{
		Kind:               "perspective",
		Run:                runID,
		Sequence:           sequence,
		Ordinal:            ordinal,
		Tick:               tick,
		Symbol:             string(perspective.Symbol()),
		Advisor:            string(perspective.Advisor()),
		ClaimSequence:      perspective.Sequence(),
		Classes:            make(map[string]float64, perspective.ClassesLength()),
		Evidence:           make(map[string][]string, perspective.ClassesLength()),
		Metrics:            metrics,
		IssuedAt:           perspective.IssuedAt(),
		ResolvedAt:         perspective.ResolvedAt(),
		ResolvedCoordinate: perspective.ResolvedCoordinate(),
		Round:              perspective.Round(),
		Lifecycle:          string(perspective.Lifecycle()),
		ResolvedBy:         string(perspective.ResolvedBy()),
		Predictions:        make([]predictionRecord, 0, perspective.PredictionsLength()),
	}

	lease := perspective.Lease(nil)

	if lease != nil {
		record.Clock = string(lease.Clock())
		record.LeaseFrom = lease.From()
		record.LeaseUntil = lease.Until()
	}

	var highestProbability float64

	for classIndex := range perspective.ClassesLength() {
		perspectiveClass := new(telemetry.EnvelopePerspectiveClass)

		if !perspective.Classes(perspectiveClass, classIndex) {
			continue
		}

		className := string(perspectiveClass.State())
		probability := perspectiveClass.Probability()
		record.Classes[className] = probability
		evidence := make([]string, 0, perspectiveClass.EvidenceLength())

		for evidenceIndex := range perspectiveClass.EvidenceLength() {
			evidence = append(evidence, string(perspectiveClass.Evidence(evidenceIndex)))
		}

		record.Evidence[className] = evidence

		if probability > highestProbability {
			highestProbability = probability
			record.Class = className
		}
	}

	for predictionIndex := range perspective.PredictionsLength() {
		prediction := new(telemetry.EnvelopePerspectivePrediction)

		if !perspective.Predictions(prediction, predictionIndex) {
			continue
		}

		record.Predictions = append(record.Predictions, predictionRecord{
			Class:  string(prediction.Class()),
			Event:  string(prediction.Event()),
			Effect: string(prediction.Effect()),
			Move:   string(prediction.Move()),
		})
	}

	return record
}

/*
buildRound projects one persisted Decision onto the shared record.

The alternatives map is the planner's own key/value record of the round, and
its keys are already namespaced by concern (move:, consensus:, execution:,
branch:, search:). Splitting them back out here keeps the exported object
readable without inventing any value the planner did not write.
*/
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

	record.Search = buildSearch(decision.Trace(nil))

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

/*
buildSearch projects the causal search's trace. A round that stopped at an
earlier gate has no trace, and reports none rather than an empty search that
would read as a search that found nothing.
*/
func buildSearch(trace *telemetry.DecisionTrace) *searchRound {
	if trace == nil {
		return nil
	}

	search := &searchRound{
		RecommendedAction:    string(trace.RecommendedAction()),
		IdentificationStatus: string(trace.IdentificationStatus()),
		DecisionUnavailable:  trace.DecisionUnavailable(),
		Iterations:           trace.Iterations(),
		Horizon:              trace.Horizon(),
		MaxDepth:             trace.MaxDepth(),
		TotalNodes:           trace.TotalNodes(),
		ExpectedOutcome:      trace.ExpectedOutcome(),
		OutcomeUncertainty:   trace.OutcomeUncertainty(),
		TransitionSource:     string(trace.TransitionSource()),
		DominantMove:         string(trace.ConsensusDominantMove()),
		Participants:         trace.ConsensusParticipants(),
	}

	for index := range trace.VetoesLength() {
		search.Vetoes = append(search.Vetoes, string(trace.Vetoes(index)))
	}

	for index := range trace.SynergiesLength() {
		search.Synergies = append(search.Synergies, string(trace.Synergies(index)))
	}

	for index := range trace.BranchesLength() {
		branch := new(telemetry.MCTSBranch)

		if !trace.Branches(branch, index) {
			continue
		}

		search.Branches = append(search.Branches, searchBranch{
			Action:             string(branch.Action()),
			Visits:             branch.Visits(),
			MeanReward:         branch.MeanReward(),
			BlendedValue:       branch.BlendedValue(),
			RewardStd:          branch.RewardStd(),
			CounterfactualMass: branch.CounterfactualMass(),
			CausalExpectation:  branch.CausalExpectation(),
			CausalDefined:      branch.CausalExpectationDefined(),
			Pruned:             branch.Pruned(),
		})
	}

	return search
}
