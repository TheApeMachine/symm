package driver

import (
	"sort"
	"time"

	"github.com/theapemachine/symm/backtest/hindsight"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
publishHindsight emits one hindsight wire frame for the dashboard.
*/
func (driver *Driver) publishHindsight(report RealizedReport) {
	driver.ui.Push(&wire.FrameT{
		Type:  wire.FrameHindsightFrame,
		Value: hindsightWire(report),
	})
}

func hindsightWire(report RealizedReport) *wire.HindsightFrameT {
	symbols := make([]*wire.HindsightSymbolT, 0, len(report.Symbols))

	for _, symbol := range report.Symbols {
		opportunities := make([]*wire.HindsightOpportunityT, 0, len(symbol.Opportunities))

		for _, opportunity := range symbol.Opportunities {
			journal := make([]*wire.HindsightSignalT, 0, len(opportunity.Journal))

			for _, decision := range opportunity.Journal {
				journal = append(journal, hindsightSignalWire(signalWireFields{
					At:                  decision.At,
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
					Alternatives:        decision.Alternatives,
				}))
			}

			opportunities = append(opportunities, &wire.HindsightOpportunityT{
				Leg: &wire.HindsightLegT{
					Symbol: opportunity.Leg.Symbol, BuyAt: opportunity.Leg.BuyAt.UnixNano(),
					SellAt: opportunity.Leg.SellAt.UnixNano(), BuyPrice: opportunity.Leg.BuyPrice,
					SellPrice: opportunity.Leg.SellPrice, ProfitPct: opportunity.Leg.ProfitPct,
				},
				Signal: hindsightSignalWire(signalWireFields{
					At:                  opportunity.Signal.At,
					Action:              opportunity.Signal.Action,
					Reason:              opportunity.Signal.Reason,
					Cause:               opportunity.Signal.Cause,
					GraphScore:          opportunity.Signal.GraphScore,
					ThesisScore:         opportunity.Signal.ThesisScore,
					ThesisConfidence:    opportunity.Signal.ThesisConfidence,
					ThesisSupport:       opportunity.Signal.ThesisSupport,
					ThesisContradiction: opportunity.Signal.ThesisContradiction,
					ThesisConditions:    opportunity.Signal.ThesisConditions,
					Direction:           opportunity.Signal.Direction,
					Confidence:          opportunity.Signal.Confidence,
					AdmissionThreshold:  opportunity.Signal.AdmissionThreshold,
					Opportunity:         opportunity.Signal.Opportunity,
					OpportunityType:     opportunity.Signal.Type,
					PredictiveReady:     opportunity.Signal.PredictiveReady,
					PredictiveStatus:    opportunity.Signal.PredictiveStatus,
					Alternatives:        opportunity.Signal.Alternatives,
				}),
				Journal:   journal,
				Diagnosis: hindsightDiagnosisWire(opportunity.Diagnosis),
				Why:       opportunity.Why,
				Captured:  opportunity.Captured, Missed: opportunity.Missed,
			})
		}

		symbols = append(symbols, &wire.HindsightSymbolT{
			Symbol: symbol.Symbol, UpboundPct: symbol.UpboundPct,
			RealizedPct: symbol.RealizedPct, MissedPct: symbol.MissedPct,
			Legs: int64(symbol.Legs), MissedLegs: int64(symbol.MissedLegs),
			Opportunities: opportunities,
		})
	}

	rootCauses := make([]*wire.HindsightRootCauseT, 0, len(report.RootCauses))

	for _, cause := range report.RootCauses {
		rootCauses = append(rootCauses, &wire.HindsightRootCauseT{
			Category:    cause.Category,
			ImpactPct:   cause.ImpactPct,
			Occurrences: int64(cause.Occurrences),
			Symbols:     cause.Symbols,
		})
	}

	recommendations := make([]*wire.HindsightRecommendationT, 0, len(report.Recommendations))

	for _, recommendation := range report.Recommendations {
		recommendations = append(
			recommendations,
			hindsightRecommendationWire(recommendation),
		)
	}

	return &wire.HindsightFrameT{
		CaptureId: report.CaptureID, Status: report.Status, Symbols: symbols,
		MissedPct: report.MissedPct, UpboundPct: report.UpboundPct,
		MissedLegs: int64(report.MissedLegs), TotalLegs: int64(report.TotalLegs),
		RealizedPct: report.RealizedPct, ValueCaptureRate: report.ValueCaptureRate,
		LegCaptureRate: report.LegCaptureRate, DiagnosticCoverage: report.DiagnosticCoverage,
		RootCauses: rootCauses, Recommendations: recommendations,
	}
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

/*
signalWireFields is the shared flat shape for one decision moment on the wire,
whether it is the signal pinned to a missed leg or one entry in its journal.
*/
type signalWireFields struct {
	At                  time.Time
	Action              string
	Reason              string
	Cause               string
	GraphScore          float64
	ThesisScore         float64
	ThesisConfidence    float64
	ThesisSupport       float64
	ThesisContradiction float64
	ThesisConditions    float64
	Direction           float64
	Confidence          float64
	AdmissionThreshold  float64
	Opportunity         bool
	OpportunityType     string
	PredictiveReady     bool
	PredictiveStatus    string
	Alternatives        map[string]float64
}

/*
hindsightSignalWire encodes one decision moment with its full thesis context.
*/
func hindsightSignalWire(fields signalWireFields) *wire.HindsightSignalT {
	return &wire.HindsightSignalT{
		At:                  fields.At.UnixNano(),
		Action:              fields.Action,
		Reason:              fields.Reason,
		Cause:               fields.Cause,
		GraphScore:          fields.GraphScore,
		ThesisScore:         fields.ThesisScore,
		ThesisConfidence:    fields.ThesisConfidence,
		ThesisSupport:       fields.ThesisSupport,
		ThesisContradiction: fields.ThesisContradiction,
		ThesisConditions:    fields.ThesisConditions,
		Direction:           fields.Direction,
		Confidence:          fields.Confidence,
		AdmissionThreshold:  fields.AdmissionThreshold,
		Opportunity:         fields.Opportunity,
		OpportunityType:     fields.OpportunityType,
		PredictiveReady:     fields.PredictiveReady,
		PredictiveStatus:    fields.PredictiveStatus,
		Alternatives:        hindsightNumbers(fields.Alternatives),
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
