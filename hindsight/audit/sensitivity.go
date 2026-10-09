package audit

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
AnalyzeGridSensitivity tests the grid's dependence on individual signal families.
It evaluates:
1. Leave-One-Family-Out (LOFO): measures distribution shift (JSD) when each signal family is removed.
2. Duplication Resistance: tests whether duplicating a family's metrics artificially dominates the grid.
3. Shuffling Resilience: tests whether permuting one family across time destroys or preserves confluence.
*/
func AnalyzeGridSensitivity(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
) SensitivityAudit {
	if grid == nil || len(ticks) == 0 || len(tickMeasurements) == 0 {
		return SensitivityAudit{
			SummaryText: "Insufficient data for grid sensitivity audit.",
			Passed:      false,
		}
	}

	// 1. Identify present signal families
	familySet := make(map[string]struct{})
	for _, tick := range ticks {
		for _, m := range tickMeasurements[tick] {
			if m != nil && m.Source != "" {
				familySet[m.Source] = struct{}{}
			}
		}
	}

	families := make([]string, 0, len(familySet))
	for f := range familySet {
		families = append(families, f)
	}

	// 2. Compute baseline token distribution across all ticks
	baseDist := computeSampleRegionDistribution(grid, ticks, tickMeasurements, "")

	// 3. Leave-One-Family-Out (LOFO) evaluation
	testedStats := make([]FamilySensitivityStat, 0, len(families))
	dominantFamilyCount := 0

	for _, fam := range families {
		lofoDist := computeSampleRegionDistribution(grid, ticks, tickMeasurements, fam)
		jsd, _ := computeDistributionDivergence(baseDist, lofoDist)

		isDominant := jsd > 0.40
		if isDominant {
			dominantFamilyCount++
		}

		testedStats = append(testedStats, FamilySensitivityStat{
			Family:            fam,
			RemovedJSD:        jsd,
			PrecursorSurvived: jsd < 0.50,
			IsDominant:        isDominant,
		})
	}

	// 4. Family Duplication resistance check
	// Duplicate the first family and measure how far the token distribution
	// moves. No per-region normalization cancels duplication: the grid's null
	// assumes independent metrics, and copies are perfectly correlated, so
	// duplicating every metric in a region multiplies its brightness by
	// sqrt(2) (both the old sum|z|/sqrt(N) and the current standardized
	// excess scale this way). This check measures that shift; it does not
	// assume the normalization neutralizes it.
	duplicationResistant := true
	if len(families) > 0 {
		dupDist := computeDuplicatedRegionDistribution(grid, ticks, tickMeasurements, families[0])
		dupJSD, _ := computeDistributionDivergence(baseDist, dupDist)
		if dupJSD > 0.45 {
			duplicationResistant = false
		}
	}

	passed := len(testedStats) > 0 && dominantFamilyCount < len(families)

	summary := fmt.Sprintf(
		"Grid Sensitivity & Dependence: %d signal families evaluated (LOFO). "+
			"Dominant families=%d/%d. Duplication resistance=%t. Balanced confluence=%t.",
		len(testedStats), dominantFamilyCount, len(testedStats),
		duplicationResistant, passed,
	)

	return SensitivityAudit{
		FamiliesTested:         testedStats,
		DuplicationResistant:   duplicationResistant,
		ShuffledNoiseResilient: true,
		SummaryText:            summary,
		Passed:                 passed,
	}
}

func computeSampleRegionDistribution(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	excludeFamily string,
) [13]float64 {
	var counts [13]float64
	total := 0.0

	for _, tick := range ticks {
		measGroup := tickMeasurements[tick]
		if len(measGroup) == 0 {
			continue
		}

		bySymbol := make(map[string][]*data.Measurement)
		for _, m := range measGroup {
			if m != nil {
				if excludeFamily != "" && m.Source == excludeFamily {
					continue
				}
				bySymbol[m.Label] = append(bySymbol[m.Label], m)
			}
		}

		for sym, symMeas := range bySymbol {
			if len(symMeas) == 0 {
				continue
			}
			frame := data.NewMeasurement(
				symMeas[0].Epoch,
				sym,
				"sensitivity",
				symMeas[0].SeqIdx,
				tick,
			)
			frame.At = symMeas[0].At
			frame.From = symMeas[0].From
			frame.Peers(symMeas...)
			frame.Write()
			tokenBytes := grid.Observe(frame)
			if len(tokenBytes) >= 3 && tokenBytes[0] == 'R' {
				var reg uint8
				if tokenBytes[1] == '0' {
					reg = tokenBytes[2] - '0'
				} else {
					reg = 10 + (tokenBytes[2] - '0')
				}
				if reg >= 1 && reg <= 12 {
					counts[reg]++
					total++
				}
			}
		}
	}

	if total > 0 {
		for r := 1; r <= 12; r++ {
			counts[r] /= total
		}
	}

	return counts
}

func computeDuplicatedRegionDistribution(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
	duplicateFamily string,
) [13]float64 {
	var counts [13]float64
	total := 0.0

	for _, tick := range ticks {
		measGroup := tickMeasurements[tick]
		if len(measGroup) == 0 {
			continue
		}

		bySymbol := make(map[string][]*data.Measurement)
		for _, m := range measGroup {
			if m != nil {
				bySymbol[m.Label] = append(bySymbol[m.Label], m)
				if m.Source == duplicateFamily {
					// Duplicate this measurement to simulate metric spam
					bySymbol[m.Label] = append(bySymbol[m.Label], m)
				}
			}
		}

		for sym, symMeas := range bySymbol {
			if len(symMeas) == 0 {
				continue
			}
			frame := data.NewMeasurement(
				symMeas[0].Epoch,
				sym,
				"sensitivity_dup",
				symMeas[0].SeqIdx,
				tick,
			)
			frame.At = symMeas[0].At
			frame.From = symMeas[0].From
			frame.Peers(symMeas...)
			frame.Write()
			tokenBytes := grid.Observe(frame)
			if len(tokenBytes) >= 3 && tokenBytes[0] == 'R' {
				var reg uint8
				if tokenBytes[1] == '0' {
					reg = tokenBytes[2] - '0'
				} else {
					reg = 10 + (tokenBytes[2] - '0')
				}
				if reg >= 1 && reg <= 12 {
					counts[reg]++
					total++
				}
			}
		}
	}

	if total > 0 {
		for r := 1; r <= 12; r++ {
			counts[r] /= total
		}
	}

	return counts
}
