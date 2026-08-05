package audit

import (
	"fmt"
	"sort"

	"github.com/theapemachine/symm/types"
)

/*
DecisionEvents projects one completed Thesis cycle into durable facts. It keeps
only the latest measurement for each source/peer identity that was available at
decision time; the complete raw series remains owned by Thesis.
*/
func DecisionEvents(thesis *types.Thesis) []Event {
	if thesis == nil || len(thesis.Decisions) == 0 {
		return nil
	}

	events := make([]Event, 0, len(thesis.Decisions)*4)

	for _, stored := range thesis.Decisions {
		decision := stored

		// A deferral with no forecast has no model timestamp of its own. Thesis.At
		// is the authoritative time of the evaluation that emitted it.
		if decision.At.IsZero() {
			decision.At = thesis.At
		}

		context := decisionContext(thesis.Tick, decision)

		if context.Validate() == nil {
			events = append(events, context)
		}

		forecast := forecastIssued(thesis.Tick, decision)

		if forecast.Validate() == nil {
			events = append(events, forecast)
		}

		events = append(events, evidenceEvents(thesis, decision)...)
	}

	return events
}

/*
RecordDecisionCycle writes the curated projection before the planner closes the
canonical Thesis cycle.
*/
func RecordDecisionCycle(recorder *Recorder, thesis *types.Thesis) error {
	for _, event := range DecisionEvents(thesis) {
		if err := Record(recorder, event); err != nil {
			return err
		}
	}

	return nil
}

func decisionContext(tick int64, decision types.Decision) DecisionContext {
	return DecisionContext{
		DecisionID:       decision.ID,
		Symbol:           decision.Symbol,
		At:               decision.At,
		Tick:             tick,
		Action:           decision.Action,
		Cause:            decision.Cause,
		Reason:           decision.Reason,
		Utility:          decision.Utility,
		Alternatives:     decision.Alternatives,
		Opportunity:      decision.Opportunity,
		AllocationClass:  decision.AllocationClass,
		AllocationCut:    decision.AllocationHaircut,
		AllocationReason: decision.AllocationHaircutReason,
		ProposedQuantity: decision.ProposedQuantity,
		ProposedNotional: decision.ProposedNotional,
		Risk:             decision.Risk,
		Trace:            decision.Trace,
	}
}

func forecastIssued(tick int64, decision types.Decision) ForecastIssued {
	horizonSteps := 0

	if decision.Trace != nil {
		horizonSteps = decision.Trace.MCTS.HorizonSteps
	}

	if horizonSteps <= 0 && tick > 0 &&
		decision.ValidThroughEpoch > uint64(tick) {
		horizonSteps = int(decision.ValidThroughEpoch - uint64(tick))
	}

	return ForecastIssued{
		DecisionID:       decision.ID,
		Symbol:           decision.Symbol,
		IssuedAt:         decision.At,
		Source:           decision.ForecastSource,
		Model:            decision.ForecastModel,
		Epoch:            decision.ForecastEpoch,
		ValidThrough:     decision.ValidThroughEpoch,
		HorizonSteps:     horizonSteps,
		CalibrationCount: decision.CalibrationCount,
		ReferencePrice:   decision.ReferencePrice,
		ExpectedReturn:   decision.ExpectedReturn,
		ExpectedFees:     decision.ExpectedFees,
		ExpectedSpread:   decision.ExpectedSpread,
		ExpectedImpact:   decision.ExpectedImpact,
		Confidence:       decision.Confidence,
		Uncertainty:      decision.Uncertainty,
	}
}

func evidenceEvents(thesis *types.Thesis, decision types.Decision) []Event {
	latest := latestEvidence(thesis, decision)
	keys := make([]string, 0, len(latest))

	for key := range latest {
		keys = append(keys, key)
	}

	sort.Strings(keys)
	events := make([]Event, 0, len(keys))
	required := requiredEvidence(decision)

	for _, key := range keys {
		measurement := latest[key]

		if evidence := validSignalEvidence(decision, measurement); evidence != nil {
			events = append(events, *evidence)
			delete(required, measurement.Source)
			continue
		}

		if _, material := required[measurement.Source]; !material {
			continue
		}

		events = append(events, unavailableMeasurement(decision, measurement))
		delete(required, measurement.Source)
	}

	missing := make([]string, 0, len(required))

	for source := range required {
		missing = append(missing, string(source))
	}

	sort.Strings(missing)

	for _, sourceName := range missing {
		source := types.SourceType(sourceName)
		events = append(events, EvidenceUnavailable{
			DecisionID: decision.ID,
			Symbol:     decision.Symbol,
			At:         decision.At,
			Source:     source,
			Reason:     required[source],
		})
	}

	return events
}

func latestEvidence(
	thesis *types.Thesis,
	decision types.Decision,
) map[string]*types.Measurement {
	latest := make(map[string]*types.Measurement)

	for _, measurement := range thesis.Series(decision.Symbol) {
		if measurement == nil || measurement.At.IsZero() ||
			decision.At.IsZero() || measurement.At.After(decision.At) {
			continue
		}

		key := string(measurement.Source) + ":" + measurement.Peer
		latest[key] = measurement
	}

	return latest
}

func validSignalEvidence(
	decision types.Decision,
	measurement *types.Measurement,
) *SignalEvidence {
	if measurement == nil || measurement.Validity.State != types.ValidityValid {
		return nil
	}

	copy := *measurement
	copy.Metrics = make(map[string]types.MetricSample, len(measurement.Metrics))

	for key, sample := range measurement.Metrics {
		if !finite(sample.Raw) ||
			(sample.Normalized != nil && !finite(*sample.Normalized)) {
			continue
		}

		copy.Metrics[key] = sample
	}

	event := SignalEvidence{
		DecisionID: decision.ID,
		DecisionAt: decision.At,
		Evidence:   &copy,
	}

	if event.Validate() != nil {
		return nil
	}

	return &event
}

func unavailableMeasurement(
	decision types.Decision,
	measurement *types.Measurement,
) EvidenceUnavailable {
	reason := measurement.Validity.Reason

	if reason == "" {
		reason = fmt.Sprintf(
			"%s evidence was not valid for %s",
			measurement.Source,
			decision.Cause,
		)
	}

	return EvidenceUnavailable{
		DecisionID: decision.ID,
		Symbol:     decision.Symbol,
		At:         decision.At,
		Source:     measurement.Source,
		State:      measurement.Validity.State,
		Readiness:  measurement.Validity.Readiness,
		Reason:     reason,
	}
}

func requiredEvidence(decision types.Decision) map[types.SourceType]string {
	required := make(map[types.SourceType]string)

	switch decision.Cause {
	case "no_forecast":
		required[types.SourceResonance] = decision.Reason
	case "hold_discount_unavailable":
		required[types.SourceExhaustion] = decision.Reason
	case "hawkes_propagation_unavailable":
		required[types.SourceHawkes] = decision.Reason
	case "allocation_evidence_unavailable":
		required[types.SourceLiquidity] = decision.Reason
		required[types.SourceToxicity] = decision.Reason
	case "continuation":
		if decision.ForecastSource == "" {
			required[types.SourceResonance] = decision.Reason
		}
	}

	return required
}
