package trader

import (
	"math"

	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
)

/*
opportunity is the trader's cross-section-calibrated entry candidate.
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
) (opportunity, bool) {
	decisions := decision.Decisions(measurements, nil)
	names := entryNames(decisions)

	if len(names) == 0 {
		return opportunity{}, false
	}

	score := thesisScore(measurements, names)

	if score <= 0 {
		return opportunity{}, false
	}

	feePct := crypto.takerFeePct(symbol)

	if !entryClearsFriction(score, feePct, crypto.quotes.spreadBPS(symbol)) {
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

/*
thesisScore aggregates SNR only from measurements relevant to the authorizing
playbooks (categories those trees can read).
*/
func thesisScore(
	measurements []perspectives.Measurement,
	playbookNames []string,
) float64 {
	relevant := playbookCategories(playbookNames)
	energy := 0.0
	observations := 0

	for _, measurement := range measurements {
		if len(relevant) > 0 && !relevant[measurement.Category] {
			continue
		}

		strength := measurement.SNR

		if strength <= 0 {
			continue
		}

		energy += strength * strength
		observations++
	}

	if observations == 0 {
		return 0
	}

	score := math.Sqrt(energy / float64(observations))

	if len(playbookNames) <= 1 {
		return score
	}

	return score * math.Sqrt(float64(len(playbookNames)))
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
