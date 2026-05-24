package trader

import "sort"

/*
statusPayload maps portfolio telemetry into the dashboard status wire shape.
*/
func statusPayload(snapshot StatusSnapshot) map[string]any {
	payload := map[string]any{
		"equity_eur":     snapshot.EquityEUR,
		"cash_eur":       snapshot.CashEUR,
		"closed_pnl_eur": snapshot.ClosedPnLEUR,
		"trade_count":    snapshot.TradeCount,
		"win_rate":       snapshot.WinRate,
		"open_count":     snapshot.OpenCount,
	}

	if len(snapshot.Positions) > 0 {
		payload["positions"] = snapshot.Positions
	}

	return payload
}

func scoreboardTargets(
	decision DecisionSnapshot,
	quotes QuoteReader,
	riskReader RiskReader,
) []map[string]any {
	targets := make([]map[string]any, 0, len(decision.Evaluations))

	for _, evaluation := range decision.Evaluations {
		trailPct := 0.0

		if quotes != nil && evaluation.Symbol != "" {
			last, bid, ask, _, ok := quotes.Quote(evaluation.Symbol)

			if ok {
				trailPct = clampTrailPct(trailPctFromQuoteRisk(
					last, bid, ask, evaluation.Symbol, riskReader, nil,
				))
			}
		}

		targets = append(targets, map[string]any{
			"symbol":          evaluation.Symbol,
			"regime":          evaluation.Regime,
			"reason":          evaluation.Reason,
			"score":           evaluation.CombinedScore,
			"effective_score": evaluation.CombinedScore,
			"trail_pct":       trailPct,
		})
	}

	return targets
}

func evaluationRows(
	decision DecisionSnapshot,
	candidates CandidateStore,
	liveCandidates CandidateStore,
) []map[string]any {
	rows := make([]map[string]any, 0, len(decision.Evaluations))

	for _, evaluation := range decision.Evaluations {
		signals := candidateSignals(candidates, evaluation.Symbol)
		liveSignals := candidateSignals(liveCandidates, evaluation.Symbol)

		row := evaluationToMap(evaluation)

		if len(signals) > 0 {
			row["signals"] = signals
		}

		if len(liveSignals) > 0 {
			row["live_signals"] = liveSignals
		}

		rows = append(rows, row)
	}

	return rows
}

func candidateSignals(
	candidates CandidateStore,
	symbol string,
) []map[string]any {
	sources, ok := candidates.bySymbol[symbol]

	if !ok || len(sources) == 0 {
		return nil
	}

	sourceNames := make([]string, 0, len(sources))

	for source := range sources {
		sourceNames = append(sourceNames, source)
	}

	sort.Strings(sourceNames)

	rows := make([]map[string]any, 0, len(sourceNames))

	for _, source := range sourceNames {
		candidate := sources[source]

		rows = append(rows, map[string]any{
			"source":     candidate.Source,
			"regime":     candidate.Regime,
			"reason":     candidate.Reason,
			"confidence": candidate.Confidence,
			"executable": candidate.Executable,
		})
	}

	return rows
}

func decisionTracePayload(
	decision DecisionSnapshot,
	candidates CandidateStore,
	liveCandidates CandidateStore,
) map[string]any {
	decisions := make([]map[string]any, 0, len(decision.Decisions))
	allowed := 0

	for _, row := range decision.Decisions {
		decisions = append(decisions, decisionToMap(row))

		if row.Allow {
			allowed++
		}
	}

	return map[string]any{
		"line":        decision.Line,
		"median":      decision.Median,
		"mad":         decision.MAD,
		"scored":      len(decision.Decisions),
		"in_play":     len(decision.Decisions),
		"allowed":     allowed,
		"decisions":   decisions,
		"evaluations": evaluationRows(decision, candidates, liveCandidates),
	}
}
