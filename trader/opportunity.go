package trader

import (
	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
)

/*
opportunity is the trader's cross-section-calibrated entry candidate.

Score is thesis_score (aggregated playbook SNR, sigma units). Edge is Score minus
cross-section baseline and spread in the same units. conviction in execution
audit is this same Score.
*/
type opportunity struct {
	Symbol   string
	Score    float64
	Baseline float64
	Spread   float64
	Edge     float64
	Trigger  perspectives.Measurement
	Names    []string
}

/*
entryOpportunity asks every perspective for the flat-entry view and packages the
symbol as a candidate only when at least one playbook authorizes an entry.
*/
func (crypto *Crypto) entryOpportunity(
	symbol string,
	measurements []perspectives.Measurement,
	contextProviders ...func(string) perspectives.DecisionContext,
) (opportunity, bool) {
	var context func(string) perspectives.DecisionContext

	if len(contextProviders) > 0 && contextProviders[0] != nil {
		context = contextProviders[0]
	}

	if context == nil {
		context = crypto.entryContextProvider(symbol, measurements)
	}

	decisions := decision.DecisionsWithContext(measurements, nil, context)
	names := entryNames(decisions)

	if len(names) == 0 {
		return opportunity{}, false
	}

	score := thesisScore(measurements, names)

	if score <= 0 {
		return opportunity{}, false
	}

	feePct := crypto.takerFeePct(symbol)

	if !entryClearsFriction(
		score,
		crypto.scopedRuntime().Risk.EntryEdgeMultiple,
		feePct,
		crypto.quotes.spreadBPS(symbol),
	) {
		return opportunity{}, false
	}

	playbook := primaryPlaybook(names)

	if !crypto.economics.AllowsEntry(playbook) {
		return opportunity{}, false
	}

	return opportunity{
		Symbol:  symbol,
		Score:   score,
		Trigger: strongestPlaybook(measurements, names),
		Names:   names,
	}, true
}

func (crypto *Crypto) marketCalibrated(candidate opportunity) bool {
	_, ok := crypto.calibrateOpportunity(candidate)

	return ok
}

func (crypto *Crypto) calibrateOpportunity(candidate opportunity) (opportunity, bool) {
	snapshot := crypto.ensureCrossSection()

	if len(snapshot.Scores) < 2 {
		return opportunity{}, false
	}

	baseline := snapshot.Baseline
	spread := snapshot.Spread
	candidate.Baseline = baseline
	candidate.Spread = spread
	candidate.Edge = candidate.Score - baseline - spread

	if spread <= 0 {
		candidate.Edge = candidate.Score - baseline
	}

	return candidate, candidate.Edge > 0
}

func (crypto *Crypto) opportunityShare(candidate opportunity) float64 {
	calibrated, ok := crypto.calibrateOpportunity(candidate)

	if !ok {
		return 0
	}

	denominator := calibrated.Score + crypto.marketScoreMass()

	if denominator <= 0 {
		return 0
	}

	share := calibrated.Edge / denominator

	if share > 1 {
		return 1
	}

	return share
}

func (crypto *Crypto) marketScoreMass() float64 {
	scores := crypto.marketScores()
	total := 0.0

	for _, score := range scores {
		if score <= 0 {
			continue
		}

		total += score
	}

	return total
}

func (crypto *Crypto) marketScores() []float64 {
	return crypto.ensureCrossSection().Scores
}

/*
entryRejectReason reports why a symbol with playbook allows still failed desk gates.
Returns an empty reason when no playbook authorized entry.
*/
func (crypto *Crypto) entryRejectReason(
	symbol string,
	measurements []perspectives.Measurement,
	context func(string) perspectives.DecisionContext,
) (string, map[string]any) {
	decisions := decision.DecisionsWithContext(measurements, nil, context)
	names := entryNames(decisions)

	if len(names) == 0 {
		return "", nil
	}

	score := thesisScore(measurements, names)
	fields := map[string]any{
		"playbooks": names,
		"score":     score,
	}

	if score <= 0 {
		return "thesis_score_zero", fields
	}

	feePct := crypto.takerFeePct(symbol)
	spreadBPS := crypto.quotes.spreadBPS(symbol)
	fields["spread_bps"] = spreadBPS
	fields["fee_pct"] = feePct

	if !entryClearsFriction(
		score,
		crypto.scopedRuntime().Risk.EntryEdgeMultiple,
		feePct,
		spreadBPS,
	) {
		fields["required_multiple"] = crypto.scopedRuntime().Risk.EntryEdgeMultiple

		return "friction_gate", fields
	}

	playbook := primaryPlaybook(names)
	fields["playbook"] = playbook

	if !crypto.economics.AllowsEntry(playbook) {
		return "economics_cold", fields
	}

	return "", nil
}

func (crypto *Crypto) entryContextProvider(
	symbol string,
	measurements []perspectives.Measurement,
) func(string) perspectives.DecisionContext {
	snapshot := crypto.ensureCrossSection()
	cache := make(map[string]perspectives.DecisionContext)

	return func(playbook string) perspectives.DecisionContext {
		if context, ok := cache[playbook]; ok {
			return context
		}

		context := crypto.entryDecisionContext(symbol, measurements, playbook, snapshot.Baseline)
		cache[playbook] = context

		return context
	}
}

