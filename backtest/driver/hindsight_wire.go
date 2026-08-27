package driver

import (
	"sort"

	"github.com/theapemachine/symm/backtest/hindsight"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
publishHindsight emits one hindsight wire frame for the dashboard.
*/
func (driver *Driver) publishHindsight(report RealizedReport) {
	if driver.ui != nil {
		driver.ui.Publish(types.ChannelUI, &types.UIFrame{
			Type:  wire.FrameHindsightFrame,
			Value: hindsightWire(report),
		})
	}
}

func hindsightWire(report RealizedReport) *wire.HindsightFrameT {
	symbols := make([]*wire.HindsightSymbolT, 0, len(report.Symbols))

	for _, symbol := range report.Symbols {
		opportunities := make([]*wire.HindsightOpportunityT, 0, len(symbol.Opportunities))

		for _, opportunity := range symbol.Opportunities {
			journal := make([]*wire.HindsightSignalT, 0, len(opportunity.Journal))

			for _, decision := range opportunity.Journal {
				journal = append(journal, hindsightDecisionSignalWire(decision))
			}

			opportunities = append(opportunities, &wire.HindsightOpportunityT{
				Leg: &wire.HindsightLegT{
					Symbol:         opportunity.Leg.Symbol,
					BuyAt:          opportunity.Leg.BuyAt.UnixNano(),
					SellAt:         opportunity.Leg.SellAt.UnixNano(),
					BuyPrice:       opportunity.Leg.BuyPrice,
					SellPrice:      opportunity.Leg.SellPrice,
					ProfitPct:      opportunity.Leg.ProfitPct,
					GrossProfitPct: opportunity.Leg.GrossProfitPct,
					FrictionPct:    opportunity.Leg.FrictionPct,
				},
				Signal:    hindsightSignalWire(opportunity.Signal),
				Journal:   journal,
				Diagnosis: hindsightDiagnosisWire(opportunity.Diagnosis),
				Why:       opportunity.Why,
				Captured:  opportunity.Captured,
				Missed:    opportunity.Missed,
			})
		}

		losses := make([]*wire.HindsightLossT, 0, len(symbol.Losses))

		for _, loss := range symbol.Losses {
			lossJournal := make([]*wire.HindsightSignalT, 0, len(loss.Journal))

			for _, decision := range loss.Journal {
				lossJournal = append(lossJournal, hindsightDecisionSignalWire(decision))
			}

			losses = append(losses, &wire.HindsightLossT{
				Symbol:        loss.Symbol,
				DecisionId:    loss.DecisionID,
				EntryAt:       loss.EntryAt.UnixNano(),
				ExitAt:        loss.ExitAt.UnixNano(),
				EntryPrice:    loss.EntryPrice,
				ExitPrice:     loss.ExitPrice,
				Pnl:           loss.LossPerUnit,
				ReturnPct:     loss.ReturnPct,
				GrossPct:      loss.GrossPct,
				FrictionPct:   loss.FrictionPct,
				TriggerReason: loss.TriggerReason,
				Diagnosis:     hindsightDiagnosisWire(loss.Diagnosis),
				Signal:        hindsightSignalWire(loss.Signal),
				Journal:       lossJournal,
			})
		}

		symbols = append(symbols, &wire.HindsightSymbolT{
			Symbol:        symbol.Symbol,
			UpboundPct:    symbol.UpboundPct,
			RealizedPct:   symbol.RealizedPct,
			MissedPct:     symbol.MissedPct,
			LossPct:       symbol.LossPct,
			Legs:          int64(symbol.Legs),
			MissedLegs:    int64(symbol.MissedLegs),
			LossPositions: int64(symbol.LossPositions),
			Opportunities: opportunities,
			Losses:        losses,
		})
	}

	rootCauses := hindsightRootCausesWire(report.RootCauses)

	recommendations := make([]*wire.HindsightRecommendationT, 0, len(report.Recommendations))

	for _, recommendation := range report.Recommendations {
		recommendations = append(
			recommendations,
			hindsightRecommendationWire(recommendation),
		)
	}

	lossRootCauses := hindsightRootCausesWire(report.LossRootCauses)

	lossRecommendations := make([]*wire.HindsightRecommendationT, 0, len(report.LossRecommendations))

	for _, recommendation := range report.LossRecommendations {
		lossRecommendations = append(
			lossRecommendations,
			hindsightRecommendationWire(recommendation),
		)
	}

	return &wire.HindsightFrameT{
		CaptureId:           report.CaptureID,
		Status:              report.Status,
		Symbols:             symbols,
		MissedPct:           report.MissedPct,
		UpboundPct:          report.UpboundPct,
		MissedLegs:          int64(report.MissedLegs),
		TotalLegs:           int64(report.TotalLegs),
		RealizedPct:         report.RealizedPct,
		LossPct:             report.LossPct,
		LossPositions:       int64(report.LossPositions),
		ValueCaptureRate:    report.ValueCaptureRate,
		LegCaptureRate:      report.LegCaptureRate,
		DiagnosticCoverage:  report.DiagnosticCoverage,
		RootCauses:          rootCauses,
		Recommendations:     recommendations,
		LossRootCauses:      lossRootCauses,
		LossRecommendations: lossRecommendations,
	}
}

/*
hindsightDecisionSignalWire projects one journal decision onto the wire signal
shape. Journal entries expose the raw Decision opportunity type.
*/
func hindsightDecisionSignalWire(decision hindsight.Decision) *wire.HindsightSignalT {
	return &wire.HindsightSignalT{
		Id:                  decision.ID,
		At:                  decision.At.UnixNano(),
		Action:              decision.Action,
		Reason:              decision.Reason,
		Cause:               decision.Cause,
		GraphScore:          decision.GraphScore,
		ThesisScore:         decision.ThesisScore,
		ThesisConfidence:    decision.ThesisConfidence,
		ThesisSupport:       decision.ThesisSupport,
		ThesisContradiction: decision.ThesisContradiction,
		ThesisConditions:    decision.ThesisConditions,
		Direction:           decision.Direction,
		Confidence:          decision.Confidence,
		AdmissionThreshold:  decision.AdmissionThreshold,
		Opportunity:         decision.Opportunity,
		OpportunityType:     decision.OpportunityType,
		PredictiveReady:     decision.PredictiveReady,
		PredictiveStatus:    decision.PredictiveStatus,
		Alternatives:        hindsightNumbers(decision.Alternatives),
	}
}

/*
hindsightSignalWire projects one entry-signal context onto the wire signal
shape. Signal contexts carry the opportunity type in their Type field.
*/
func hindsightSignalWire(signal hindsight.SignalContext) *wire.HindsightSignalT {
	return &wire.HindsightSignalT{
		Id:                  signal.ID,
		At:                  signal.At.UnixNano(),
		Action:              signal.Action,
		Reason:              signal.Reason,
		Cause:               signal.Cause,
		GraphScore:          signal.GraphScore,
		ThesisScore:         signal.ThesisScore,
		ThesisConfidence:    signal.ThesisConfidence,
		ThesisSupport:       signal.ThesisSupport,
		ThesisContradiction: signal.ThesisContradiction,
		ThesisConditions:    signal.ThesisConditions,
		Direction:           signal.Direction,
		Confidence:          signal.Confidence,
		AdmissionThreshold:  signal.AdmissionThreshold,
		Opportunity:         signal.Opportunity,
		OpportunityType:     signal.Type,
		PredictiveReady:     signal.PredictiveReady,
		PredictiveStatus:    signal.PredictiveStatus,
		Alternatives:        hindsightNumbers(signal.Alternatives),
	}
}

/*
hindsightRootCausesWire projects a root-cause summary list onto the wire shape.
*/
func hindsightRootCausesWire(
	causes []hindsight.RootCauseSummary,
) []*wire.HindsightRootCauseT {
	result := make([]*wire.HindsightRootCauseT, 0, len(causes))

	for _, cause := range causes {
		result = append(result, &wire.HindsightRootCauseT{
			Category:    cause.Category,
			ImpactPct:   cause.ImpactPct,
			Occurrences: int64(cause.Occurrences),
			Symbols:     cause.Symbols,
		})
	}

	return result
}

func hindsightDiagnosisWire(
	diagnosis hindsight.Diagnosis,
) *wire.HindsightDiagnosisT {
	if diagnosis.Category == "" && diagnosis.Summary == "" {
		return nil
	}

	blockers := make([]*wire.HindsightBlockerT, 0, len(diagnosis.Blockers))

	for _, blocker := range diagnosis.Blockers {
		blockers = append(blockers, &wire.HindsightBlockerT{
			Key:         blocker.Key,
			Category:    blocker.Category,
			Label:       blocker.Label,
			Source:      blocker.Source,
			Observed:    blocker.Observed,
			Target:      blocker.Target,
			HasTarget:   blocker.HasTarget,
			Gap:         blocker.Gap,
			Severity:    blocker.Severity,
			Explanation: blocker.Explanation,
		})
	}

	return &wire.HindsightDiagnosisT{
		Category:        diagnosis.Category,
		Summary:         diagnosis.Summary,
		EvidenceQuality: diagnosis.EvidenceQuality,
		EvidenceStatus:  diagnosis.EvidenceStatus,
		Blockers:        blockers,
		Recommendation:  hindsightRecommendationWire(diagnosis.Recommendation),
	}
}

func hindsightRecommendationWire(
	recommendation hindsight.Recommendation,
) *wire.HindsightRecommendationT {
	if recommendation.Key == "" {
		return nil
	}

	return &wire.HindsightRecommendationT{
		Key:          recommendation.Key,
		Kind:         recommendation.Kind,
		Target:       recommendation.Target,
		Title:        recommendation.Title,
		Action:       recommendation.Action,
		Rationale:    recommendation.Rationale,
		Current:      recommendation.Current,
		Suggested:    recommendation.Suggested,
		HasCurrent:   recommendation.HasCurrent,
		HasSuggested: recommendation.HasSuggested,
		Adjustment:   recommendation.Adjustment,
		Confidence:   recommendation.Confidence,
		ImpactPct:    recommendation.ImpactPct,
		Occurrences:  int64(recommendation.Occurrences),
		Symbols:      recommendation.Symbols,
	}
}

func hindsightNumbers(values map[string]float64) []*wire.NamedNumberT {
	names := make([]string, 0, len(values))

	for name := range values {
		names = append(names, name)
	}

	sort.Strings(names)
	result := make([]*wire.NamedNumberT, 0, len(names))

	for _, name := range names {
		result = append(result, &wire.NamedNumberT{Name: name, Value: values[name]})
	}

	return result
}