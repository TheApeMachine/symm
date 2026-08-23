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
	case DiagnosisAdmission:
		if !blocker.HasTarget || blocker.Source == "" {
			recommendation.Kind = RecommendationImproveAudit
			recommendation.Target = "admission decision evidence"
			recommendation.Title = "Retain the exact failed admission margin before tuning"
			recommendation.Action = "Record the evaluated boundary, observed value, signed margin, and accepted result for every admission criterion; then re-run hindsight so a replay sweep can use the actual counterfactual rather than a textual rejection."
			recommendation.Rationale = "The retained reason names admission, but it does not contain a numerical failed boundary. Changing a live parameter from prose alone would be unsafe."
			break
		}

		recommendation.Kind = RecommendationTuneParameter
		recommendation.Title = "Backtest a " + blocker.Label + " boundary sweep"
		recommendation.Target = blocker.Source
		recommendation.Current = blocker.Target
		recommendation.Suggested = blocker.Observed
		recommendation.HasCurrent = true
		recommendation.HasSuggested = true
		recommendation.Adjustment = parameterAdjustment(blocker.Key)
		recommendation.Action = fmt.Sprintf(
			"Replay retained no-action decisions while sweeping %s from %.4f toward %.4f; compare recovered value with wallet loss and false-positive entries before changing the live boundary.",
			blocker.Source,
			blocker.Target,
			blocker.Observed,
		)
		recommendation.Rationale = "The audit contains an exact failed margin, so this is a testable policy counterfactual rather than a guessed signal rewrite."
	case DiagnosisDirection:
		recommendation.Kind = RecommendationImproveMeasurement
		recommendation.Target = "directional thesis inputs"
		recommendation.Title = "Repair directional evidence before relaxing admission"
		recommendation.Action = "Measure sign accuracy and onset time for each contributing source around missed legs, then recalibrate or replace the sources that consistently point against realized forward returns."
		recommendation.Rationale = "Changing the required long direction would admit trades the thesis itself says are wrong; the evidence layer must improve first."
	case DiagnosisMeasurement:
		recommendation.Kind = RecommendationImproveMeasurement
		recommendation.Target = firstNonEmpty(blocker.Source, "opportunity detection")
		recommendation.Title = measurementRecommendationTitle(blocker)
		recommendation.Action = measurementRecommendationAction(blocker)
		recommendation.Rationale = "The missed move was absent from the opportunity vocabulary or opposed by a recorded measurement, so policy tuning alone cannot create a reliable early signal."
	case DiagnosisPredictive:
		recommendation.Kind = RecommendationCollectOutcomes
		recommendation.Target = "resonance forward-outcome ledger"
		recommendation.Title = "Resolve blocked decisions into predictive training outcomes"
		recommendation.Action = "Keep no-action and blocked candidates in the temporal outcome ledger, publish calibration count and forward error by opportunity type, and re-run hindsight after the model reaches evidence coverage for this regime."
		recommendation.Rationale = "Predictive readiness was incomplete; more retained outcomes can improve measurement confidence without turning readiness into an unrecorded veto."
	case DiagnosisAllocation:
		recommendation.Kind = RecommendationFixAllocation
		recommendation.Target = "allocation and slot policy"
		recommendation.Title = "Replay the allocation frontier for qualified opportunities"
		recommendation.Action = "Record normal-slot, reserve-slot, capital, displacement, and haircut outcomes at the exact rejection point; then simulate the smallest capacity or allocation-policy change that would have admitted the missed setup."
		recommendation.Rationale = "The setup reached allocation, so adding signal strength will not recover it unless capacity policy changes or the candidate displaces a weaker position."
	case DiagnosisExecution:
		recommendation.Kind = RecommendationFixExecution
		recommendation.Target = "execution coverage and friction measurements"
		recommendation.Title = "Make execution feasibility measurable before admission"
		recommendation.Action = "Retain visible depth coverage, spread, impact, fee, quantity, and desk rejection at the decision timestamp; size or reject from those measurements and replay whether a smaller executable order captured net value."
		recommendation.Rationale = "The opportunity existed, but current market geometry could not carry the proposed order honestly."
	case DiagnosisRegulator:
		recommendation.Kind = RecommendationValidateRegulator
		recommendation.Target = "global regulator entry-release policy"
		recommendation.Title = "Replay the regulator release boundary around delayed entries"
		recommendation.Action = "Retain regulator status, surprise, energy, prediction activity, and release transitions at each delayed decision; replay whether an earlier release captured the move without increasing drawdown or unstable entries."
		recommendation.Rationale = "The opportunity and policy path were ready, but the global regulator intentionally held execution while observing or adapting."
	case DiagnosisObservability:
		recommendation.Kind = RecommendationImproveAudit
		recommendation.Target = "decision audit stream"
		recommendation.Title = "Close the decision-evidence gap before tuning"
		recommendation.Action = "Retain every evaluated decision for this exact symbol through the outcome horizon, including nothing decisions, failed admission margins, allocation results, desk validation, and venue submission status."
		recommendation.Rationale = "Without same-symbol decision evidence, any parameter or measurement recommendation would be post-hoc fiction."
	default:
		recommendation.Kind = RecommendationInstrumentFunnel
		recommendation.Target = "planner → allocation → desk → venue funnel"
		recommendation.Title = "Identify the first stage that stopped entry"
		recommendation.Action = "Emit one retained stage outcome for admission, ranking, allocation, desk validation, order submission, and venue acknowledgement so the next hindsight pass can name the first failed boundary."
		recommendation.Rationale = "The retained reason is insufficient to distinguish a signal problem from a downstream execution problem."
	}

	if recommendation.Target == "" {
		recommendation.Target = blocker.Key
	}

	return recommendation
}

func measurementRecommendationTitle(blocker Blocker) string {
	if blocker.Key == "opportunity:not_classified" {
		return "Develop an earlier opportunity measurement for this move class"
	}

	return "Recalibrate the opposing " + firstNonEmpty(blocker.Source, "measurement") + " evidence"
}

func measurementRecommendationAction(blocker Blocker) string {
	if blocker.Key == "opportunity:not_classified" {
		return "Label the move from its actual pre-entry market state, add a same-symbol measurement whose timestamp precedes the leg, and validate onset, sign precision, and net tradability on held-out captures."
	}

	return fmt.Sprintf(
		"Audit %s around every missed and losing leg, measure forward sign precision by regime, and change or replace it only when held-out outcomes show the recorded %.4f opposition was systematically wrong.",
		firstNonEmpty(blocker.Source, blocker.Key),
		blocker.Observed,
	)
}

func parameterAdjustment(key string) string {
	if key == "admission:contradiction" {
		return "raise"
	}

	return "lower"
}