func (crypto *Crypto) entryDecisionContext(
	symbol string,
	measurements []perspectives.Measurement,
	playbook string,
	baseline float64,
) perspectives.DecisionContext {
	score := thesisScore(measurements, []string{playbook})
	feePct := crypto.takerFeePct(symbol)
	spreadBPS := crypto.quotes.spreadBPS(symbol)
	roundTripCostPct := perspectives.RoundTripFrictionPct(feePct, spreadBPS)
	requiredSNR := perspectives.RequiredThesisScore(
		crypto.scopedRuntime().Risk.EntryEdgeMultiple,
		feePct,
		spreadBPS,
	)
	scoreCostRatio := 0.0

	if requiredSNR > 0 {
		scoreCostRatio = score / requiredSNR
	}

	inPlay := 0.0

	if score > baseline {
		inPlay = 1
	}

	return perspectives.DecisionContext{
		Metrics: map[string]float64{
			perspectives.MetricThesisScore:      score,
			perspectives.MetricSpreadBPS:        spreadBPS,
			perspectives.MetricFeePct:           feePct,
			perspectives.MetricRoundTripCostBPS: roundTripCostPct * 10000,
			perspectives.MetricRequiredScore:    requiredSNR,
			perspectives.MetricScoreCostRatio:   scoreCostRatio,
			perspectives.MetricInPlay:           inPlay,
		},
	}
}

func (crypto *Crypto) calibrateRejectFields(candidate opportunity) map[string]any {
	return map[string]any{
		"playbooks": candidate.Names,
		"score":     candidate.Score,
		"baseline":  candidate.Baseline,
		"spread":    candidate.Spread,
		"edge":      candidate.Edge,
	}
}

func entryNames(decisions []decision.Decision) []string {
	names := make([]string, 0, len(decisions))

	for _, verdict := range decisions {
		if verdict.Action != perspectives.ActionEnter {
			continue
		}

		names = append(names, verdict.Name)
	}

	return names
}

func thesisScore(
	measurements []perspectives.Measurement,
	playbookNames []string,
) float64 {
	return perspectives.AggregateThesisScore(
		measurements,
		playbookCategories(playbookNames),
	)
}

func playbookCategories(playbookNames []string) map[perspectives.CategoryType]bool {
	if len(playbookNames) == 0 {
		return nil
	}

	allowed := make(map[perspectives.CategoryType]bool)

	for _, name := range playbookNames {
		for _, category := range categoriesForPlaybook(name) {
			allowed[category] = true
		}
	}

	return allowed
}

func categoriesForPlaybook(name string) []perspectives.CategoryType {
	if categories := decision.EntryCategoriesForPlaybook(name); len(categories) > 0 {
		return categories
	}

	switch perspectives.PlaybookName(name) {
	case perspectives.PlaybookTrend:
		return []perspectives.CategoryType{
			perspectives.CategoryRiskOnSurge,
			perspectives.CategoryDivergentMove,
			perspectives.CategoryDecoupledAlpha,
			perspectives.CategoryEndogenousAlpha,
			perspectives.CategoryFrenzy,
			perspectives.CategoryLaminar,
			perspectives.CategoryInertial,
			perspectives.CategoryAggressiveDrive,
			perspectives.CategoryHardSupport,
			perspectives.CategoryLoadedImbalance,
		}
	case perspectives.PlaybookDrive:
		return []perspectives.CategoryType{
			perspectives.CategoryAggressiveDrive,
			perspectives.CategoryHiddenAbsorption,
		}
	case perspectives.PlaybookLeadLag:
		return []perspectives.CategoryType{
			perspectives.CategoryRiskOnSurge,
			perspectives.CategoryDivergentMove,
			perspectives.CategoryDecoupledAlpha,
			perspectives.CategoryInefficientLag,
		}
	case perspectives.PlaybookScarcity:
		return []perspectives.CategoryType{
			perspectives.CategoryExtremeScarcity,
			perspectives.CategoryVerticalIgnition,
			perspectives.CategoryCoiledCompression,
		}
	case perspectives.PlaybookPump:
		return []perspectives.CategoryType{
			perspectives.CategoryCoiledCompression,
			perspectives.CategorySpoofTrap,
			perspectives.CategoryVerticalIgnition,
		}
	default:
		return nil
	}
}

func strongestPlaybook(
	measurements []perspectives.Measurement,
	playbookNames []string,
) perspectives.Measurement {
	relevant := playbookCategories(playbookNames)
	var best perspectives.Measurement

	for _, measurement := range measurements {
		if len(relevant) > 0 && !relevant[measurement.Category] {
			continue
		}

		if measurement.SNR > best.SNR {
			best = measurement
		}
	}

	return best
}

func robustCenter(values []float64) (median, mad float64) {
	if len(values) == 0 {
		return 0, 0
	}

	sorted := numeric.CopySorted(values)
	median = numeric.Median(values)
	mad = numeric.MedianAbsoluteDeviation(sorted, median)

	return median, mad
}
