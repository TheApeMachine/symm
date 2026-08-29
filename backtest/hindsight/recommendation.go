package hindsight

import "fmt"

func recommendationFor(
	blocker Blocker,
	leg Leg,
	confidence float64,
) Recommendation {
	recommendation := Recommendation{
		Key:         blocker.Key,
		Target:      blocker.Source,
		Confidence:  confidence,
		ImpactPct:   leg.ProfitPct,
		Occurrences: 1,
		Symbols:     []string{leg.Symbol},
	}

	switch blocker.Category {
	case DiagnosisDetection:
		recommendation.Kind = RecommendationImproveMeasurement
		recommendation.Target = "opportunity detection"
		recommendation.Title = "Develop an earlier opportunity measurement for this move class"
		recommendation.Action = "Label the move from its actual pre-entry market state, add a same-symbol measurement whose timestamp precedes the leg, and validate onset on held-out captures."
		recommendation.Rationale = "The missed move was absent from the opportunity vocabulary, so selection tuning alone cannot create a reliable early signal."
	case DiagnosisValuation:
		recommendation.Kind = RecommendationCollectOutcomes
		recommendation.Target = "valuation evidence"
		recommendation.Title = "Resolve valuation evidence before selection"
		recommendation.Action = "Retain whether valuation was attempted and available, the causal identification, and the expected return/fees/spread/impact at each decision; re-run hindsight after the valuation path is recorded."
		recommendation.Rationale = "Economic consequence was not estimable at the decision point, so selection could not compare a real alternative."
	case DiagnosisSelection:
		recommendation.Kind = RecommendationInstrumentFunnel
		recommendation.Target = "MCTS selection trace"
		recommendation.Title = "Record the MCTS alternatives before tuning"
		recommendation.Action = "Retain the compared alternatives, their utilities, and the recommended CASH/WAIT action at each declined opportunity; re-run hindsight to separate a selection problem from a valuation problem."
		recommendation.Rationale = "Without the compared alternatives, whether MCTS chose CASH for good reason or because it could not see the opportunity is unknown."
	case DiagnosisExecution:
		recommendation.Kind = RecommendationFixExecution
		recommendation.Target = "execution coverage and depth"
		recommendation.Title = "Make execution feasibility measurable before admission"
		recommendation.Action = "Retain visible depth coverage, spread, impact, fee, and quantity at the decision timestamp; size or reject from those measurements and replay whether a smaller executable order captured net value."
		recommendation.Rationale = "The opportunity existed but the recorded quantity could not be executed through historical depth."
	case DiagnosisAllocation:
		recommendation.Kind = RecommendationFixAllocation
		recommendation.Target = "allocation and slot policy"
		recommendation.Title = "Replay the allocation frontier for qualified opportunities"
		recommendation.Action = "Record capital, slot, and haircut outcomes at the exact rejection point; then simulate the smallest allocation-policy change that would have admitted the missed setup."
		recommendation.Rationale = "The setup reached allocation, so adding signal strength will not recover it unless capacity policy changes."
	case DiagnosisRegulator:
		recommendation.Kind = RecommendationValidateRegulator
		recommendation.Target = "global regulator entry-release policy"
		recommendation.Title = "Replay the regulator release boundary around delayed entries"
		recommendation.Action = "Retain regulator status and release transitions at each delayed decision; replay whether an earlier release captured the move without increasing drawdown."
		recommendation.Rationale = "The opportunity and policy path were ready, but the global regulator held execution."
	case DiagnosisObservability:
		recommendation.Kind = RecommendationImproveAudit
		recommendation.Target = "decision audit stream"
		recommendation.Title = "Close the decision-evidence gap before tuning"
		recommendation.Action = "Retain every evaluated decision for this exact symbol through the outcome horizon, including nothing decisions, valuation results, allocation results, and venue submission status."
		recommendation.Rationale = "Without same-symbol decision evidence, any parameter or measurement recommendation would be post-hoc fiction."
	default:
		recommendation.Kind = RecommendationInstrumentFunnel
		recommendation.Target = "opportunity → valuation → MCTS → execution funnel"
		recommendation.Title = "Identify the first stage that stopped entry"
		recommendation.Action = "Emit one retained stage outcome for detection, valuation, selection, allocation, desk validation, and venue acknowledgement so the next hindsight pass can name the first failed boundary."
		recommendation.Rationale = "The retained reason is insufficient to distinguish a detection problem from a downstream execution problem."
	}

	if recommendation.Target == "" {
		recommendation.Target = blocker.Key
	}

	if blocker.Category == DiagnosisAdmission {
		if !blocker.HasTarget || blocker.Source == "" {
			recommendation.Kind = RecommendationImproveAudit
			recommendation.Target = "admission decision evidence"
			recommendation.Title = "Retain the exact failed admission margin before tuning"
			recommendation.Action = "Record the evaluated boundary, observed value, signed margin, and accepted result for every admission criterion."
			recommendation.Rationale = "The retained reason names admission without a numerical failed boundary."
		} else {
			recommendation.Kind = RecommendationTuneParameter
			recommendation.Target = blocker.Source
			recommendation.Title = "Backtest a " + blocker.Label + " boundary sweep"
			recommendation.Current = blocker.Target
			recommendation.Suggested = blocker.Observed
			recommendation.HasCurrent = true
			recommendation.HasSuggested = true
			recommendation.Adjustment = parameterAdjustment(blocker.Key)
			recommendation.Action = fmt.Sprintf(
				"Replay retained no-action decisions while sweeping %s from %.4f toward %.4f; compare recovered value with wallet loss and false-positive entries.",
				blocker.Source,
				blocker.Target,
				blocker.Observed,
			)
			recommendation.Rationale = "The audit contains an exact failed margin, so this is a testable policy counterfactual."
		}
	}

	return recommendation
}

func parameterAdjustment(key string) string {
	return "lower"
}
