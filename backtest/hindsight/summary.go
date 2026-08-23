package hindsight

import "sort"

/*
AggregateRecommendations ranks the smallest set of next experiments by missed
value. Parameter candidates are combined only while their live boundary stayed
identical; a moving regulator boundary is left unmerged rather than reported as
one fictional setting.
*/
func AggregateRecommendations(reports []PerSymbol) []Recommendation {
	aggregates := map[string]*recommendationAggregate{}

	for _, report := range reports {
		for _, opportunity := range report.Opportunities {
			if !opportunity.Missed {
				continue
			}

			recommendation := opportunity.Diagnosis.Recommendation

			if recommendation.Key == "" {
				continue
			}

			aggregate := aggregates[recommendation.Key]

			if aggregate == nil {
				aggregate = &recommendationAggregate{
					recommendation: recommendation,
					symbols:        map[string]struct{}{},
					currentStable:  true,
				}
				aggregate.recommendation.ImpactPct = 0
				aggregate.recommendation.Occurrences = 0
				aggregate.recommendation.Symbols = nil
				aggregates[recommendation.Key] = aggregate
			}

			aggregate.recommendation.ImpactPct += opportunity.Leg.ProfitPct
			aggregate.recommendation.Occurrences++
			aggregate.weightedTrust += opportunity.Diagnosis.EvidenceQuality * opportunity.Leg.ProfitPct
			aggregate.weight += opportunity.Leg.ProfitPct
			aggregate.symbols[report.Symbol] = struct{}{}
			mergeRecommendationBoundary(aggregate, recommendation)
		}
	}

	result := make([]Recommendation, 0, len(aggregates))

	for _, aggregate := range aggregates {
		if aggregate.weight > 0 {
			aggregate.recommendation.Confidence = aggregate.weightedTrust / aggregate.weight
		}

		if !aggregate.currentStable {
			aggregate.recommendation.HasCurrent = false
			aggregate.recommendation.HasSuggested = false
			aggregate.recommendation.Action += " The live boundary moved during this capture, so inspect each opportunity's exact current and candidate values rather than applying one aggregate number."
		}

		for symbol := range aggregate.symbols {
			aggregate.recommendation.Symbols = append(
				aggregate.recommendation.Symbols,
				symbol,
			)
		}

		sort.Strings(aggregate.recommendation.Symbols)
		result = append(result, aggregate.recommendation)
	}

	sort.SliceStable(result, func(left, right int) bool {
		if result[left].ImpactPct != result[right].ImpactPct {
			return result[left].ImpactPct > result[right].ImpactPct
		}

		if result[left].Occurrences != result[right].Occurrences {
			return result[left].Occurrences > result[right].Occurrences
		}

		return result[left].Key < result[right].Key
	})

	return result
}

func mergeRecommendationBoundary(
	aggregate *recommendationAggregate,
	recommendation Recommendation,
) {
	if !recommendation.HasCurrent || !recommendation.HasSuggested {
		return
	}

	if !aggregate.currentSet {
		aggregate.current = recommendation.Current
		aggregate.currentSet = true
		aggregate.recommendation.Current = recommendation.Current
		aggregate.recommendation.Suggested = recommendation.Suggested
		aggregate.recommendation.HasCurrent = true
		aggregate.recommendation.HasSuggested = true
		return
	}

	if aggregate.current != recommendation.Current {
		aggregate.currentStable = false
		return
	}

	switch recommendation.Adjustment {
	case "lower":
		if recommendation.Suggested < aggregate.recommendation.Suggested {
			aggregate.recommendation.Suggested = recommendation.Suggested
		}
	case "raise":
		if recommendation.Suggested > aggregate.recommendation.Suggested {
			aggregate.recommendation.Suggested = recommendation.Suggested
		}
	}
}

/* RootCauseSummaries ranks failure classes by the value left untraded. */
func RootCauseSummaries(reports []PerSymbol) []RootCauseSummary {
	summaries := map[string]*RootCauseSummary{}
	symbols := map[string]map[string]struct{}{}

	for _, report := range reports {
		for _, opportunity := range report.Opportunities {
			if !opportunity.Missed || opportunity.Diagnosis.Category == "" {
				continue
			}

			category := opportunity.Diagnosis.Category
			summary := summaries[category]

			if summary == nil {
				summary = &RootCauseSummary{Category: category}
				summaries[category] = summary
				symbols[category] = map[string]struct{}{}
			}

			summary.ImpactPct += opportunity.Leg.ProfitPct
			summary.Occurrences++
			symbols[category][report.Symbol] = struct{}{}
		}
	}

	result := make([]RootCauseSummary, 0, len(summaries))

	for category, summary := range summaries {
		for symbol := range symbols[category] {
			summary.Symbols = append(summary.Symbols, symbol)
		}

		sort.Strings(summary.Symbols)
		result = append(result, *summary)
	}

	sort.SliceStable(result, func(left, right int) bool {
		if result[left].ImpactPct != result[right].ImpactPct {
			return result[left].ImpactPct > result[right].ImpactPct
		}

		return result[left].Category < result[right].Category
	})

	return result
}

/* DiagnosticCoverage reports the share of missed legs with retained decisions. */
func DiagnosticCoverage(reports []PerSymbol) float64 {
	missed := 0
	diagnosed := 0

	for _, report := range reports {
		for _, opportunity := range report.Opportunities {
			if !opportunity.Missed {
				continue
			}

			missed++

			if opportunity.Diagnosis.EvidenceStatus != "missing" {
				diagnosed++
			}
		}
	}

	if missed == 0 {
		return 1
	}

	return float64(diagnosed) / float64(missed)
}
