package trader

import (
	"math"
	"time"

	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric"
)

/*
opportunity is the trader's cross-section-calibrated entry candidate. Score is the
symbol's live thesis strength; Baseline and Spread are the current market's robust
median and MAD, so Edge is how far this symbol stands above what the rest of the
market is doing now.
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

	score := thesisScore(measurements, len(names))

	if score <= 0 {
		return opportunity{}, false
	}

	return opportunity{
		Symbol:  symbol,
		Score:   score,
		Trigger: strongest(measurements),
		Names:   names,
	}, true
}

/*
marketCalibrated accepts only candidates that are outliers versus the current
cross-section of all observed symbols. This keeps a broad, uniform warm-up signal
from opening the entire wallet while still allowing several simultaneous entries
when several symbols genuinely stand above the live market distribution.
*/
func (crypto *Crypto) marketCalibrated(candidate opportunity) bool {
	_, ok := crypto.calibrateOpportunity(candidate)

	return ok
}

func (crypto *Crypto) calibrateOpportunity(candidate opportunity) (opportunity, bool) {
	scores := crypto.marketScores()

	if len(scores) < 2 {
		return opportunity{}, false
	}

	baseline, spread := robustCenter(scores)
	candidate.Baseline = baseline
	candidate.Spread = spread
	candidate.Edge = candidate.Score - baseline - spread

	if spread <= 0 {
		candidate.Edge = candidate.Score - baseline
	}

	return candidate, candidate.Edge > 0
}

/*
opportunityShare sizes a candidate by its positive edge as a share of all live
thesis energy currently visible in the market. The denominator includes the whole
observed cross-section, not just qualified candidates, so a single outlier does not
automatically consume the whole wallet and later concurrent strategies still have
capital to work with.
*/
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
	crypto.mu.RLock()
	defer crypto.mu.RUnlock()

	now := time.Now()
	scores := make([]float64, 0, len(crypto.readings))

	for _, set := range crypto.readings {
		measurements := snapshotTimedMeasurements(set, now)
		scores = append(scores, thesisScore(measurements, 0))
	}

	return scores
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
	confirmations int,
) float64 {
	energy := 0.0
	observations := 0

	for _, measurement := range measurements {
		strength := measurement.SNR

		if strength <= 0 {
			strength = measurement.Confidence
		}

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

	if confirmations <= 1 {
		return score
	}

	return score * math.Sqrt(float64(confirmations))
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
